//go:build darwin

package device

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestImportFromVolume_Success(t *testing.T) {
	srcDir := t.TempDir()
	dcimDir := filepath.Join(srcDir, "DCIM", "100APPLE")
	if err := os.MkdirAll(dcimDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"IMG_0001.JPG", "IMG_0002.JPG", "IMG_0003.HEIC"} {
		if err := os.WriteFile(filepath.Join(dcimDir, name), []byte("fake photo data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	destDir := t.TempDir()
	count, err := importFromVolume(context.Background(), srcDir, destDir)
	if err != nil {
		t.Fatalf("importFromVolume failed: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}

	copied, _ := os.ReadDir(filepath.Join(destDir, "100APPLE"))
	if len(copied) != 3 {
		t.Errorf("copied files = %d, want 3", len(copied))
	}
}

func TestImportFromVolume_NoDCIM(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()
	count, err := importFromVolume(context.Background(), srcDir, destDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestImportFromVolume_CancelContext(t *testing.T) {
	srcDir := t.TempDir()
	dcimDir := filepath.Join(srcDir, "DCIM", "100APPLE")
	os.MkdirAll(dcimDir, 0o755)
	os.WriteFile(filepath.Join(dcimDir, "IMG_0001.JPG"), []byte("data"), 0o644)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	destDir := t.TempDir()
	_, err := importFromVolume(ctx, srcDir, destDir)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestImportFromVolume_SkipExisting(t *testing.T) {
	srcDir := t.TempDir()
	dcimDir := filepath.Join(srcDir, "DCIM", "100APPLE")
	os.MkdirAll(dcimDir, 0o755)
	os.WriteFile(filepath.Join(dcimDir, "existing.jpg"), []byte("source data"), 0o644)

	destDir := t.TempDir()
	destSubDir := filepath.Join(destDir, "100APPLE")
	os.MkdirAll(destSubDir, 0o755)
	os.WriteFile(filepath.Join(destSubDir, "existing.jpg"), []byte("source data"), 0o644)

	count, err := importFromVolume(context.Background(), srcDir, destDir)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (skipped but counted)", count)
	}
}

func TestImportFromVolume_FileCountLimit(t *testing.T) {
	srcDir := t.TempDir()
	dcimDir := filepath.Join(srcDir, "DCIM", "100APPLE")
	os.MkdirAll(dcimDir, 0o755)

	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(dcimDir, fmt.Sprintf("IMG_%04d.JPG", i)), []byte("data"), 0o644)
	}

	destDir := t.TempDir()
	count, err := importFromVolume(context.Background(), srcDir, destDir)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
}
