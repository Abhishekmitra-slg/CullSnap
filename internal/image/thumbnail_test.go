package image

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"
)

func createDummyImage(path string) error {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for x := 0; x < 100; x++ {
		for y := 0; y < 100; y++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, nil)
}

func TestGetThumbnail(t *testing.T) {
	dummyPath := "dummy.jpg"
	if err := createDummyImage(dummyPath); err != nil {
		t.Fatalf("Failed to create dummy image: %v", err)
	}
	defer os.Remove(dummyPath)

	thumb, err := GetThumbnail(dummyPath)
	if err != nil {
		t.Fatalf("GetThumbnail failed: %v", err)
	}

	if thumb == nil {
		t.Fatal("Thumbnail is nil")
	}

	// Since dummy image has no EXIF, it should hit the fallback which resizes to 300 width
	// But our dummy is 100x100, and imaging.Resize with 300, 0 might upscale or keep it.
	// Actually imaging.Resize(img, 300, 0) sets width to 300 and preserves aspect ratio.
	// Check if it returned a valid image.
	bounds := thumb.Bounds()
	if bounds.Dx() == 0 || bounds.Dy() == 0 {
		t.Error("Thumbnail has zero dimensions")
	}
}
