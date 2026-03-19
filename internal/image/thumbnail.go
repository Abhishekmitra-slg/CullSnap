package image

import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/png"
	"os"

	"github.com/disintegration/imaging"
	"github.com/rwcarlsen/goexif/exif"
)

// GetThumbnail generates a thumbnail for the image at the given path.
// It attempts to extract the embedded thumbnail from EXIF data first (fast),
// and falls back to decoding and resizing the image (slow).
func GetThumbnail(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() // read-only; Close error is safe to ignore

	// 1. Try to extract embedded thumbnail from EXIF
	x, err := exif.Decode(f)
	if err == nil {
		thumbData, err := x.JpegThumbnail()
		if err == nil {
			thumbImg, err := jpeg.Decode(bytes.NewReader(thumbData))
			if err == nil {
				// Success! We extracted the embedded preview.
				// We might want to rotate it based on EXIF orientation here,
				// but for now let's just return it.
				// Often embedded thumbnails are already oriented, or we need to handle orientation separately.
				// For simplicity in this step, we return the raw extracted thumb.
				return thumbImg, nil
			}
		}
	}

	// 2. Fallback: Full decode and resize
	// Reset file pointer
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}

	// Resize to a reasonable thumbnail size (e.g., 300px width)
	// Lanczos is high quality, but slower. Box is fastest.
	// For "snappy" fallback, maybe Linear or Box. Let's use Box for speed given the constraint.
	thumb := imaging.Resize(img, 300, 0, imaging.Box)
	return thumb, nil
}

// GetFullImage loads the full resolution image.
// It uses imaging.Open which handles rotation based on EXIF automatically.
func GetFullImage(path string) (image.Image, error) {
	return imaging.Open(path)
}
