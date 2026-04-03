package raw

import (
	"bytes"
	"context"
	"cullsnap/internal/logger"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

var bufPool = sync.Pool{
	New: func() interface{} {
		// Pre-allocate 2MB — typical JPEG preview size.
		return bytes.NewBuffer(make([]byte, 0, 2*1024*1024))
	},
}

// dcraw binary provisioning state.
var (
	dcrawPath      string
	dcrawAvailable bool
)

// dcrawTimeout is the maximum time allowed for a dcraw extraction.
const dcrawTimeout = 30 * time.Second

// Init checks for dcraw at ~/.cullsnap/bin/dcraw (or dcraw.exe on Windows)
// and attempts to download it if missing. Gracefully degrades if dcraw is
// not available — callers should check dcrawAvailable before use.
func Init() error {
	home, err := os.UserHomeDir()
	if err != nil {
		logger.Log.Debug("dcraw: cannot determine home directory", "error", err)
		return nil
	}

	binary := "dcraw"
	if runtime.GOOS == "windows" {
		binary = "dcraw.exe"
	}
	dcrawPath = filepath.Join(home, ".cullsnap", "bin", binary)

	logger.Log.Debug("dcraw: checking for binary", "path", dcrawPath)

	if _, err := os.Stat(dcrawPath); err == nil {
		dcrawAvailable = true
		logger.Log.Debug("dcraw: binary found", "path", dcrawPath)
		return nil
	}

	logger.Log.Debug("dcraw: binary not found, attempting download", "path", dcrawPath)

	if err := downloadDcraw(); err != nil {
		logger.Log.Debug("dcraw: download failed, operating without dcraw", "error", err)
		return nil
	}

	// Verify the download succeeded.
	if _, err := os.Stat(dcrawPath); err == nil {
		dcrawAvailable = true
		logger.Log.Debug("dcraw: binary installed successfully", "path", dcrawPath)
	}

	return nil
}

// downloadDcraw downloads the appropriate dcraw binary for the current platform.
// This is a placeholder that will be implemented in a future release.
func downloadDcraw() error {
	return fmt.Errorf("dcraw auto-download not yet implemented")
}

// ExtractPreviewDcraw runs dcraw -e -c on the given RAW file and returns
// the extracted JPEG preview bytes from stdout. Returns an error if dcraw
// is not available or the extraction fails.
func ExtractPreviewDcraw(path string) ([]byte, error) {
	if !dcrawAvailable {
		return nil, errors.New("dcraw: not available")
	}

	logger.Log.Debug("dcraw: extracting preview", "path", path)

	ctx, cancel := context.WithTimeout(context.Background(), dcrawTimeout)
	defer cancel()

	// dcrawPath is set exclusively by Init() from os.UserHomeDir() + hardcoded
	// relative path (e.g. ~/.cullsnap/bin/dcraw) — never from user input.
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := exec.CommandContext(ctx, filepath.Clean(dcrawPath), "-e", "-c", path)

	stdout := bufPool.Get().(*bytes.Buffer)
	stdout.Reset()
	defer bufPool.Put(stdout)

	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logger.Log.Debug("dcraw: extraction timed out", "path", path, "timeout", dcrawTimeout)
			return nil, fmt.Errorf("dcraw: extraction timed out after %v", dcrawTimeout)
		}
		logger.Log.Debug("dcraw: extraction failed", "path", path, "error", err, "stderr", stderr.String())
		return nil, fmt.Errorf("dcraw: extraction failed: %w", err)
	}

	const maxPreviewSize = 50 * 1024 * 1024 // 50MB
	if stdout.Len() > maxPreviewSize {
		return nil, fmt.Errorf("dcraw: preview output too large (%d bytes, max %d)", stdout.Len(), maxPreviewSize)
	}

	// Copy data before returning buffer to pool.
	data := make([]byte, stdout.Len())
	copy(data, stdout.Bytes())
	if len(data) < 2 {
		logger.Log.Debug("dcraw: no output data", "path", path)
		return nil, errors.New("dcraw: no preview data returned")
	}

	// Validate JPEG SOI marker.
	if data[0] != 0xFF || data[1] != 0xD8 {
		logger.Log.Debug("dcraw: output is not JPEG", "path", path, "firstBytes", fmt.Sprintf("%02x%02x", data[0], data[1]))
		return nil, errors.New("dcraw: output does not start with JPEG SOI marker")
	}

	logger.Log.Debug("dcraw: preview extracted successfully", "path", path, "size", len(data))
	return data, nil
}
