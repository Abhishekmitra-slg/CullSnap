package hfclient

import (
	"context"
	"cullsnap/internal/logger"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const downloadChunkSize = 256 * 1024

// progressWriter is an io.Writer that calls fn after each successful Write.
type progressWriter struct {
	w  io.Writer
	fn func(int)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	if n > 0 && p.fn != nil {
		p.fn(n)
	}
	return n, err
}

// downloadOneFile streams the resource at url into destPath via destPath+".incomplete",
// resuming if a partial exists. Verifies SHA-256 (LFS) or git SHA-1 (non-LFS) against expect.
// Returns total bytes written. progressFn is called with bytes-since-last-call after each chunk.
func downloadOneFile(
	ctx context.Context,
	url, destPath string,
	expect FileEntry,
	progressFn func(int),
) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return 0, fmt.Errorf("hfclient: mkdir: %w", err)
	}

	if err := headVerify(ctx, url, expect); err != nil {
		return 0, err
	}

	partial := destPath + ".incomplete"
	var resumeOffset int64
	if info, err := os.Stat(partial); err == nil {
		resumeOffset = info.Size()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("hfclient: build GET: %w", err)
	}
	if resumeOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
	}
	cli := &http.Client{Timeout: 0}
	resp, err := cli.Do(req)
	if err != nil {
		return 0, fmt.Errorf("hfclient: GET %q: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		if resumeOffset > 0 {
			_ = os.Remove(partial)
			resumeOffset = 0
		}
	case http.StatusPartialContent:
	case http.StatusRequestedRangeNotSatisfiable:
		_ = os.Remove(partial)
		return 0, fmt.Errorf("hfclient: 416 — discarded partial, retry needed")
	case http.StatusUnauthorized:
		return 0, fmt.Errorf("hfclient: %s: 401 unauthorized", url)
	case http.StatusForbidden:
		return 0, fmt.Errorf("hfclient: %s: 403 forbidden (license gate)", url)
	case http.StatusNotFound:
		return 0, fmt.Errorf("hfclient: %s: 404 not found (manifest drift)", url)
	default:
		return 0, fmt.Errorf("hfclient: %s: status %d", url, resp.StatusCode)
	}

	flag := os.O_CREATE | os.O_WRONLY | os.O_EXCL
	if resumeOffset > 0 {
		flag = os.O_WRONLY | os.O_APPEND
	}
	f, err := os.OpenFile(partial, flag, 0o644)
	if err != nil {
		return 0, fmt.Errorf("hfclient: open partial: %w", err)
	}

	hasher := newFileHasher()
	if resumeOffset > 0 {
		existing, openErr := os.Open(partial)
		if openErr != nil {
			_ = f.Close()
			return 0, fmt.Errorf("hfclient: re-hash open: %w", openErr)
		}
		if _, copyErr := io.Copy(hasher, existing); copyErr != nil {
			_ = existing.Close()
			_ = f.Close()
			return 0, fmt.Errorf("hfclient: re-hash copy: %w", copyErr)
		}
		_ = existing.Close()
	}

	sink := io.MultiWriter(f, hasher)
	pw := &progressWriter{w: sink, fn: progressFn}
	written, copyErr := io.CopyBuffer(pw, resp.Body, make([]byte, downloadChunkSize))
	if copyErr != nil {
		_ = f.Close()
		return 0, fmt.Errorf("hfclient: stream: %w", copyErr)
	}
	if err := f.Close(); err != nil {
		return 0, fmt.Errorf("hfclient: close partial: %w", err)
	}

	total := resumeOffset + written

	if expect.IsLFS {
		got := hasher.SumHex()
		if !strings.EqualFold(got, expect.SHA256) {
			_ = os.Remove(partial)
			return 0, fmt.Errorf("hfclient: SHA-256 mismatch for %q: got %s want %s", destPath, got, expect.SHA256)
		}
	} else {
		rf, err := os.Open(partial)
		if err != nil {
			return 0, fmt.Errorf("hfclient: reopen for git-sha1: %w", err)
		}
		got, hashErr := GitBlobSHA1(rf, total)
		_ = rf.Close()
		if hashErr != nil {
			return 0, fmt.Errorf("hfclient: git-sha1: %w", hashErr)
		}
		if !strings.EqualFold(got, expect.SHA1) {
			_ = os.Remove(partial)
			return 0, fmt.Errorf("hfclient: git-SHA1 mismatch for %q: got %s want %s", destPath, got, expect.SHA1)
		}
	}
	if err := os.Rename(partial, destPath); err != nil {
		return 0, fmt.Errorf("hfclient: rename partial: %w", err)
	}
	if logger.Log != nil {
		logger.Log.Debug("hfclient: file downloaded", "path", destPath, "bytes", total)
	}
	return total, nil
}

// headVerify checks that x-linked-etag (LFS) matches expect.SHA256, or that
// Content-Length (non-LFS) matches expect.Size. Defense in depth against tree/resolve mismatch.
func headVerify(ctx context.Context, url string, expect FileEntry) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return err
	}
	cli := &http.Client{Timeout: 30 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("hfclient: HEAD %q: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hfclient: HEAD %q: status %d", url, resp.StatusCode)
	}
	if expect.IsLFS {
		got := resp.Header.Get("X-Linked-Etag")
		if !strings.EqualFold(got, expect.SHA256) {
			return fmt.Errorf("hfclient: HEAD %q x-linked-etag mismatch: got %s want %s", url, got, expect.SHA256)
		}
	} else if cl := resp.ContentLength; cl > 0 && cl != expect.Size {
		return fmt.Errorf("hfclient: HEAD %q size mismatch: got %d want %d", url, cl, expect.Size)
	}
	return nil
}
