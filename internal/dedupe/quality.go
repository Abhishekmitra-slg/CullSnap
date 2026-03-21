package dedupe

import (
	"bytes"
	"context"
	"cullsnap/internal/raw"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
)

// CalculateLaplacianVariance computes the variance of the laplacian for an image.
// High variance = sharp edges (in-focus). Low variance = smooth (blurry).
// A common pure-Go implementation of OpenCV's Laplacian Variance.
func CalculateLaplacianVariance(imgPath string) (variance float64, err error) {
	ext := strings.ToLower(filepath.Ext(imgPath))
	var img image.Image

	if raw.IsRAWExt(ext) {
		previewBytes, extractErr := raw.ExtractPreview(imgPath)
		if extractErr != nil {
			return 0, fmt.Errorf("RAW preview extraction failed: %w", extractErr)
		}
		decoded, decErr := jpeg.Decode(bytes.NewReader(previewBytes))
		if decErr != nil {
			return 0, fmt.Errorf("failed to decode RAW preview: %w", decErr)
		}
		img = decoded
	} else {
		file, openErr := os.Open(imgPath)
		if openErr != nil {
			return 0, fmt.Errorf("failed to open image: %w", openErr)
		}
		defer func() {
			if cerr := file.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}()

		decoded, _, decErr := image.Decode(file)
		if decErr != nil {
			return 0, fmt.Errorf("failed to decode image: %w", decErr)
		}
		img = decoded
	}

	// 1. Resize/Downscale to speed up processing substantially.
	// Sharpness is largely retained at medium resolutions (e.g., 500x500).
	// We do not need 24-megapixel calculations for relative sharpness grouping.
	thumb := imaging.Resize(img, 500, 0, imaging.NearestNeighbor)

	// 2. Grayscale conversion via imaging library
	grayImg := imaging.Grayscale(thumb)

	bounds := grayImg.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Ensure the image isn't too small for a 3x3 kernel
	if width < 3 || height < 3 {
		return 0, fmt.Errorf("image too small for laplacian kernel")
	}

	// 3. Apply 3x3 Laplacian Kernel:
	// [ 0  1  0 ]
	// [ 1 -4  1 ]
	// [ 0  1  0 ]
	laplacian := make([]float64, 0, (width-2)*(height-2))
	var sum float64

	for y := 1; y < height-1; y++ {
		for x := 1; x < width-1; x++ {
			// Get grayscale intensities (0-255)
			// Since Grayscale returns Gray or NRGBA, we extract the Y (luminance) channel safely.
			valT := colorToLuminance(grayImg.At(x, y-1))
			valB := colorToLuminance(grayImg.At(x, y+1))
			valL := colorToLuminance(grayImg.At(x-1, y))
			valR := colorToLuminance(grayImg.At(x+1, y))
			valC := colorToLuminance(grayImg.At(x, y))

			// Kernel convolution
			lVal := valT + valB + valL + valR - (4 * valC)

			laplacian = append(laplacian, lVal)
			sum += lVal
		}
	}

	// 4. Calculate Variance
	pixelCount := float64(len(laplacian))
	mean := sum / pixelCount

	var sqDiffSum float64
	for _, lVal := range laplacian {
		diff := lVal - mean
		sqDiffSum += diff * diff
	}

	variance = sqDiffSum / pixelCount
	return variance, nil
}

// Extract luminance from any color quickly
func colorToLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	// Convert from 16-bit alpha-premultiplied scalar to 8-bit (0-255)
	// Weighted luminance: 0.299*R + 0.587*G + 0.114*B
	luminance := (0.299 * float64(r>>8)) + (0.587 * float64(g>>8)) + (0.114 * float64(b>>8))
	return luminance
}

// FindBestPhoto updates the duplicate group by selecting the one with highest sharpness variance.
func FindBestPhotos(ctx context.Context, groups []*DuplicateGroup, progressCallback func(current, total int, message string)) error {
	totalGroups := len(groups)
	for i, group := range groups {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if progressCallback != nil {
			progressCallback(i+1, totalGroups, "Selecting best quality photos...")
		}

		if len(group.Photos) <= 1 {
			// Unique photo is inherently the "best"
			if len(group.Photos) == 1 {
				group.Photos[0].IsUnique = true
			}
			continue
		}

		var bestPhoto *PhotoInfo
		var maxVariance float64 = -1

		for _, photo := range group.Photos {
			if photo.Hash == nil {
				continue
			}

			variance, err := CalculateLaplacianVariance(photo.Path)
			if err != nil {
				continue // Skip gracefully
			}

			if variance > maxVariance {
				maxVariance = variance
				bestPhoto = photo
			}
		}

		// Mark the best photo as the true "Unique" representative, others are duplicates
		for _, photo := range group.Photos {
			if photo == bestPhoto && bestPhoto != nil {
				photo.IsUnique = true
			} else {
				photo.IsUnique = false
			}
		}

		// Fallback: If all failed sharpness detection, mark the first one as representative
		if bestPhoto == nil && len(group.Photos) > 0 {
			group.Photos[0].IsUnique = true
			for i := 1; i < len(group.Photos); i++ {
				group.Photos[i].IsUnique = false
			}
		}
	}
	return nil
}
