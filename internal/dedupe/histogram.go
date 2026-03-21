package dedupe

import (
	"image"
	"image/color"
	"math"
)

// ImageMetrics holds histogram-derived quality metrics.
type ImageMetrics struct {
	HighlightClip float64 // fraction of pixels with luminance >= 250
	ShadowClip    float64 // fraction of pixels with luminance <= 5
	MeanLuminance float64 // average luminance 0-255
	RMSContrast   float64 // std_dev(luminance) / mean(luminance)
}

// ComputeHistogramMetrics computes exposure and contrast metrics in a single pass.
func ComputeHistogramMetrics(img image.Image) ImageMetrics {
	bounds := img.Bounds()
	totalPixels := float64(bounds.Dx() * bounds.Dy())
	if totalPixels == 0 {
		return ImageMetrics{}
	}

	var sumLum, sumLumSq float64
	var highlightCount, shadowCount float64

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			lum := pixelLuminance(img.At(x, y))
			sumLum += lum
			sumLumSq += lum * lum
			if lum >= 250 {
				highlightCount++
			}
			if lum <= 5 {
				shadowCount++
			}
		}
	}

	meanLum := sumLum / totalPixels
	variance := (sumLumSq / totalPixels) - (meanLum * meanLum)
	stdDev := math.Sqrt(math.Max(0, variance))

	rmsContrast := 0.0
	if meanLum > 0 {
		rmsContrast = stdDev / meanLum
	}

	return ImageMetrics{
		HighlightClip: highlightCount / totalPixels,
		ShadowClip:    shadowCount / totalPixels,
		MeanLuminance: meanLum,
		RMSContrast:   rmsContrast,
	}
}

// EstimateNoise computes noise sigma using the Immerkaer method.
func EstimateNoise(img image.Image) float64 {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width < 3 || height < 3 {
		return 0
	}

	var sum float64
	for y := bounds.Min.Y + 1; y < bounds.Max.Y-1; y++ {
		for x := bounds.Min.X + 1; x < bounds.Max.X-1; x++ {
			val := pixelLuminance(img.At(x-1, y-1)) - 2*pixelLuminance(img.At(x, y-1)) + pixelLuminance(img.At(x+1, y-1)) -
				2*pixelLuminance(img.At(x-1, y)) + 4*pixelLuminance(img.At(x, y)) - 2*pixelLuminance(img.At(x+1, y)) +
				pixelLuminance(img.At(x-1, y+1)) - 2*pixelLuminance(img.At(x, y+1)) + pixelLuminance(img.At(x+1, y+1))
			sum += math.Abs(val)
		}
	}

	sigma := sum * math.Sqrt(math.Pi/2.0) / (6.0 * float64(width-2) * float64(height-2))
	return sigma
}

// ExposureScore computes a 0-1 score penalizing clipped highlights/shadows.
func ExposureScore(m ImageMetrics, groupMedianLuminance float64) float64 {
	balance := math.Abs(m.MeanLuminance-groupMedianLuminance) / 128.0
	score := 1.0 - (3.0 * m.HighlightClip) - (1.0 * m.ShadowClip) - (0.5 * balance)
	return math.Max(0, math.Min(1, score))
}

func pixelLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
}
