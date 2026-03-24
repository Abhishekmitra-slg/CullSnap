package raw

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestInitPreviewCache(t *testing.T) {
	// Save and restore the global
	origDir := previewCacheDir
	defer func() { previewCacheDir = origDir }()

	err := InitPreviewCache()
	if err != nil {
		t.Fatalf("InitPreviewCache failed: %v", err)
	}
	if previewCacheDir == "" {
		t.Fatal("previewCacheDir should be set after InitPreviewCache")
	}
	info, err := os.Stat(previewCacheDir)
	if err != nil {
		t.Fatalf("previewCacheDir does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("previewCacheDir should be a directory")
	}
}

func TestCachePreview_Roundtrip(t *testing.T) {
	origDir := previewCacheDir
	defer func() { previewCacheDir = origDir }()

	previewCacheDir = t.TempDir()

	// Create a dummy file so previewCachePath can stat it
	tmpFile := filepath.Join(t.TempDir(), "test.cr2")
	if err := os.WriteFile(tmpFile, []byte("fake raw"), 0o644); err != nil {
		t.Fatal(err)
	}

	data := []byte("fake JPEG preview data for testing")
	if err := CachePreview(tmpFile, data); err != nil {
		t.Fatalf("CachePreview failed: %v", err)
	}

	loaded, err := GetCachedPreview(tmpFile)
	if err != nil {
		t.Fatalf("GetCachedPreview failed: %v", err)
	}
	if !bytes.Equal(loaded, data) {
		t.Errorf("cached data mismatch: got %d bytes, want %d bytes", len(loaded), len(data))
	}
}

func TestGetCachedPreview_CacheMiss(t *testing.T) {
	origDir := previewCacheDir
	defer func() { previewCacheDir = origDir }()

	previewCacheDir = t.TempDir()

	_, err := GetCachedPreview("/nonexistent/file.cr2")
	if err == nil {
		t.Error("expected error for cache miss")
	}
}

func TestPreviewCachePath_Deterministic(t *testing.T) {
	origDir := previewCacheDir
	defer func() { previewCacheDir = origDir }()

	previewCacheDir = "/tmp/test-cache"

	p1 := previewCachePath("/some/file.cr2")
	p2 := previewCachePath("/some/file.cr2")
	if p1 != p2 {
		t.Errorf("previewCachePath not deterministic: %q vs %q", p1, p2)
	}
}

func TestPreviewCachePath_DifferentFiles(t *testing.T) {
	origDir := previewCacheDir
	defer func() { previewCacheDir = origDir }()

	previewCacheDir = "/tmp/test-cache"

	p1 := previewCachePath("/some/file1.cr2")
	p2 := previewCachePath("/some/file2.cr2")
	if p1 == p2 {
		t.Error("different files should have different cache paths")
	}
}
