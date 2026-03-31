//go:build linux

package device

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestImportFromGVFS_Success(t *testing.T) {
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
	count, err := importFromGVFS(context.Background(), srcDir, destDir)
	if err != nil {
		t.Fatalf("importFromGVFS failed: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}

	copied, _ := os.ReadDir(filepath.Join(destDir, "100APPLE"))
	if len(copied) != 3 {
		t.Errorf("copied files = %d, want 3", len(copied))
	}
}

func TestImportFromGVFS_NoDCIM(t *testing.T) {
	srcDir := t.TempDir()
	destDir := t.TempDir()
	count, err := importFromGVFS(context.Background(), srcDir, destDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestImportFromGVFS_CancelContext(t *testing.T) {
	srcDir := t.TempDir()
	dcimDir := filepath.Join(srcDir, "DCIM", "100APPLE")
	os.MkdirAll(dcimDir, 0o755)
	os.WriteFile(filepath.Join(dcimDir, "IMG_0001.JPG"), []byte("data"), 0o644)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	destDir := t.TempDir()
	_, err := importFromGVFS(ctx, srcDir, destDir)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestImportFromMassStorage_Success(t *testing.T) {
	srcDir := t.TempDir()
	dcimDir := filepath.Join(srcDir, "DCIM", "Camera")
	os.MkdirAll(dcimDir, 0o755)
	os.WriteFile(filepath.Join(dcimDir, "photo1.jpg"), []byte("data"), 0o644)
	os.WriteFile(filepath.Join(dcimDir, "photo2.jpg"), []byte("data"), 0o644)

	destDir := t.TempDir()
	count, err := importFromMassStorage(context.Background(), srcDir, destDir)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestCountDCIMFiles(t *testing.T) {
	srcDir := t.TempDir()
	dcimDir := filepath.Join(srcDir, "DCIM")
	sub1 := filepath.Join(dcimDir, "100APPLE")
	sub2 := filepath.Join(dcimDir, "101APPLE")
	os.MkdirAll(sub1, 0o755)
	os.MkdirAll(sub2, 0o755)
	os.WriteFile(filepath.Join(sub1, "a.jpg"), []byte("data"), 0o644)
	os.WriteFile(filepath.Join(sub1, "b.jpg"), []byte("data"), 0o644)
	os.WriteFile(filepath.Join(sub2, "c.jpg"), []byte("data"), 0o644)

	count := countDCIMFiles(filepath.Join(srcDir, "DCIM"))
	if count != 3 {
		t.Errorf("countDCIMFiles = %d, want 3", count)
	}
}

func TestCountDCIMFiles_Missing(t *testing.T) {
	count := countDCIMFiles("/nonexistent/DCIM")
	if count != 0 {
		t.Errorf("countDCIMFiles(nonexistent) = %d, want 0", count)
	}
}

func TestImportFromGVFS_SkipExisting(t *testing.T) {
	srcDir := t.TempDir()
	dcimDir := filepath.Join(srcDir, "DCIM", "100APPLE")
	os.MkdirAll(dcimDir, 0o755)
	os.WriteFile(filepath.Join(dcimDir, "existing.jpg"), []byte("source data"), 0o644)

	destDir := t.TempDir()
	destSubDir := filepath.Join(destDir, "100APPLE")
	os.MkdirAll(destSubDir, 0o755)
	// Write a file with same size to dest
	os.WriteFile(filepath.Join(destSubDir, "existing.jpg"), []byte("source data"), 0o644)

	count, err := importFromGVFS(context.Background(), srcDir, destDir)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (skipped but counted)", count)
	}
}
