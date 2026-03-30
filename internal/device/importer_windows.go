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

// importScript is the PowerShell script that imports photos from an iPhone/iPad
// via MTP using the Windows Shell.Application COM object. It reads a JSON object
// with a "destDir" field from stdin, enumerates DCIM subfolders, and copies each
// file to the destination directory. Progress is reported as NDJSON lines on stdout.
//
// Security: this script is a compile-time constant and is never modified at runtime.
// Parameters are passed via stdin JSON, never interpolated into the script text.
const importScript = `$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$inputJson = [Console]::In.ReadToEnd()
$params = $inputJson | ConvertFrom-Json
$destDir = $params.destDir

$shell = New-Object -ComObject Shell.Application

$myComputer = $shell.Namespace(0x11)
$device = $myComputer.Items() | Where-Object {
    $_.Name -match 'iPhone|iPad'
} | Select-Object -First 1

if (-not $device) {
    @{ type = 'error'; code = 'no_device'; message = 'No iPhone or iPad found. Connect your device via USB.' } |
        ConvertTo-Json -Compress
    exit 1
}

$storage = $device.GetFolder.Items() |
    Where-Object { $_.Name -eq 'Internal Storage' }
if (-not $storage) {
    @{ type = 'error'; code = 'not_trusted';
       message = 'Cannot access device storage. Please unlock your iPhone and tap Trust when prompted, then try again.' } |
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

// maxFileCount is a safety limit to prevent runaway imports from consuming
// excessive disk space or time. If the device reports more files than this,
// the import is aborted before any files are copied.
const maxFileCount = 50000

// ImportFromDevice imports photos from a connected iPhone/iPad via MTP on Windows.
// It uses a PowerShell script that leverages the Shell.Application COM object to
// enumerate and copy files from the device's DCIM folder.
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

	logger.Log.Info("device: starting MTP import via PowerShell",
		"serial", serial,
		"cleanSerial", cleanSerial,
		"importDir", importDir,
	)

	// Marshal parameters as JSON for stdin.
	type importParams struct {
		DestDir string `json:"destDir"`
	}
	paramsJSON, err := json.Marshal(importParams{DestDir: importDir})
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

// countFilesRecursive counts all non-directory entries under dir, recursively.
// Returns 0 if the directory does not exist or cannot be read.
func countFilesRecursive(dir string) int {
	count := 0
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count
}

// userFriendlyError maps PowerShell error codes to user-friendly messages.
func userFriendlyError(evt ProgressEvent) string {
	switch evt.Code {
	case "no_device":
		return "No iPhone or iPad found. Connect your device via USB and try again."
	case "not_trusted":
		return "Please unlock your iPhone and tap 'Trust' when prompted, then try again."
	case "no_dcim":
		return "No photos found on device."
	default:
		return evt.Message
	}
}
