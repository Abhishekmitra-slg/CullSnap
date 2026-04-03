//go:build darwin

package device

import (
	"context"
	"cullsnap/internal/logger"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ImportFromDevice imports photos from a connected device on macOS.
// It determines the import strategy based on the device's type and mount path.
func ImportFromDevice(ctx context.Context, serial, baseDir string) (string, int, error) {
	cleanSerial := SanitizeSerial(serial)
	importDir := filepath.Join(baseDir, cleanSerial)

	if err := validateDestDir(importDir, baseDir); err != nil {
		return "", 0, fmt.Errorf("device: security check failed: %w", err)
	}

	if err := os.MkdirAll(importDir, 0o700); err != nil {
		return "", 0, fmt.Errorf("device: failed to create import dir: %w", err)
	}

	// Look up the device in the detector's state.
	dev, found := lookupDeviceBySerial(serial)

	logger.Log.Info("device: starting macOS import",
		"serial", serial,
		"type", dev.Type,
		"mountPath", dev.MountPath,
		"found", found,
		"importDir", importDir,
	)

	var count int
	var err error

	switch {
	case dev.Type == "storage" && dev.MountPath != "":
		// Mass storage / SD card: direct file copy from volume
		count, err = importFromVolume(ctx, dev.MountPath, importDir)

	case dev.Type == "android":
		// Android: try gphoto2, fall back to Image Capture guidance
		count, err = importFromGphoto2Darwin(ctx, importDir)
		if err != nil {
			logger.Log.Warn("device: gphoto2 import failed, suggesting PTP mode", "error", err)
			return "", 0, fmt.Errorf("to import from Android: switch USB mode to 'Photo Transfer (PTP)' on your phone, or install gphoto2 (brew install gphoto2)")
		}

	default:
		// iPhone, camera, or unknown: use Image Capture
		count, err = importFromImageCapture(ctx, serial, importDir)
	}

	if err != nil {
		logger.Log.Error("device: import failed",
			"serial", serial,
			"type", dev.Type,
			"error", err,
			"partialCount", count,
		)
		verifyNoPathTraversal(importDir)
		return importDir, count, err
	}

	removed := verifyNoPathTraversal(importDir)
	if removed > 0 {
		logger.Log.Warn("device: removed files that escaped import directory", "removed", removed)
	}

	finalCount := countFilesRecursive(importDir)
	logger.Log.Info("device: import finished",
		"serial", serial,
		"type", dev.Type,
		"fileCount", finalCount,
	)
	return importDir, finalCount, nil
}

// importFromImageCapture uses osascript to drive Image Capture for PTP devices.
func importFromImageCapture(ctx context.Context, serial, importDir string) (int, error) {
	logger.Log.Info("device: starting Image Capture import",
		"serial", serial,
		"importDir", importDir,
	)

	script := fmt.Sprintf(`
		tell application "Image Capture"
			activate
			set targetFolder to POSIX file "%s" as alias
		end tell
	`, importDir)

	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logger.Log.Error("device: Image Capture import failed",
			"error", err,
			"output", string(out),
		)
		return countFiles(importDir), fmt.Errorf(
			"automatic import failed. You can import manually: " +
				"Open Image Capture from Spotlight, select your device, " +
				"drag photos to a folder, then open that folder in CullSnap")
	}

	count := countFiles(importDir)
	logger.Log.Info("device: Image Capture import complete", "serial", serial, "files", count)
	return count, nil
}

// importFromVolume copies files from a mounted volume's DCIM folder.
func importFromVolume(ctx context.Context, mountPath, destDir string) (int, error) {
	dcimPath := filepath.Join(mountPath, "DCIM")
	if _, err := os.Stat(dcimPath); os.IsNotExist(err) {
		logger.Log.Info("device: no DCIM folder found", "mountPath", mountPath)
		return 0, nil
	}

	total := countFilesRecursive(dcimPath)
	logger.Log.Info("device: DCIM enumeration complete", "total", total)
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

		relPath, err := filepath.Rel(dcimPath, path)
		if err != nil {
			return nil
		}

		destPath := filepath.Join(destDir, relPath)

		if existingInfo, err := os.Stat(destPath); err == nil {
			if existingInfo.Size() == info.Size() {
				copied++
				return nil
			}
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
			return nil
		}

		if err := copyFile(path, destPath); err != nil {
			logger.Log.Warn("device: failed to copy file", "name", info.Name(), "error", err)
			return nil
		}

		copied++
		if copied%100 == 0 {
			logger.Log.Debug("device: copy progress", "copied", copied, "total", total)
		}

		return nil
	})

	return copied, err
}

// importFromGphoto2Darwin uses gphoto2 to import photos from a PTP/MTP device on macOS.
func importFromGphoto2Darwin(ctx context.Context, destDir string) (int, error) {
	gphoto2Path, err := resolveSecureBinary("gphoto2")
	if err != nil {
		return 0, fmt.Errorf("gphoto2 not available: %w", err)
	}

	// Kill macOS PTPCamera daemon to release the device.
	killCtx, killCancel := context.WithTimeout(ctx, 5*time.Second)
	_ = exec.CommandContext(killCtx, "killall", "PTPCamera").Run()
	killCancel()

	logger.Log.Info("device: importing via gphoto2", "binary", gphoto2Path, "destDir", destDir)

	importCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(importCtx, gphoto2Path, // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
		"-P",
		"--folder", "/DCIM",
		"--filename", "%f.%C",
	)
	cmd.Dir = destDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Log.Error("device: gphoto2 import failed",
			"error", err,
			"output", string(output),
		)
		count := countFilesRecursive(destDir)
		if count > 0 {
			return count, fmt.Errorf("device: gphoto2 partially completed (%d files): %w", count, err)
		}
		return 0, fmt.Errorf("device: gphoto2 import failed: %w", err)
	}

	count := countFilesRecursive(destDir)
	return count, nil
}
