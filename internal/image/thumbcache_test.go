package image

import (
	"cullsnap/internal/logger"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func createTestJpeg(path string) error {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: 100})
}

func TestThumbCache(t *testing.T) {
	// Initialize logger to discard to avoid nil pointer panics in logger.Log operations
	logger.Init("/dev/null")

	tmpDir := t.TempDir()
	tc, err := NewThumbCache(tmpDir)
	if err != nil {
		t.Fatalf("NewThumbCache failed: %v", err)
	}

	srcDir := t.TempDir()
	srcImg := filepath.Join(srcDir, "test.jpg")
	if err := createTestJpeg(srcImg); err != nil {
		t.Fatalf("Failed to create test jpeg: %v", err)
	}

	modTime := time.Now()

	// 1. Should not exist yet
	if cached := tc.GetCachedPath(srcImg, modTime); cached != "" {
		t.Errorf("Expected GetCachedPath to return empty, got %s", cached)
	}

	// 2. Generate
	thumbPath, err := tc.GenerateThumbnail(srcImg, modTime)
	if err != nil {
		t.Fatalf("GenerateThumbnail failed: %v", err)
	}
	if !strings.Contains(thumbPath, tmpDir) {
		t.Errorf("Expected thumbnail path to be inside temp dir, but got: %s", thumbPath)
	}

	// 3. Check exist
	if cached := tc.GetCachedPath(srcImg, modTime); cached != thumbPath {
		t.Errorf("Expected GetCachedPath to return %s, got %s", thumbPath, cached)
	}

	// 4. Generate Batch (Parallel)
	items := []struct {
		Path    string
		ModTime time.Time
	}{
		{Path: srcImg, ModTime: modTime},
	}

	var progressCount int
	batchResult := tc.GenerateBatch(items, 2, func(completed, total int) {
		progressCount = completed
	})

	if len(batchResult) != 1 {
		t.Errorf("Expected 1 result in batch generation, got %d", len(batchResult))
	}
	if batchResult[srcImg] != thumbPath {
		t.Errorf("Batch result path mismatch: got %s, want %s", batchResult[srcImg], thumbPath)
	}
	if progressCount != 1 {
		t.Errorf("Expected progress callback to report 1 completion, got %d", progressCount)
	}
}
