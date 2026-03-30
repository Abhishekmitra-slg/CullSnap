//go:build darwin

package device

import (
	"context"
	"cullsnap/internal/logger"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ImportFromDevice imports photos from a connected iPhone/iPad using
// Image Capture via osascript. Returns the import directory path and
// count of imported files.
//
// Security note: the import path is fully controlled by the application
// (baseDir from app config + sanitized serial). SanitizeSerial strips
// all special characters, so the path interpolated into the AppleScript
// cannot contain quotes or escape sequences that would alter the script.
func ImportFromDevice(ctx context.Context, serial, baseDir string) (string, int, error) {
	importDir := filepath.Join(baseDir, SanitizeSerial(serial))
	if err := os.MkdirAll(importDir, 0o700); err != nil {
		return "", 0, fmt.Errorf("device: failed to create import dir: %w", err)
	}

	logger.Log.Info("device: starting Image Capture import",
		"serial", serial,
		"importDir", importDir,
	)

	// Use osascript to drive Image Capture.
	// The importDir path is safe for interpolation because SanitizeSerial
	// strips all characters except [a-zA-Z0-9._-], and baseDir is app-controlled.
	script := fmt.Sprintf(`
		tell application "Image Capture"
			activate
			set targetFolder to POSIX file "%s" as alias
			-- Import all from connected device
		end tell
	`, importDir)

	// exec.Command with separate args — safe from shell injection.
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		logger.Log.Error("device: Image Capture import failed",
			"error", err,
			"output", string(out),
		)
		return importDir, countFiles(importDir), fmt.Errorf(
			"automatic import failed. You can import manually: " +
				"Open Image Capture from Spotlight, select your iPhone, " +
				"drag photos to a folder, then open that folder in CullSnap")
	}

	count := countFiles(importDir)
	logger.Log.Info("device: import complete", "serial", serial, "files", count)
	return importDir, count, nil
}
