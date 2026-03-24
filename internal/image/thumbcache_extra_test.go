package image

import (
	"cullsnap/internal/logger"
	"testing"
	"time"
)

func TestThumbCache_CacheDir(t *testing.T) {
	logger.Init("/dev/null") //nolint:errcheck
	tmpDir := t.TempDir()
	tc, err := NewThumbCache(tmpDir, false)
	if err != nil {
		t.Fatalf("NewThumbCache failed: %v", err)
	}
	if tc.CacheDir() != tmpDir {
		t.Errorf("CacheDir() = %q, want %q", tc.CacheDir(), tmpDir)
	}
}

func TestThumbCache_CacheKey_Deterministic(t *testing.T) {
	logger.Init("/dev/null") //nolint:errcheck
	tc := &ThumbCache{cacheDir: "/tmp/test"}
	mod := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	k1 := tc.cacheKey("/some/path.jpg", mod)
	k2 := tc.cacheKey("/some/path.jpg", mod)
	if k1 != k2 {
		t.Errorf("cacheKey not deterministic: %q vs %q", k1, k2)
	}
}

func TestThumbCache_CacheKey_DifferentPaths(t *testing.T) {
	logger.Init("/dev/null") //nolint:errcheck
	tc := &ThumbCache{cacheDir: "/tmp/test"}
	mod := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	k1 := tc.cacheKey("/path1.jpg", mod)
	k2 := tc.cacheKey("/path2.jpg", mod)
	if k1 == k2 {
		t.Error("different paths should produce different cache keys")
	}
}

func TestThumbCache_CacheKey_DifferentModTimes(t *testing.T) {
	logger.Init("/dev/null") //nolint:errcheck
	tc := &ThumbCache{cacheDir: "/tmp/test"}
	k1 := tc.cacheKey("/same/path.jpg", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	k2 := tc.cacheKey("/same/path.jpg", time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))
	if k1 == k2 {
		t.Error("different mod times should produce different cache keys")
	}
}

func TestThumbCache_GetCachedPath_Miss(t *testing.T) {
	logger.Init("/dev/null") //nolint:errcheck
	tc, err := NewThumbCache(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	result := tc.GetCachedPath("/nonexistent/photo.jpg", time.Now())
	if result != "" {
		t.Errorf("expected empty string for cache miss, got %q", result)
	}
}
