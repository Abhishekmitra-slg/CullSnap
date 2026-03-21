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

func createBlurryImage(path string, width, height int) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Solid color = 0 variance (blurry/smooth)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{100, 100, 100, 255})
		}
	}
	return saveImg(path, img)
}

func createSharpImage(path string, width, height int) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Checkerboard = high variance (sharp edges)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if (x/10+y/10)%2 == 0 {
				img.Set(x, y, color.RGBA{255, 255, 255, 255})
			} else {
				img.Set(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}
	return saveImg(path, img)
}

func saveImg(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: 100})
}

func TestCalculateLaplacianVariance(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dedupe-quality-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	blurryPath := filepath.Join(tmpDir, "blurry.jpg")
	sharpPath := filepath.Join(tmpDir, "sharp.jpg")

	if err := createBlurryImage(blurryPath, 100, 100); err != nil {
		t.Fatalf("Failed to create blurry image: %v", err)
	}
	if err := createSharpImage(sharpPath, 100, 100); err != nil {
		t.Fatalf("Failed to create sharp image: %v", err)
	}

	blurryVar, err := CalculateLaplacianVariance(blurryPath, "")
	if err != nil {
		t.Fatalf("CalculateLaplacianVariance failed for blurry: %v", err)
	}

	sharpVar, err := CalculateLaplacianVariance(sharpPath, "")
	if err != nil {
		t.Fatalf("CalculateLaplacianVariance failed for sharp: %v", err)
	}

	if sharpVar <= blurryVar {
		t.Errorf("Expected sharp image variance (%f) to be > blurry image variance (%f)", sharpVar, blurryVar)
	}
}

func TestFindBestPhotos(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dedupe-best-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	blurryPath := filepath.Join(tmpDir, "blurry.jpg")
	sharpPath := filepath.Join(tmpDir, "sharp.jpg")

	if err := createBlurryImage(blurryPath, 100, 100); err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}
	if err := createSharpImage(sharpPath, 100, 100); err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}

	// Create a duplicate group with one sharp and one blurry photo.
	// Hash is required by FindBestPhotos.
	blurryHash, _ := hashImage(blurryPath)
	sharpHash, _ := hashImage(sharpPath)

	group := &DuplicateGroup{
		Photos: []*PhotoInfo{
			{Path: blurryPath, Hash: blurryHash, GroupID: blurryPath},
			{Path: sharpPath, Hash: sharpHash, GroupID: blurryPath},
		},
	}

	err = FindBestPhotos(context.Background(), []*DuplicateGroup{group}, "", nil)
	if err != nil {
		t.Fatalf("FindBestPhotos failed: %v", err)
	}

	if !group.Photos[1].IsUnique {
		t.Errorf("Expected sharp photo to be selected as best (Unique)")
	}
	if group.Photos[0].IsUnique {
		t.Errorf("Expected blurry photo to be marked as duplicate (!Unique)")
	}
}
