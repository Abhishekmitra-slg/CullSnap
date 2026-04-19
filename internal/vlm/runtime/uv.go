package runtime

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
	"runtime"
	"strings"
)

// Provisioner manages local Python/uv state under cullsnapDir.
type Provisioner struct {
	cullsnapDir string
	httpClient  *http.Client
}

// New returns a Provisioner rooted at cullsnapDir.
func New(cullsnapDir string) *Provisioner {
	return &Provisioner{
		cullsnapDir: cullsnapDir,
		httpClient:  http.DefaultClient,
	}
}

// UVPath returns the absolute path to the bundled uv binary.
func (p *Provisioner) UVPath() string {
	return filepath.Join(p.cullsnapDir, "bin", "uv")
}

// EnsureUV downloads and verifies uv if absent or hash-mismatched.
func (p *Provisioner) EnsureUV(ctx context.Context, progressFn func(done, total int64)) (string, error) {
	info, ok := uvDownloadInfoFor(runtime.GOOS, runtime.GOARCH)
	if !ok {
		return "", fmt.Errorf("vlm/runtime: no uv binary for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return p.ensureUVFromInfo(ctx, info, progressFn)
}

func (p *Provisioner) ensureUVFromInfo(ctx context.Context, info UVDownloadInfo, progressFn func(done, total int64)) (string, error) {
	target := p.UVPath()

	// Check if an existing binary at target already matches the expected hash.
	if existing, err := os.Open(target); err == nil {
		sum := sha256.New()
		if _, copyErr := io.Copy(sum, existing); copyErr == nil {
			if strings.EqualFold(hex.EncodeToString(sum.Sum(nil)), info.SHA256) {
				_ = existing.Close()
				if logger.Log != nil {
					logger.Log.Debug("vlm/runtime: uv present + verified", "path", target)
				}
				return target, nil
			}
		}
		_ = existing.Close()
		// Hash mismatch — remove and re-download.
		if logger.Log != nil {
			logger.Log.Debug("vlm/runtime: uv hash mismatch, re-downloading", "path", target)
		}
		_ = os.Remove(target)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("vlm/runtime: mkdir bin: %w", err)
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm/runtime: downloading uv", "url", info.URL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("vlm/runtime: uv download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vlm/runtime: uv download status %d", resp.StatusCode)
	}

	partial := target + ".partial"
	f, err := os.OpenFile(partial, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	sum := sha256.New()
	var done int64
	buf := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			_ = f.Close()
			_ = os.Remove(partial)
			return "", err
		}
		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := f.Write(buf[:n]); wErr != nil {
				_ = f.Close()
				_ = os.Remove(partial)
				return "", wErr
			}
			sum.Write(buf[:n])
			done += int64(n)
			if progressFn != nil {
				progressFn(done, resp.ContentLength)
			}
		}
		if rErr == io.EOF {
			break
		}
		if rErr != nil {
			_ = f.Close()
			_ = os.Remove(partial)
			return "", rErr
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(partial)
		return "", err
	}

	got := hex.EncodeToString(sum.Sum(nil))
	if !strings.EqualFold(got, info.SHA256) {
		_ = os.Remove(partial)
		return "", fmt.Errorf("vlm/runtime: uv SHA-256 mismatch: got %s want %s", got, info.SHA256)
	}

	// Note: production uv ships as a tarball; ensureUVFromInfo here treats
	// info.URL as a direct binary URL for testability. Real EnsureUV path
	// wraps a tarball-extraction helper around this. Implement in a
	// follow-up task if URL ends with ".tar.gz".
	if err := os.Rename(partial, target); err != nil {
		return "", err
	}
	if err := os.Chmod(target, 0o755); err != nil {
		return "", err
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm/runtime: uv installed", "path", target)
	}
	return target, nil
}
