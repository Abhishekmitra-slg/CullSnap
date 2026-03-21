package dedupe

import (
	"bytes"
	"context"
	"crypto/md5"
	"cullsnap/internal/logger"
	"cullsnap/internal/raw"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/disintegration/imaging"
	"golang.org/x/sync/errgroup"
)

// Scoring weights for multi-factor quality assessment.
const (
	weightSharpness = 0.50
	weightExposure  = 0.25
	weightNoise     = 0.15
	weightContrast  = 0.10
)

// loadQualityImage loads an image for quality analysis, trying the thumbnail cache first.
func loadQualityImage(imgPath string, thumbnailDir string) (image.Image, error) {
	var img image.Image

	// Try cached thumbnail first (local SSD, fast)
	if thumbnailDir != "" {
		if info, statErr := os.Stat(imgPath); statErr == nil {
			h := md5.Sum([]byte(fmt.Sprintf("%s_%d", imgPath, info.ModTime().UnixNano()))) //nolint:gosec // MD5 used for cache key, not cryptography
			thumbPath := filepath.Join(thumbnailDir, fmt.Sprintf("%x.jpg", h))
			if thumbFile, openErr := os.Open(thumbPath); openErr == nil {
				if decoded, decErr := jpeg.Decode(thumbFile); decErr == nil {
					img = decoded
				}
				_ = thumbFile.Close()
			}
		}
	}

	// Fallback: read original file (RAW/JPEG)
	if img == nil {
		ext := strings.ToLower(filepath.Ext(imgPath))

		if raw.IsRAWExt(ext) {
			previewBytes, extractErr := raw.ExtractPreview(imgPath)
			if extractErr != nil {
				return nil, fmt.Errorf("RAW preview extraction failed: %w", extractErr)
			}
			decoded, decErr := jpeg.Decode(bytes.NewReader(previewBytes))
			if decErr != nil {
				return nil, fmt.Errorf("failed to decode RAW preview: %w", decErr)
			}
			img = decoded
		} else {
			file, openErr := os.Open(imgPath)
			if openErr != nil {
				return nil, fmt.Errorf("failed to open image: %w", openErr)
			}
			defer func() { _ = file.Close() }()

			decoded, _, decErr := image.Decode(file)
			if decErr != nil {
				return nil, fmt.Errorf("failed to decode image: %w", decErr)
			}
			img = decoded
		}
	}

	return img, nil
}

// computeSharpness computes Laplacian variance on a pre-loaded, resized image.
func computeSharpness(thumb image.Image) (float64, error) {
	// Gaussian pre-blur to suppress noise (Laplacian of Gaussian approach)
	blurred := imaging.Blur(thumb, 1.0)

	// Grayscale conversion via imaging library
	grayImg := imaging.Grayscale(blurred)

	bounds := grayImg.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Ensure the image isn't too small for a 3x3 kernel
	if width < 3 || height < 3 {
		return 0, fmt.Errorf("image too small for laplacian kernel")
	}

	// Apply 3x3 Laplacian Kernel:
	// [ 0  1  0 ]
	// [ 1 -4  1 ]
	// [ 0  1  0 ]
	laplacian := make([]float64, 0, (width-2)*(height-2))
	var sum float64

	for y := 1; y < height-1; y++ {
		for x := 1; x < width-1; x++ {
			valT := colorToLuminance(grayImg.At(x, y-1))
			valB := colorToLuminance(grayImg.At(x, y+1))
			valL := colorToLuminance(grayImg.At(x-1, y))
			valR := colorToLuminance(grayImg.At(x+1, y))
			valC := colorToLuminance(grayImg.At(x, y))

			lVal := valT + valB + valL + valR - (4 * valC)

			laplacian = append(laplacian, lVal)
			sum += lVal
		}
	}

	pixelCount := float64(len(laplacian))
	mean := sum / pixelCount

	var sqDiffSum float64
	for _, lVal := range laplacian {
		diff := lVal - mean
		sqDiffSum += diff * diff
	}

	return sqDiffSum / pixelCount, nil
}

// CalculateLaplacianVariance computes the variance of the laplacian for an image.
// High variance = sharp edges (in-focus). Low variance = smooth (blurry).
// A common pure-Go implementation of OpenCV's Laplacian Variance.
func CalculateLaplacianVariance(imgPath string, thumbnailDir string) (float64, error) {
	img, err := loadQualityImage(imgPath, thumbnailDir)
	if err != nil {
		return 0, err
	}

	thumb := imaging.Resize(img, 300, 0, imaging.NearestNeighbor)
	return computeSharpness(thumb)
}

// Extract luminance from any color quickly
func colorToLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	// Convert from 16-bit alpha-premultiplied scalar to 8-bit (0-255)
	// Weighted luminance: 0.299*R + 0.587*G + 0.114*B
	luminance := (0.299 * float64(r>>8)) + (0.587 * float64(g>>8)) + (0.114 * float64(b>>8))
	return luminance
}

// photoScores holds raw quality metrics for a single photo in a group.
type photoScores struct {
	photo         *PhotoInfo
	sharpness     float64
	exposure      float64
	noise         float64
	contrast      float64
	meanLuminance float64
	metrics       ImageMetrics
}

// normalizeMinMax applies min-max normalization to a slice of values.
// If all values are equal, returns 0.5 for each.
func normalizeMinMax(values []float64) []float64 {
	if len(values) == 0 {
		return nil
	}

	minVal := values[0]
	maxVal := values[0]
	for _, v := range values[1:] {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	result := make([]float64, len(values))
	rang := maxVal - minVal
	if rang == 0 {
		for i := range result {
			result[i] = 0.5
		}
		return result
	}

	for i, v := range values {
		result[i] = (v - minVal) / rang
	}
	return result
}

// FindBestPhotos selects the best photo in each duplicate group using
// multi-factor quality scoring: sharpness, exposure, noise, and contrast.
func FindBestPhotos(ctx context.Context, groups []*DuplicateGroup, thumbnailDir string, progressCallback func(current, total int, message string)) error {
	totalGroups := len(groups)
	var processedCount int32

	eg := new(errgroup.Group)
	eg.SetLimit(runtime.NumCPU())

	for _, group := range groups {
		group := group

		if len(group.Photos) <= 1 {
			if len(group.Photos) == 1 {
				group.Photos[0].IsUnique = true
			}
			count := atomic.AddInt32(&processedCount, 1)
			if progressCallback != nil {
				progressCallback(int(count), totalGroups, "Selecting best quality photos...")
			}
			continue
		}

		eg.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Collect raw scores for each photo in the group.
			var scores []photoScores

			for _, photo := range group.Photos {
				if photo.Hash == nil {
					continue
				}

				img, err := loadQualityImage(photo.Path, thumbnailDir)
				if err != nil {
					if logger.Log != nil {
						logger.Log.Debug("quality: skipping photo, load failed", "path", photo.Path, "error", err)
					}
					continue
				}

				thumb := imaging.Resize(img, 300, 0, imaging.NearestNeighbor)

				sharpness, err := computeSharpness(thumb)
				if err != nil {
					if logger.Log != nil {
						logger.Log.Debug("quality: skipping photo, sharpness failed", "path", photo.Path, "error", err)
					}
					continue
				}

				metrics := ComputeHistogramMetrics(thumb)
				noise := EstimateNoise(thumb)

				scores = append(scores, photoScores{
					photo:         photo,
					sharpness:     sharpness,
					noise:         noise,
					contrast:      metrics.RMSContrast,
					meanLuminance: metrics.MeanLuminance,
					metrics:       metrics,
				})
			}

			// Compute group median luminance for exposure scoring.
			if len(scores) > 0 {
				sortedLum := make([]float64, len(scores))
				for i, s := range scores {
					sortedLum[i] = s.meanLuminance
				}
				sort.Float64s(sortedLum)
				medianLum := sortedLum[len(sortedLum)/2]

				// Compute exposure scores using the group median.
				for i := range scores {
					scores[i].exposure = ExposureScore(scores[i].metrics, medianLum)
				}
			}

			// Select best photo using normalized weighted scoring.
			var bestPhoto *PhotoInfo
			if len(scores) > 0 {
				rawSharpness := make([]float64, len(scores))
				rawExposure := make([]float64, len(scores))
				rawNoise := make([]float64, len(scores))
				rawContrast := make([]float64, len(scores))
				for i, s := range scores {
					rawSharpness[i] = s.sharpness
					rawExposure[i] = s.exposure
					rawNoise[i] = s.noise
					rawContrast[i] = s.contrast
				}

				normSharpness := normalizeMinMax(rawSharpness)
				normExposure := normalizeMinMax(rawExposure)
				normNoise := normalizeMinMax(rawNoise)
				normContrast := normalizeMinMax(rawContrast)

				bestIdx := 0
				bestScore := math.Inf(-1)
				for i := range scores {
					// Lower noise is better, so invert.
					invertedNoise := 1.0 - normNoise[i]
					weighted := weightSharpness*normSharpness[i] +
						weightExposure*normExposure[i] +
						weightNoise*invertedNoise +
						weightContrast*normContrast[i]

					if logger.Log != nil {
						logger.Log.Debug("quality: photo score",
							"path", scores[i].photo.Path,
							"sharpness", normSharpness[i],
							"exposure", normExposure[i],
							"noise", invertedNoise,
							"contrast", normContrast[i],
							"weighted", weighted,
						)
					}

					if weighted > bestScore {
						bestScore = weighted
						bestIdx = i
					}
				}
				bestPhoto = scores[bestIdx].photo
			}

			for _, photo := range group.Photos {
				photo.IsUnique = photo == bestPhoto && bestPhoto != nil
			}

			if bestPhoto == nil && len(group.Photos) > 0 {
				group.Photos[0].IsUnique = true
				for i := 1; i < len(group.Photos); i++ {
					group.Photos[i].IsUnique = false
				}
			}

			count := atomic.AddInt32(&processedCount, 1)
			if progressCallback != nil {
				progressCallback(int(count), totalGroups, "Selecting best quality photos...")
			}
			return nil
		})
	}

	return eg.Wait()
}
