//go:build windows

package device

import (
	"bufio"
	"bytes"
	"context"
	"cullsnap/internal/logger"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// importScript is the PowerShell script that imports photos from a device via MTP
// using the Windows Shell.Application COM object. It reads a JSON object with
// "destDir" and "deviceName" fields from stdin, finds the device by name,
// discovers the DCIM folder dynamically, and copies each file to the destination.
// Progress is reported as NDJSON lines on stdout.
//
// Security: this script is a compile-time constant and is never modified at runtime.
// Parameters are passed via stdin JSON, never interpolated into the script text.
const importScript = `$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$inputJson = [Console]::In.ReadToEnd()
$params = $inputJson | ConvertFrom-Json
$destDir = $params.destDir
$deviceName = $params.deviceName

$shell = New-Object -ComObject Shell.Application

$myComputer = $shell.Namespace(0x11)
$device = $null
foreach ($item in $myComputer.Items()) {
    if ($item.Name -eq $deviceName -or $item.Name -match [regex]::Escape($deviceName)) {
        $device = $item
        break
    }
}

if (-not $device) {
    @{ type = 'error'; code = 'no_device'; message = "Device '$deviceName' not found. Connect it via USB and try again." } |
        ConvertTo-Json -Compress
    exit 1
}

# Find storage that contains DCIM (handles "Internal Storage", "Internal shared storage", "Phone", "SD Card", etc.)
$storage = $null
foreach ($storageItem in $device.GetFolder.Items()) {
    $sub = $storageItem.GetFolder
    if ($sub) {
        foreach ($child in $sub.Items()) {
            if ($child.Name -eq 'DCIM') {
                $storage = $storageItem
                break
            }
        }
    }
    if ($storage) { break }
}

if (-not $storage) {
    @{ type = 'error'; code = 'not_trusted';
       message = 'Cannot access device storage. Ensure the device is unlocked and has granted file access, then try again.' } |
        ConvertTo-Json -Compress
    exit 1
}

$dcim = $storage.GetFolder.Items() |
    Where-Object { $_.Name -eq 'DCIM' }
if (-not $dcim) {
    @{ type = 'error'; code = 'no_dcim'; message = 'No photos found on device (DCIM folder not found).' } |
        ConvertTo-Json -Compress
    exit 1
}

$dcimFolder = $dcim.GetFolder
$total = 0
foreach ($sub in $dcimFolder.Items()) {
    $total += $sub.GetFolder.Items().Count
}

@{ type = 'enumerate_done'; total = $total; device = $device.Name } |
    ConvertTo-Json -Compress

$copied = 0
foreach ($subfolder in $dcimFolder.Items()) {
    $subObj = $subfolder.GetFolder
    $subDestDir = Join-Path $destDir $subfolder.Name

    if (-not (Test-Path $subDestDir)) {
        New-Item -ItemType Directory -Path $subDestDir -Force | Out-Null
    }

    $destNS = $shell.Namespace($subDestDir)

    foreach ($item in $subObj.Items()) {
        $targetPath = Join-Path $subDestDir $item.Name

        if (Test-Path $targetPath) {
            $existing = Get-Item $targetPath
            if ($existing.Length -eq $item.Size) {
                $copied++
                @{ type = 'skip'; name = $item.Name;
                   progress = $copied; total = $total } |
                    ConvertTo-Json -Compress
                continue
            }
        }

        try {
            $destNS.CopyHere($item, 0x0414)

            if (Test-Path $targetPath) {
                $copied++
                @{ type = 'copied'; name = $item.Name;
                   progress = $copied; total = $total } |
                    ConvertTo-Json -Compress
            } else {
                @{ type = 'copy_failed'; name = $item.Name;
                   message = 'File not found after copy' } |
                    ConvertTo-Json -Compress
                $copied++
            }
        } catch {
            @{ type = 'copy_error'; name = $item.Name;
               message = $_.Exception.Message } |
                ConvertTo-Json -Compress
            $copied++
        }
    }
}

@{ type = 'complete'; copied = $copied; total = $total } |
    ConvertTo-Json -Compress
`

// ImportFromDevice imports photos from a connected device via MTP or direct copy
// on Windows. For mass storage devices (SD cards, USB drives), it copies files
// directly. For MTP devices (phones, cameras), it uses a PowerShell script with
// the Shell.Application COM object.
//
// Returns the import directory path, count of imported files, and any error.
//
// Security:
//   - serial is sanitized via SanitizeSerial before use in path construction
//   - importDir is validated to be strictly under baseDir via validateDestDir
//   - PowerShell script is a compile-time constant, parameters passed via stdin JSON
//   - Post-copy path traversal verification removes any files that escape the root
func ImportFromDevice(ctx context.Context, serial, baseDir string) (string, int, error) {
	cleanSerial := SanitizeSerial(serial)
	importDir := filepath.Join(baseDir, cleanSerial)

	if err := validateDestDir(importDir, baseDir); err != nil {
		return "", 0, fmt.Errorf("device: security check failed: %w", err)
	}

	if err := os.MkdirAll(importDir, 0o700); err != nil {
		return "", 0, fmt.Errorf("device: failed to create import dir: %w", err)
	}

	// Look up device in detector state for type and mount path.
	dev, found := lookupDeviceBySerial(serial)
	if !found || dev.Name == "" {
		return "", 0, fmt.Errorf("device: device with serial %q not found in detector state — reconnect and try again", serial)
	}

	logger.Log.Info("device: starting Windows import",
		"serial", serial,
		"type", dev.Type,
		"mountPath", dev.MountPath,
		"importDir", importDir,
	)

	// Mass storage: direct file copy (no Shell.Application needed).
	if dev.Type == "storage" && dev.MountPath != "" {
		count, err := importFromDrive(ctx, dev.MountPath, importDir)
		if err != nil {
			verifyNoPathTraversal(importDir)
			return importDir, count, err
		}
		removed := verifyNoPathTraversal(importDir)
		if removed > 0 {
			logger.Log.Warn("device: removed files that escaped import directory", "removed", removed)
		}
		return importDir, count, nil
	}

	// MTP/PTP devices: Shell.Application via PowerShell.
	type importParams struct {
		DestDir    string `json:"destDir"`
		DeviceName string `json:"deviceName"`
	}
	paramsJSON, err := json.Marshal(importParams{
		DestDir:    importDir,
		DeviceName: dev.Name,
	})
	if err != nil {
		return "", 0, fmt.Errorf("device: failed to marshal import params: %w", err)
	}

	// Execute via runPowerShellWithStdin which uses the hardened execPowerShell
	// helper (hardcoded powershellExe constant, no variable executable path).
	stdout, stderr, runErr := runPowerShellWithStdin(ctx, importScript, paramsJSON)

	// Parse NDJSON progress lines from the buffered stdout.
	var (
		lastErr    error
		deviceName string
		totalFiles int
	)

	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		ev, parseErr := parseProgressLine(line)
		if parseErr != nil {
			logger.Log.Debug("device: skipping unparseable progress line",
				"line", string(line),
				"error", parseErr,
			)
			continue
		}

		switch ev.Type {
		case "enumerate_done":
			totalFiles = ev.Total
			deviceName = ev.Device
			logger.Log.Info("device: enumeration complete",
				"device", deviceName,
				"totalFiles", totalFiles,
			)
			if totalFiles > maxFileCount {
				logger.Log.Error("device: file count exceeds safety limit",
					"total", totalFiles,
					"limit", maxFileCount,
				)
				return "", 0, fmt.Errorf("device: too many files on device (%d, limit is %d)", totalFiles, maxFileCount)
			}

		case "copied":
			logger.Log.Debug("device: copied file",
				"name", ev.Name,
				"progress", ev.Progress,
				"total", ev.Total,
			)

		case "skip":
			logger.Log.Debug("device: skipped existing file",
				"name", ev.Name,
				"progress", ev.Progress,
				"total", ev.Total,
			)

		case "copy_failed":
			logger.Log.Warn("device: file copy failed",
				"name", ev.Name,
				"message", ev.Message,
			)

		case "copy_error":
			logger.Log.Warn("device: file copy error",
				"name", ev.Name,
				"message", ev.Message,
			)

		case "error":
			lastErr = fmt.Errorf("device: %s", userFriendlyError(ev))
			logger.Log.Error("device: script reported error",
				"code", ev.Code,
				"message", ev.Message,
			)

		case "complete":
			logger.Log.Info("device: import complete",
				"copied", ev.Copied,
				"total", ev.Total,
			)

		default:
			logger.Log.Debug("device: unknown progress event type",
				"type", ev.Type,
				"line", string(line),
			)
		}
	}

	if scanErr := scanner.Err(); scanErr != nil {
		logger.Log.Error("device: scanner error reading stdout", "error", scanErr)
	}

	// Handle PowerShell execution error.
	if runErr != nil && lastErr == nil {
		stderrOut := bytes.TrimSpace(stderr)
		logger.Log.Error("device: PowerShell process exited with error",
			"error", runErr,
			"stderr", string(stderrOut),
		)
		// If we got some files before the error, return a partial result.
		importedCount := countFilesRecursive(importDir)
		if importedCount > 0 {
			logger.Log.Warn("device: partial import — returning files copied before error",
				"importedCount", importedCount,
			)
			verifyNoPathTraversal(importDir)
			return importDir, importedCount, fmt.Errorf("device: import partially completed (%d files) before error: %w", importedCount, runErr)
		}
		return "", 0, fmt.Errorf("device: PowerShell import failed: %w", runErr)
	}

	if lastErr != nil {
		return "", 0, lastErr
	}

	// Post-copy security check: remove any files that escape the import directory.
	removed := verifyNoPathTraversal(importDir)
	if removed > 0 {
		logger.Log.Warn("device: removed files that escaped import directory",
			"removed", removed,
		)
	}

	finalCount := countFilesRecursive(importDir)
	logger.Log.Info("device: import finished successfully",
		"serial", serial,
		"importDir", importDir,
		"fileCount", finalCount,
	)

	return importDir, finalCount, nil
}

// userFriendlyError maps PowerShell error codes to user-friendly messages.
func userFriendlyError(evt ProgressEvent) string {
	switch evt.Code {
	case "no_device":
		return "Device not found. Connect your device via USB and try again."
	case "not_trusted":
		return "Cannot access device storage. Ensure the device is unlocked and has granted file access, then try again."
	case "no_dcim":
		return "No photos found on device."
	default:
		return evt.Message
	}
}

// importFromDrive copies files from a mounted removable drive's DCIM folder.
func importFromDrive(ctx context.Context, drivePath, destDir string) (int, error) {
	dcimPath := filepath.Join(drivePath, string(filepath.Separator), "DCIM")
	if _, err := os.Stat(dcimPath); os.IsNotExist(err) {
		logger.Log.Info("device: no DCIM folder on drive", "path", drivePath)
		return 0, nil
	}

	total := countFilesRecursive(dcimPath)
	logger.Log.Info("device: drive DCIM enumeration complete", "total", total)
	if total > maxFileCount {
		return 0, fmt.Errorf("device: too many files (%d, limit is %d)", total, maxFileCount)
	}

	copied := 0
	err := filepath.Walk(dcimPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if info.IsDir() {
			return nil
		}

		relPath, relErr := filepath.Rel(dcimPath, path)
		if relErr != nil {
			return nil
		}

		destPath := filepath.Join(destDir, relPath)

		if existingInfo, statErr := os.Stat(destPath); statErr == nil {
			if existingInfo.Size() == info.Size() {
				copied++
				return nil
			}
		}

		if mkdirErr := os.MkdirAll(filepath.Dir(destPath), 0o700); mkdirErr != nil {
			return nil
		}

		if cpErr := copyFile(path, destPath); cpErr != nil {
			logger.Log.Warn("device: failed to copy file", "name", info.Name(), "error", cpErr)
			return nil
		}

		copied++
		if copied%100 == 0 {
			logger.Log.Debug("device: drive copy progress", "copied", copied, "total", total)
		}
		return nil
	})

	return copied, err
}
