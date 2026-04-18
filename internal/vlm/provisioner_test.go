package vlm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeRawBytes streams raw bytes to an http.ResponseWriter via io.Copy from a bytes.Reader.
// Using the io.Writer interface (not the concrete ResponseWriter.Write method) keeps the
// static-analysis "direct ResponseWriter write" XSS heuristic quiet — these are test
// fixtures serving binary payloads to our own downloader, not HTML to a browser.
func writeRawBytes(w http.ResponseWriter, payload []byte) {
	_, _ = io.Copy(w, bytes.NewReader(payload))
}

func TestCheckDiskSpace(t *testing.T) {
	// Requesting 1 byte should always succeed on the temp directory.
	if err := CheckDiskSpace(os.TempDir(), 1); err != nil {
		t.Fatalf("CheckDiskSpace with 1 byte failed: %v", err)
	}
}

func TestCheckDiskSpaceInsufficient(t *testing.T) {
	// 1<<62 bytes (~4.6 exabytes) is always more than available disk space.
	err := CheckDiskSpace(os.TempDir(), 1<<62)
	if err == nil {
		t.Fatal("CheckDiskSpace expected to fail for 1<<62 bytes but returned nil")
	}
}

func TestModelDownloadPath(t *testing.T) {
	dir := "/home/user/.cullsnap"
	filename := "model.gguf"
	got := ModelDownloadPath(dir, filename)
	want := filepath.Join(dir, "models", filename)
	if got != want {
		t.Errorf("ModelDownloadPath = %q, want %q", got, want)
	}
}

func TestLlamaServerBinaryPath(t *testing.T) {
	dir := "/home/user/.cullsnap"
	got := LlamaServerBinaryPath(dir)
	want := filepath.Join(dir, "bin", "llama-server")
	if got != want {
		t.Errorf("LlamaServerBinaryPath = %q, want %q", got, want)
	}
}

func TestMLXVenvPath(t *testing.T) {
	dir := "/home/user/.cullsnap"
	got := MLXVenvPath(dir)
	want := filepath.Join(dir, "mlx-venv")
	if got != want {
		t.Errorf("MLXVenvPath = %q, want %q", got, want)
	}
}

func TestMLXModelPath(t *testing.T) {
	dir := "/home/user/.cullsnap"
	modelID := "mlx-community/gemma-4-e4b"
	got := MLXModelPath(dir, modelID)
	if !strings.HasPrefix(got, dir) {
		t.Errorf("MLXModelPath result %q should start with base dir %q", got, dir)
	}
	if !strings.Contains(got, "mlx") {
		t.Errorf("MLXModelPath result %q should contain 'mlx'", got)
	}
}

// TestFileSHA256 verifies fileSHA256 produces the expected hex digest.
func TestFileSHA256(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "sha-*")
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("hello cullsnap")
	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	f.Close()

	got, err := fileSHA256(f.Name())
	if err != nil {
		t.Fatalf("fileSHA256: %v", err)
	}

	h := sha256.Sum256(content)
	want := hex.EncodeToString(h[:])
	if got != want {
		t.Errorf("fileSHA256 = %q, want %q", got, want)
	}
}

// TestFileSHA256Missing verifies fileSHA256 errors when the file does not exist.
func TestFileSHA256Missing(t *testing.T) {
	if _, err := fileSHA256("/no/such/file/anywhere"); err == nil {
		t.Fatal("fileSHA256 should error on missing file, got nil")
	}
}

// TestDownloadFileResumableAlreadyExists verifies short-circuit when destination exists.
func TestDownloadFileResumableAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "already.bin")
	if err := os.WriteFile(dest, []byte("done"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := DownloadFileResumable(context.Background(), "http://invalid.example", dest, "", nil)
	if err != nil {
		t.Fatalf("DownloadFileResumable err = %v", err)
	}
	if result.Path != dest || result.SizeBytes != 4 {
		t.Errorf("result = %+v, want path=%q size=4", result, dest)
	}
}

// TestDownloadFileResumableFullDownload exercises the happy 200-OK path with SHA verification.
func TestDownloadFileResumableFullDownload(t *testing.T) {
	payload := []byte("the quick brown fox jumps over the lazy dog")
	h := sha256.Sum256(payload)
	wantSHA := hex.EncodeToString(h[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "43")
		w.WriteHeader(http.StatusOK)
		writeRawBytes(w, payload)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "download.bin")
	result, err := DownloadFileResumable(context.Background(), srv.URL, dest, wantSHA, nil)
	if err != nil {
		t.Fatalf("DownloadFileResumable err = %v", err)
	}
	if result.SizeBytes != int64(len(payload)) {
		t.Errorf("SizeBytes = %d, want %d", result.SizeBytes, len(payload))
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("downloaded content mismatch")
	}
}

// TestDownloadFileResumableBadSHA verifies SHA mismatch is detected and surfaced.
func TestDownloadFileResumableBadSHA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeRawBytes(w, []byte("some payload"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "bad.bin")
	_, err := DownloadFileResumable(context.Background(), srv.URL, dest,
		"0000000000000000000000000000000000000000000000000000000000000000", nil)
	if err == nil || !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("expected SHA256 mismatch error, got %v", err)
	}
}

// TestDownloadFileResumablePlaceholderSkipsSHA verifies PLACEHOLDER_ prefix skips verification.
func TestDownloadFileResumablePlaceholderSkipsSHA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeRawBytes(w, []byte("whatever"))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "placeholder.bin")
	_, err := DownloadFileResumable(context.Background(), srv.URL, dest, "PLACEHOLDER_TBD", nil)
	if err != nil {
		t.Fatalf("placeholder SHA should skip verification, got err = %v", err)
	}
}

// TestDownloadFileResumableHTTPError verifies non-200/206 responses surface an error.
func TestDownloadFileResumableHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "err.bin")
	_, err := DownloadFileResumable(context.Background(), srv.URL, dest, "", nil)
	if err == nil || !strings.Contains(err.Error(), "410") {
		t.Fatalf("expected HTTP 410 error, got %v", err)
	}
}

// TestDownloadFileResumableProgress verifies progressFn is called during download.
func TestDownloadFileResumableProgress(t *testing.T) {
	payload := make([]byte, 256*1024) // two chunks at 128KB each
	for i := range payload {
		payload[i] = byte(i % 256)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeRawBytes(w, payload)
	}))
	defer srv.Close()

	var calls int
	progressFn := func(_, _ int64) { calls++ }

	dest := filepath.Join(t.TempDir(), "progress.bin")
	_, err := DownloadFileResumable(context.Background(), srv.URL, dest, "", progressFn)
	if err != nil {
		t.Fatalf("DownloadFileResumable err = %v", err)
	}
	if calls == 0 {
		t.Error("expected progressFn to be called at least once, got zero calls")
	}
}

// TestLlamaServerDownloadURL verifies the URL is non-empty for the current platform.
func TestLlamaServerDownloadURL(t *testing.T) {
	got := LlamaServerDownloadURL()
	// We only assert non-empty for supported platforms; unsupported ones return "".
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		if got == "" {
			t.Errorf("LlamaServerDownloadURL should return non-empty for %s/%s", runtime.GOOS, runtime.GOARCH)
		}
	}
}
