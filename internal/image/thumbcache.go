package image

import (
	"crypto/md5"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"sync"
	"time"

	"cullsnap/internal/logger"
)

// ThumbCache manages a disk-based thumbnail cache.
// Thumbnails are stored in a user-private directory with restricted permissions.
type ThumbCache struct {
	cacheDir string
	mu       sync.Mutex
}

// NewThumbCache creates a ThumbCache at ~/.cullsnap/thumbs/ with 0700 permissions.
// Only the owner can read/write/list the cache directory.
func NewThumbCache() (*ThumbCache, error) {
	cacheBase, err := os.UserCacheDir()
	if err != nil {
		// Fallback to home dir
		home, err2 := os.UserHomeDir()
		if err2 != nil {
			return nil, fmt.Errorf("cannot determine cache directory: %w", err)
		}
		cacheBase = home
	}

	cacheDir := filepath.Join(cacheBase, "CullSnap", "thumbs")
	// 0700 = owner read/write/execute only — no other users can access
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create thumbnail cache: %w", err)
	}

	logger.Log.Info("Thumbnail cache initialized", "cacheDir", cacheDir)
	return &ThumbCache{cacheDir: cacheDir}, nil
}

// cacheKey generates a unique filename for a given image path and modification time.
// Uses MD5 of path+modTime to ensure cache invalidation when files change.
func (tc *ThumbCache) cacheKey(path string, modTime time.Time) string {
	h := md5.Sum([]byte(fmt.Sprintf("%s_%d", path, modTime.UnixNano())))
	return fmt.Sprintf("%x.jpg", h)
}

// GetCachedPath returns the thumbnail path if it exists in cache.
// Returns empty string if no cache hit.
func (tc *ThumbCache) GetCachedPath(path string, modTime time.Time) string {
	thumbPath := filepath.Join(tc.cacheDir, tc.cacheKey(path, modTime))
	if _, err := os.Stat(thumbPath); err == nil {
		return thumbPath
	}
	return ""
}

// GenerateThumbnail creates a 300px-wide JPEG thumbnail and stores it in the cache.
// Returns the path to the cached thumbnail.
// File is written with 0600 permissions (owner read/write only).
func (tc *ThumbCache) GenerateThumbnail(path string, modTime time.Time) (string, error) {
	thumbPath := filepath.Join(tc.cacheDir, tc.cacheKey(path, modTime))

	// Check if already cached (race-safe)
	if _, err := os.Stat(thumbPath); err == nil {
		return thumbPath, nil
	}

	// Generate thumbnail using existing GetThumbnail (EXIF extraction → fallback resize)
	thumb, err := GetThumbnail(path)
	if err != nil {
		return "", fmt.Errorf("thumbnail generation failed for %s: %w", filepath.Base(path), err)
	}

	// Write to temp file then rename for atomicity (no partial files visible)
	tmpPath := thumbPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", fmt.Errorf("failed to create thumbnail file: %w", err)
	}

	if err := jpeg.Encode(f, thumb, &jpeg.Options{Quality: 80}); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to encode thumbnail: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to close thumbnail file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, thumbPath); err != nil {
		os.Remove(tmpPath)
		return "", err
	}

	logger.Log.Debug("Thumbnail generated", "file", filepath.Base(path), "thumb", filepath.Base(thumbPath))
	return thumbPath, nil
}

// ThumbResult holds the result of a parallel thumbnail generation.
type ThumbResult struct {
	OriginalPath  string
	ThumbnailPath string
	Error         error
}

// GenerateBatch generates thumbnails for multiple images in parallel using a worker pool.
// numWorkers controls parallelism. progressFn is called for each completed item.
// This function is safe for concurrent use and cleans up goroutines properly.
func (tc *ThumbCache) GenerateBatch(
	items []struct {
		Path    string
		ModTime time.Time
	},
	numWorkers int,
	progressFn func(completed, total int),
) map[string]string {
	result := make(map[string]string, len(items))
	var resultMu sync.Mutex
	var completed int

	// Work channel — buffered to avoid blocking producers
	work := make(chan struct {
		Path    string
		ModTime time.Time
	}, len(items))

	var wg sync.WaitGroup

	// Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range work {
				// Check cache first
				thumbPath := tc.GetCachedPath(item.Path, item.ModTime)
				if thumbPath == "" {
					// Generate thumbnail
					var err error
					thumbPath, err = tc.GenerateThumbnail(item.Path, item.ModTime)
					if err != nil {
						// Skip failed thumbnails — grid will use original path
						thumbPath = ""
					}
				}

				resultMu.Lock()
				if thumbPath != "" {
					result[item.Path] = thumbPath
				} else {
					logger.Log.Warn("Thumbnail failed, will use original", "file", filepath.Base(item.Path))
				}
				completed++
				c := completed
				resultMu.Unlock()

				if progressFn != nil {
					progressFn(c, len(items))
				}
			}
		}()
	}

	// Feed work
	for _, item := range items {
		work <- item
	}
	close(work)

	// Wait for all workers to complete
	wg.Wait()

	logger.Log.Info("Batch thumbnail generation complete", "total", len(items), "cached", len(result), "failed", len(items)-len(result))
	return result
}
