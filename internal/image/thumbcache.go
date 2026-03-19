package image

import (
	"crypto/md5"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cullsnap/internal/logger"
	"cullsnap/internal/video"
)

// ThumbCache manages a disk-based thumbnail cache.
// Thumbnails are stored in a user-private directory with restricted permissions.
type ThumbCache struct {
	cacheDir string
}

// NewThumbCache creates a ThumbCache at the given directory with 0700 permissions.
// Pass AppConfig.CacheDir to ensure the cache location matches what the media server uses.
func NewThumbCache(cacheDir string) (*ThumbCache, error) {
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
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

	ext := strings.ToLower(filepath.Ext(path))
	isVideo := false
	switch ext {
	case ".mp4", ".mov", ".webm", ".mkv", ".avi":
		isVideo = true
	}

	if isVideo {
		// Extract thumbnail directly to the cache file (atomic rename not as easy with external commands,
		// but we can extract to temp and rename)
		tmpPath := thumbPath + ".tmp"
		if err := video.ExtractThumbnail(path, tmpPath); err != nil {
			_ = os.Remove(tmpPath) // best-effort cleanup
			return "", fmt.Errorf("video thumbnail extraction failed for %s: %w", filepath.Base(path), err)
		}

		if err := os.Rename(tmpPath, thumbPath); err != nil {
			_ = os.Remove(tmpPath) // best-effort cleanup
			return "", err
		}
	} else {
		// Generate photo thumbnail using existing GetThumbnail (EXIF extraction → fallback resize)
		thumb, err := GetThumbnail(path)
		if err != nil {
			return "", fmt.Errorf("thumbnail generation failed for %s: %w", filepath.Base(path), err)
		}

		// Write to temp file then rename for atomicity (no partial files visible)
		tmpPath := thumbPath + ".tmp"
		f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return "", fmt.Errorf("failed to create thumbnail file: %w", err)
		}

		if err := jpeg.Encode(f, thumb, &jpeg.Options{Quality: 80}); err != nil {
			_ = f.Close()          // best-effort close on encode failure
			_ = os.Remove(tmpPath) // best-effort cleanup
			return "", fmt.Errorf("failed to encode thumbnail: %w", err)
		}

		if err := f.Close(); err != nil {
			_ = os.Remove(tmpPath) // best-effort cleanup
			return "", fmt.Errorf("failed to close thumbnail file: %w", err)
		}

		// Atomic rename
		if err := os.Rename(tmpPath, thumbPath); err != nil {
			_ = os.Remove(tmpPath) // best-effort cleanup
			return "", err
		}
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
