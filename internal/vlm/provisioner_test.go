package vlm

import (
	"os"
	"path/filepath"
	"testing"
)

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
