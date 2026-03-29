package cloudsource

import (
	"cullsnap/internal/logger"
	"cullsnap/internal/storage"
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
