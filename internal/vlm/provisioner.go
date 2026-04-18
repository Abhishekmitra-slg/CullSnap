package vlm

import (
	"context"
	"crypto/sha256"
	"cullsnap/internal/logger"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
)

const (
	modelSubdir    = "models"
	binSubdir      = "bin"
	mlxVenvSubdir  = "mlx-venv"
	mlxModelSubdir = "mlx-models"

	// bufferBytes is added to required disk space as a safety margin (500 MB).
	bufferBytes = 500 * 1024 * 1024

	downloadChunkSize = 256 * 1024 // 256 KB
)

// ModelDownloadPath returns the full path for a model file inside cullsnapDir.
func ModelDownloadPath(cullsnapDir, filename string) string {
	return filepath.Join(cullsnapDir, modelSubdir, filename)
}

// LlamaServerBinaryPath returns the full path to the llama-server binary inside cullsnapDir.
func LlamaServerBinaryPath(cullsnapDir string) string {
	return filepath.Join(cullsnapDir, binSubdir, "llama-server")
}

// MLXVenvPath returns the full path to the MLX virtual-environment directory.
func MLXVenvPath(cullsnapDir string) string {
	return filepath.Join(cullsnapDir, mlxVenvSubdir)
}

// MLXModelPath returns the full path to a named MLX model directory.
func MLXModelPath(cullsnapDir, modelName string) string {
	return filepath.Join(cullsnapDir, mlxModelSubdir, modelName)
}

// CheckDiskSpace returns an error if path has fewer than requiredBytes + bufferBytes of free space.
func CheckDiskSpace(path string, requiredBytes int64) error {
	usage, err := disk.Usage(path)
	if err != nil {
		if logger.Log != nil {
			logger.Log.Warn("vlm: provisioner: disk usage query failed", "path", path, "err", err)
		}
		return fmt.Errorf("vlm: disk usage query failed for %q: %w", path, err)
	}

	needed := requiredBytes + bufferBytes
	if int64(usage.Free) < needed {
		if logger.Log != nil {
			logger.Log.Warn("vlm: provisioner: insufficient disk space",
				"path", path,
				"free_bytes", usage.Free,
				"needed_bytes", needed,
			)
		}
		return fmt.Errorf(
			"vlm: insufficient disk space at %q: need %d bytes (%d + %d buffer), have %d bytes free",
			path, needed, requiredBytes, int64(bufferBytes), int64(usage.Free),
		)
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: provisioner: disk space ok",
			"path", path,
			"free_bytes", usage.Free,
			"required_bytes", requiredBytes,
		)
	}
	return nil
}

// DownloadResult holds metadata about a completed download.
type DownloadResult struct {
	Path      string
	SizeBytes int64
	Duration  time.Duration
}

// DownloadFileResumable downloads url to destPath, resuming from a .partial file if present.
// SHA256 verification is skipped when expectedSHA256 is empty or starts with "PLACEHOLDER_".
// progressFn is called with (downloaded, total) after each chunk; total may be -1 if unknown.
func DownloadFileResumable(
	ctx context.Context,
	url, destPath, expectedSHA256 string,
	progressFn func(downloaded, total int64),
) (*DownloadResult, error) {
	if logger.Log != nil {
		logger.Log.Debug("vlm: provisioner: download requested", "url", url, "dest", destPath)
	}

	// Ensure destination directory exists.
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return nil, fmt.Errorf("vlm: mkdir for download dest: %w", err)
	}

	// If the final file already exists, return immediately without re-downloading.
	if info, err := os.Stat(destPath); err == nil {
		if logger.Log != nil {
			logger.Log.Debug("vlm: provisioner: file already exists, skipping download",
				"path", destPath,
				"size", info.Size(),
			)
		}
		return &DownloadResult{Path: destPath, SizeBytes: info.Size()}, nil
	}

	partialPath := destPath + ".partial"

	// Determine resume offset from any existing partial file.
	var resumeOffset int64
	if info, err := os.Stat(partialPath); err == nil {
		resumeOffset = info.Size()
		if logger.Log != nil {
			logger.Log.Debug("vlm: provisioner: resuming download",
				"partial", partialPath,
				"offset", resumeOffset,
			)
		}
	}

	start := time.Now()

	// Build the HTTP request, adding a Range header when resuming.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("vlm: build request: %w", err)
	}
	if resumeOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vlm: http get %q: %w", url, err)
	}
	defer resp.Body.Close()

	// If server returned 200 (not 206 Partial Content), restart from scratch.
	if resp.StatusCode == http.StatusOK && resumeOffset > 0 {
		if logger.Log != nil {
			logger.Log.Debug("vlm: provisioner: server returned 200 instead of 206, restarting download")
		}
		resumeOffset = 0
		if err := os.Remove(partialPath); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("vlm: remove stale partial: %w", err)
		}
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("vlm: unexpected HTTP status %d for %q", resp.StatusCode, url)
	}

	// Total size for progress reporting (-1 when unknown).
	var totalBytes int64 = -1
	if resp.ContentLength > 0 {
		totalBytes = resumeOffset + resp.ContentLength
	}

	// Open partial file for writing (append when resuming, create otherwise).
	flag := os.O_CREATE | os.O_WRONLY
	if resumeOffset > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}

	f, err := os.OpenFile(partialPath, flag, 0o644)
	if err != nil {
		return nil, fmt.Errorf("vlm: open partial file: %w", err)
	}

	var downloaded int64 = resumeOffset
	buf := make([]byte, downloadChunkSize)

	for {
		if ctx.Err() != nil {
			f.Close()
			return nil, ctx.Err()
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				f.Close()
				return nil, fmt.Errorf("vlm: write to partial file: %w", writeErr)
			}
			downloaded += int64(n)
			if progressFn != nil {
				progressFn(downloaded, totalBytes)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			f.Close()
			return nil, fmt.Errorf("vlm: read response body: %w", readErr)
		}
	}

	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("vlm: close partial file: %w", err)
	}

	// SHA256 verification (skip for empty or placeholder checksums).
	skipVerify := expectedSHA256 == "" || strings.HasPrefix(expectedSHA256, "PLACEHOLDER_")
	if !skipVerify {
		if logger.Log != nil {
			logger.Log.Debug("vlm: provisioner: verifying SHA256", "path", partialPath)
		}
		got, hashErr := fileSHA256(partialPath)
		if hashErr != nil {
			return nil, fmt.Errorf("vlm: sha256 hash failed: %w", hashErr)
		}
		if !strings.EqualFold(got, expectedSHA256) {
			_ = os.Remove(partialPath)
			return nil, fmt.Errorf("vlm: SHA256 mismatch for %q: got %s, want %s", destPath, got, expectedSHA256)
		}
		if logger.Log != nil {
			logger.Log.Debug("vlm: provisioner: SHA256 verified", "hash", got)
		}
	}

	// Atomic rename from .partial to final destination.
	if err := os.Rename(partialPath, destPath); err != nil {
		return nil, fmt.Errorf("vlm: rename partial to final: %w", err)
	}

	info, err := os.Stat(destPath)
	if err != nil {
		return nil, fmt.Errorf("vlm: stat final file: %w", err)
	}

	elapsed := time.Since(start)
	if logger.Log != nil {
		logger.Log.Debug("vlm: provisioner: download complete",
			"path", destPath,
			"size_bytes", info.Size(),
			"duration_ms", elapsed.Milliseconds(),
		)
	}

	return &DownloadResult{
		Path:      destPath,
		SizeBytes: info.Size(),
		Duration:  elapsed,
	}, nil
}

// fileSHA256 returns the lowercase hex SHA256 digest of the file at path.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
