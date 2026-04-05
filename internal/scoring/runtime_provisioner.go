//go:build !windows

package scoring

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"cullsnap/internal/logger"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	ortVersion         = "1.23.0"
	ortDownloadTimeout = 5 * time.Minute
	ortLibDirPerm      = 0o755
	ortLibFilePerm     = 0o755
)

// onnxRuntimeLibName returns the platform-specific library filename.
func onnxRuntimeLibName() string {
	if runtime.GOOS == "darwin" {
		return "libonnxruntime.dylib"
	}
	return "libonnxruntime.so"
}

// onnxRuntimeDownloadURL returns the download URL for the current platform.
func onnxRuntimeDownloadURL() (string, error) {
	base := "https://github.com/microsoft/onnxruntime/releases/download/v" + ortVersion

	switch {
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return base + "/onnxruntime-osx-arm64-" + ortVersion + ".tgz", nil
	case runtime.GOOS == "darwin" && runtime.GOARCH == "amd64":
		return base + "/onnxruntime-osx-x86_64-" + ortVersion + ".tgz", nil
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return base + "/onnxruntime-linux-x64-" + ortVersion + ".tgz", nil
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

// ProvisionONNXRuntime downloads the ONNX Runtime shared library if not already present.
// Returns the path to the library file.
func ProvisionONNXRuntime(ctx context.Context, cacheDir string) (string, error) {
	libDir := filepath.Join(cacheDir, "lib")
	libPath := filepath.Join(libDir, onnxRuntimeLibName())

	// Check if already provisioned.
	if _, err := os.Stat(libPath); err == nil {
		logger.Log.Debug("scoring: ONNX runtime already provisioned", "path", libPath)
		return libPath, nil
	}

	if err := os.MkdirAll(libDir, ortLibDirPerm); err != nil {
		return "", fmt.Errorf("create lib directory: %w", err)
	}

	url, err := onnxRuntimeDownloadURL()
	if err != nil {
		return "", err
	}

	logger.Log.Info("scoring: downloading ONNX Runtime",
		"version", ortVersion,
		"platform", runtime.GOOS+"/"+runtime.GOARCH,
		"url", url,
	)

	dlCtx, cancel := context.WithTimeout(ctx, ortDownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download ONNX Runtime: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // HTTP response body

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download ONNX Runtime: HTTP %d", resp.StatusCode)
	}

	// Extract the shared library from the .tgz archive.
	if err := extractLibFromTgz(resp.Body, libDir, onnxRuntimeLibName()); err != nil {
		return "", fmt.Errorf("extract ONNX Runtime: %w", err)
	}

	// Verify the extracted file exists.
	if _, err := os.Stat(libPath); err != nil {
		return "", fmt.Errorf("ONNX Runtime library not found after extraction: %w", err)
	}

	logger.Log.Info("scoring: ONNX Runtime provisioned", "path", libPath, "version", ortVersion)
	return libPath, nil
}

// extractLibFromTgz extracts the shared library from a .tgz archive.
// It looks for files matching the target library name in any lib/ directory.
func extractLibFromTgz(r io.Reader, destDir, libName string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gzr.Close() //nolint:errcheck // decompression cleanup

	tr := tar.NewReader(gzr)
	found := false

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar reader: %w", err)
		}

		// Look for the library file in any path (e.g., onnxruntime-osx-arm64-1.23.0/lib/libonnxruntime.dylib).
		base := filepath.Base(header.Name)
		if base != libName {
			// Also match versioned variants (libonnxruntime.1.23.0.dylib, libonnxruntime.so.1.23.0).
			if !strings.Contains(base, "onnxruntime") || !strings.Contains(header.Name, "/lib/") {
				continue
			}
			// Skip symlinks and non-regular files — we want the actual library.
			if header.Typeflag != tar.TypeReg {
				continue
			}
			// Skip providers_shared and other ancillary libs.
			if strings.Contains(base, "providers") {
				continue
			}
		}

		if header.Typeflag == tar.TypeSymlink {
			continue // Skip symlinks, keep looking for the real file.
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		destPath := filepath.Join(destDir, libName)
		f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, ortLibFilePerm) //nolint:gosec // trusted path
		if err != nil {
			return fmt.Errorf("create lib file: %w", err)
		}

		// Cap at 100MB to mitigate zip bomb attacks (ONNX Runtime lib is ~10-15MB).
		const maxLibSize = 100 * 1024 * 1024
		written, err := io.CopyN(f, tr, maxLibSize) //nolint:gosec // trusted archive from GitHub releases
		if err == io.EOF {
			err = nil // CopyN returns EOF when source is smaller than limit — expected.
		}
		closeErr := f.Close()
		if err != nil {
			return fmt.Errorf("write lib file: %w", err)
		}
		if closeErr != nil {
			return fmt.Errorf("close lib file: %w", closeErr)
		}

		found = true
		logger.Log.Info("scoring: extracted ONNX Runtime library",
			"file", base,
			"size", written,
			"dest", destPath,
		)
		break
	}

	if !found {
		return fmt.Errorf("library %s not found in archive", libName)
	}

	return nil
}
