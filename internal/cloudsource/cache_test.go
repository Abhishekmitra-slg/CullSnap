package cloudsource

import (
	"cullsnap/internal/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestNewCacheManager(t *testing.T) {
	cm := NewCacheManager("/tmp/test", nil, 512)
	if cm.baseDir != "/tmp/test" {
		t.Errorf("baseDir = %q, want /tmp/test", cm.baseDir)
	}
	if cm.maxCacheMB != 512 {
		t.Errorf("maxCacheMB = %d, want 512", cm.maxCacheMB)
	}
}

func TestSetMaxCacheMB(t *testing.T) {
	cm := NewCacheManager("/tmp/test", nil, 512)
	cm.SetMaxCacheMB(1024)
	if cm.maxCacheMB != 1024 {
		t.Errorf("maxCacheMB = %d, want 1024", cm.maxCacheMB)
	}
}

func TestAlbumDiskUsage_NonExistent(t *testing.T) {
	cm := NewCacheManager(t.TempDir(), nil, 512)
	bytes, files, err := cm.AlbumDiskUsage("nope", "nope")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bytes != 0 || files != 0 {
		t.Errorf("expected 0/0, got %d/%d", bytes, files)
	}
}

func TestAlbumDiskUsage_WithFiles(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "provider", "album1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create two 100-byte files.
	for _, name := range []string{"a.jpg", "b.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), make([]byte, 100), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cm := NewCacheManager(base, nil, 512)
	bytes, files, err := cm.AlbumDiskUsage("provider", "album1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bytes != 200 {
		t.Errorf("bytes = %d, want 200", bytes)
	}
	if files != 2 {
		t.Errorf("files = %d, want 2", files)
	}
}

func TestGetCacheStats_Empty(t *testing.T) {
	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cm := NewCacheManager(t.TempDir(), store, 1024)
	stats, err := cm.GetCacheStats()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalBytes != 0 {
		t.Errorf("TotalBytes = %d, want 0", stats.TotalBytes)
	}
	if stats.AlbumCount != 0 {
		t.Errorf("AlbumCount = %d, want 0", stats.AlbumCount)
	}
	if stats.LimitBytes != 1024*1024*1024 {
		t.Errorf("LimitBytes = %d, want %d", stats.LimitBytes, 1024*1024*1024)
	}
}
