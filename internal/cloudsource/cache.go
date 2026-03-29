package cloudsource

import (
	"cullsnap/internal/logger"
	"cullsnap/internal/storage"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// CachedAlbum represents a cloud album cached on disk.
type CachedAlbum struct {
	ProviderID string    `json:"provider_id"`
	AlbumID    string    `json:"album_id"`
	AlbumTitle string    `json:"album_title"`
	LocalPath  string    `json:"-"`
	SizeBytes  int64     `json:"size_bytes"`
	FileCount  int       `json:"file_count"`
	SyncedAt   time.Time `json:"synced_at"`
}

// CacheStats summarises overall cloud cache usage.
type CacheStats struct {
	TotalBytes int64 `json:"total_bytes"`
	AlbumCount int   `json:"album_count"`
	LimitBytes int64 `json:"limit_bytes"`
}

// EvictedAlbum records an album removed during LRU eviction.
type EvictedAlbum struct {
	AlbumTitle string `json:"album_title"`
	SizeBytes  int64  `json:"size_bytes"`
}

// CacheManager provides disk-usage tracking, listing and LRU eviction for
// cloud album mirrors stored under baseDir.
type CacheManager struct {
	baseDir    string
	store      *storage.SQLiteStore
	maxCacheMB int
}

// NewCacheManager creates a CacheManager.
func NewCacheManager(baseDir string, store *storage.SQLiteStore, maxCacheMB int) *CacheManager {
	return &CacheManager{
		baseDir:    baseDir,
		store:      store,
		maxCacheMB: maxCacheMB,
	}
}

// SetMaxCacheMB updates the cache size limit.
func (c *CacheManager) SetMaxCacheMB(mb int) {
	c.maxCacheMB = mb
	logger.Log.Debug("cache: updated max cache size", "maxCacheMB", mb)
}

// AlbumDiskUsage returns the total bytes and file count for a cached album directory.
// Returns 0, 0, nil if the directory does not exist.
func (c *CacheManager) AlbumDiskUsage(providerID, albumID string) (bytes int64, files int, err error) {
	dir := filepath.Join(c.baseDir, SanitizeID(providerID), SanitizeID(albumID))

	info, statErr := os.Stat(dir)
	if os.IsNotExist(statErr) || (statErr == nil && !info.IsDir()) {
		return 0, 0, nil
	}
	if statErr != nil {
		return 0, 0, fmt.Errorf("cache: stat album dir: %w", statErr)
	}

	err = filepath.Walk(dir, func(_ string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable entries
		}
		if !fi.IsDir() {
			bytes += fi.Size()
			files++
		}
		return nil
	})
	if err != nil {
		return 0, 0, fmt.Errorf("cache: walk album dir: %w", err)
	}
	return bytes, files, nil
}

// ListCachedAlbums returns all cached albums with their disk usage, sorted
// by SyncedAt descending (most recent first).
func (c *CacheManager) ListCachedAlbums() ([]CachedAlbum, error) {
	mirrors, err := c.store.ListCloudMirrors()
	if err != nil {
		return nil, fmt.Errorf("cache: list mirrors: %w", err)
	}

	albums := make([]CachedAlbum, 0, len(mirrors))
	for _, m := range mirrors {
		sz, cnt, diskErr := c.AlbumDiskUsage(m.ProviderID, m.AlbumID)
		if diskErr != nil {
			logger.Log.Warn("cache: disk usage error", "provider", m.ProviderID, "album", m.AlbumID, "error", diskErr)
			continue
		}
		albums = append(albums, CachedAlbum{
			ProviderID: m.ProviderID,
			AlbumID:    m.AlbumID,
			AlbumTitle: m.AlbumTitle,
			LocalPath:  m.LocalPath,
			SizeBytes:  sz,
			FileCount:  cnt,
			SyncedAt:   m.SyncedAt,
		})
	}

	sort.Slice(albums, func(i, j int) bool {
		return albums[i].SyncedAt.After(albums[j].SyncedAt)
	})

	return albums, nil
}

// GetCacheStats returns aggregate cache statistics.
func (c *CacheManager) GetCacheStats() (CacheStats, error) {
	albums, err := c.ListCachedAlbums()
	if err != nil {
		return CacheStats{}, err
	}

	var total int64
	for _, a := range albums {
		total += a.SizeBytes
	}

	return CacheStats{
		TotalBytes: total,
		AlbumCount: len(albums),
		LimitBytes: int64(c.maxCacheMB) * 1024 * 1024,
	}, nil
}
