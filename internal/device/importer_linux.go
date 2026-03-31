//go:build linux

package device

import (
	"context"
	"cullsnap/internal/logger"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const maxFileCount = 50000

// ImportFromDevice imports photos from a connected device on Linux.
func ImportFromDevice(ctx context.Context, serial, baseDir string) (string, int, error) {
	cleanSerial := SanitizeSerial(serial)
	importDir := filepath.Join(baseDir, cleanSerial)

	if err := validateDestDir(importDir, baseDir); err != nil {
		return "", 0, fmt.Errorf("device: security check failed: %w", err)
	}

	if err := os.MkdirAll(importDir, 0o700); err != nil {
		return "", 0, fmt.Errorf("device: failed to create import dir: %w", err)
	}

	dev, found := lookupDeviceBySerial(serial)
	if !found {
		logger.Log.Error("device: device not found in detector state", "serial", serial)
		return "", 0, fmt.Errorf("device not found — it may have been disconnected")
	}

	logger.Log.Info("device: starting Linux import",
		"serial", serial,
		"type", dev.Type,
		"mountPath", dev.MountPath,
		"importDir", importDir,
	)

	var count int
	var err error

	switch {
	case dev.MountPath != "" && (dev.Type == "iphone" || dev.Type == "android" || dev.Type == "camera"):
		uid := os.Getuid()
		if !validateGVFSPath(dev.MountPath, uid) {
			return "", 0, fmt.Errorf("device: GVFS mount path failed security validation: %s", dev.MountPath)
		}
		count, err = importFromGVFS(ctx, dev.MountPath, importDir)

	case dev.Type == "storage" && dev.MountPath != "":
		count, err = importFromMassStorage(ctx, dev.MountPath, importDir)

	default:
		count, err = importFromGphoto2(ctx, importDir)
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

func importFromGVFS(ctx context.Context, mountPath, destDir string) (int, error) {
	dcimPath := filepath.Join(mountPath, "DCIM")
	if _, err := os.Stat(dcimPath); os.IsNotExist(err) {
		logger.Log.Info("device: no DCIM folder found", "mountPath", mountPath)
		return 0, nil
	}

	total := countDCIMFiles(dcimPath)
	logger.Log.Info("device: DCIM enumeration complete", "total", total)
	if total > maxFileCount {
		return 0, fmt.Errorf("device: too many files (%d, limit is %d)", total, maxFileCount)
	}

	copied := 0
	err := filepath.Walk(dcimPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			logger.Log.Warn("device: walk error", "path", path, "error", walkErr)
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
			logger.Log.Warn("device: failed to compute relative path", "path", path, "error", err)
			return nil
		}

		destPath := filepath.Join(destDir, relPath)

		if existingInfo, err := os.Stat(destPath); err == nil {
			if existingInfo.Size() == info.Size() {
				logger.Log.Debug("device: skipping existing file", "name", info.Name())
				copied++
				return nil
			}
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
			logger.Log.Warn("device: failed to create subdir", "error", err)
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

func importFromMassStorage(ctx context.Context, mountPath, destDir string) (int, error) {
	return importFromGVFS(ctx, mountPath, destDir)
}

func importFromGphoto2(ctx context.Context, destDir string) (int, error) {
	gphoto2Path, err := resolveSecureBinary("gphoto2")
	if err != nil {
		return 0, fmt.Errorf("gphoto2 not available: %w", err)
	}

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
	logger.Log.Info("device: gphoto2 import complete", "files", count)
	return count, nil
}

func countDCIMFiles(dcimDir string) int {
	count := 0
	_ = filepath.Walk(dcimDir, func(_ string, info os.FileInfo, err error) error {
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

func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // G304: src is validated by the caller via DCIM walk
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst) //nolint:gosec // G304: dst is validated by validateDestDir + verifyNoPathTraversal
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Close()
}
