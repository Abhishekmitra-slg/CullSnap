package dedupe

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func createTestImage(path string, width, height int, c color.RGBA) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Add a pattern based on coordinated and color bits
			// to ensure different colors/images have distinctly different perceptual hashes.
			r := uint8(c.R ^ uint8(x))
			g := uint8(c.G ^ uint8(y))
			b := uint8(c.B ^ uint8((x+y)/2))
			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: 100})
}

func TestFindDuplicates(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dedupe-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a red image, and an exact duplicate
	redColor := color.RGBA{255, 0, 0, 255}
	if err := createTestImage(filepath.Join(tmpDir, "red1.jpg"), 100, 100, redColor); err != nil {
		t.Fatalf("Failed to create red1.jpg: %v", err)
	}
	if err := createTestImage(filepath.Join(tmpDir, "red2.jpg"), 100, 100, redColor); err != nil {
		t.Fatalf("Failed to create red2.jpg: %v", err)
	}

	// Create a completely different blue image
	blueColor := color.RGBA{0, 0, 255, 255}
	if err := createTestImage(filepath.Join(tmpDir, "blue1.jpg"), 100, 100, blueColor); err != nil {
		t.Fatalf("Failed to create blue1.jpg: %v", err)
	}

	// Find duplicates with a similarity threshold of 5
	groups, err := FindDuplicates(context.Background(), tmpDir, 5, "", nil)
	if err != nil {
		t.Fatalf("FindDuplicates failed: %v", err)
	}

	// We expect 2 groups: one for red (2 items) and one for blue (1 item)
	if len(groups) != 2 {
		t.Fatalf("Expected 2 duplicate groups, got %d", len(groups))
	}

	foundRedGroup := false
	foundBlueGroup := false

	for _, g := range groups {
		if len(g.Photos) == 2 {
			foundRedGroup = true
			if g.Photos[0].IsUnique || g.Photos[1].IsUnique {
				t.Errorf("Photos in duplicate group should not be marked as unique")
			}
		} else if len(g.Photos) == 1 {
			foundBlueGroup = true
			if !g.Photos[0].IsUnique {
				t.Errorf("Isolated photo should be marked as unique")
			}
		}
	}

	if !foundRedGroup {
		t.Errorf("Did not find the expected duplicate group with 2 identical red images")
	}
	if !foundBlueGroup {
		t.Errorf("Did not find the expected unique group with 1 blue image")
	}
}
