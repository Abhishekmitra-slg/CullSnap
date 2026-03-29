package cloudsource

import (
	"cullsnap/internal/storage"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewCacheManager(t *testing.T) {
	cm := NewCacheManager("/tmp/test", nil, 512)
	if cm.baseDir != "/tmp/test" {
		t.Errorf("baseDir = %q, want /tmp/test", cm.baseDir)
	}
	if cm.maxCacheMB.Load() != 512 {
		t.Errorf("maxCacheMB = %d, want 512", cm.maxCacheMB.Load())
	}
}

func TestSetMaxCacheMB(t *testing.T) {
	cm := NewCacheManager("/tmp/test", nil, 512)
	cm.SetMaxCacheMB(1024)
	if cm.maxCacheMB.Load() != 1024 {
		t.Errorf("maxCacheMB = %d, want 1024", cm.maxCacheMB.Load())
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

func TestDeleteAlbum(t *testing.T) {
	base := t.TempDir()
	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create dir and DB record.
	dir := filepath.Join(base, "prov", "alb")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "photo.jpg"), make([]byte, 50), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCloudMirror("prov", "alb", "My Album", dir); err != nil {
		t.Fatal(err)
	}

	cm := NewCacheManager(base, store, 512)
	if err := cm.DeleteAlbum("prov", "alb"); err != nil {
		t.Fatalf("DeleteAlbum: %v", err)
	}

	// Dir should be gone.
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Error("expected dir to be removed")
	}
	// DB record should be gone.
	if _, getErr := store.GetCloudMirror("prov", "alb"); getErr == nil {
		t.Error("expected mirror record to be deleted")
	}
}

func TestDeleteAlbum_Idempotent(t *testing.T) {
	base := t.TempDir()
	dbPath := filepath.Join(base, "test.db")
	store, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cm := NewCacheManager(base, store, 100)
	// Deleting a non-existent album should not error
	if err := cm.DeleteAlbum("nonexistent", "album"); err != nil {
		t.Errorf("expected nil error for idempotent delete, got: %v", err)
	}
	// DB should still have no record
	_, getErr := store.GetCloudMirror("nonexistent", "album")
	if getErr == nil {
		t.Error("expected error getting non-existent mirror after delete")
	}
}

func TestEvictIfNeeded_NoEviction(t *testing.T) {
	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cm := NewCacheManager(t.TempDir(), store, 1024) // 1 GB limit
	evicted, err := cm.EvictIfNeeded(100, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evicted) != 0 {
		t.Errorf("expected no evictions, got %d", len(evicted))
	}
}

func TestEvictIfNeeded_EvictsOldest(t *testing.T) {
	base := t.TempDir()
	store, err := storage.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create two albums: "old" synced earlier, "new" synced later.
	// Each has 400KB of data. Limit is 1MB. Requesting 400KB more should evict old.
	for _, album := range []struct {
		id    string
		title string
	}{
		{"old-album", "Old Album"},
		{"new-album", "New Album"},
	} {
		dir := filepath.Join(base, "prov", album.id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "photo.jpg"), make([]byte, 400*1024), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveCloudMirror("prov", album.id, album.title, dir); err != nil {
			t.Fatal(err)
		}
		// Small delay to ensure different SyncedAt timestamps.
		time.Sleep(10 * time.Millisecond)
	}

	cm := NewCacheManager(base, store, 1) // 1 MB limit

	// Request 400KB more; currently using ~800KB with 1MB limit.
	// 800KB + 400KB = 1200KB > 1MB, need to evict oldest (400KB) to fit.
	evicted, err := cm.EvictIfNeeded(400*1024, "prov", "new-album")
	if err != nil {
		t.Fatalf("EvictIfNeeded: %v", err)
	}
	if len(evicted) != 1 {
		t.Fatalf("expected 1 eviction, got %d", len(evicted))
	}
	if evicted[0].AlbumTitle != "Old Album" {
		t.Errorf("expected Old Album evicted, got %q", evicted[0].AlbumTitle)
	}

	// new-album should still exist (it was excluded).
	newDir := filepath.Join(base, "prov", "new-album")
	if _, statErr := os.Stat(newDir); os.IsNotExist(statErr) {
		t.Error("new-album should not have been evicted")
	}
}
