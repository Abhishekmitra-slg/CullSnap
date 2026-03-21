package raw

import (
	"crypto/md5"
	"cullsnap/internal/logger"
	"fmt"
	"os"
	"path/filepath"
)

var previewCacheDir string

// InitPreviewCache creates the preview cache directory under ~/.cullsnap/previews.
func InitPreviewCache() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	previewCacheDir = filepath.Join(home, ".cullsnap", "previews")
	return os.MkdirAll(previewCacheDir, 0o755)
}

// GetCachedPreview returns a previously cached preview JPEG for the given RAW file path.
func GetCachedPreview(path string) ([]byte, error) {
	cachePath := previewCachePath(path)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}
	logger.Log.Debug("raw: preview cache hit", "path", path)
	return data, nil
}

// CachePreview writes a preview JPEG to the disk cache atomically (temp + rename).
func CachePreview(path string, data []byte) error {
	cachePath := previewCachePath(path)
	tmpPath := cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, cachePath)
}

func previewCachePath(path string) string {
	info, err := os.Stat(path)
	var modTime string
	if err == nil {
		modTime = info.ModTime().String()
	}
	// nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-md5
	hash := md5.Sum([]byte(path + modTime)) //nolint:gosec // MD5 used for cache key, not security
	return filepath.Join(previewCacheDir, fmt.Sprintf("%x.jpg", hash))
}
