package dedupe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFile_Roundtrip(t *testing.T) {
	src := filepath.Join(t.TempDir(), "source.txt")
	dst := filepath.Join(t.TempDir(), "dest.txt")

	data := []byte("test file content for copy roundtrip")
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Errorf("copied data mismatch: got %q, want %q", got, data)
	}
}

func TestCopyFile_MissingSrc(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "dest.txt")
	err := copyFile("/nonexistent/file.txt", dst)
	if err == nil {
		t.Error("expected error for non-existent source")
	}
}

func TestCopyFile_InvalidDest(t *testing.T) {
	src := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := copyFile(src, "/nonexistent/dir/dest.txt")
	if err == nil {
		t.Error("expected error for invalid dest directory")
	}
}

func TestCopyFile_LargeFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "large.bin")
	dst := filepath.Join(t.TempDir(), "large_copy.bin")

	// 1 MB file
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(src, data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(data) {
		t.Errorf("file size mismatch: got %d, want %d", len(got), len(data))
	}
}
