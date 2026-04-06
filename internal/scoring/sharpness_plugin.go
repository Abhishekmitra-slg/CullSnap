package scoring

import (
	"context"
	"cullsnap/internal/logger"
	"image"
	"image/color"
)

// SharpnessPlugin is a no-model quality plugin that measures global image
// sharpness using the Laplacian-variance method. It is always available and
// requires no external model files.
type SharpnessPlugin struct{}

// Name returns the plugin identifier.
func (s *SharpnessPlugin) Name() string { return "sharpness" }

// Category returns CategoryQuality.
func (s *SharpnessPlugin) Category() PluginCategory { return CategoryQuality }

// Models returns nil — this plugin needs no external model files.
func (s *SharpnessPlugin) Models() []ModelSpec { return nil }

// Available always returns true — the plugin is purely computational.
func (s *SharpnessPlugin) Available() bool { return true }

// Init is a no-op; there are no models to load.
func (s *SharpnessPlugin) Init(_ string) error { return nil }

// Close is a no-op; there are no resources to release.
func (s *SharpnessPlugin) Close() error { return nil }

// Process converts img to grayscale, computes the Laplacian variance over the
// full image bounds, normalises it to [0,1], and returns a PluginResult with
// the quality score.
func (s *SharpnessPlugin) Process(_ context.Context, img image.Image) (PluginResult, error) {
	gray := toGray(img)
	variance := LaplacianVariance(gray, gray.Bounds())
	score := NormalizeLaplacian(variance)

	logger.Log.Debug("scoring: sharpness plugin: processed image",
		"bounds", gray.Bounds(),
		"laplacian_variance", variance,
		"normalized_score", score,
	)

	return PluginResult{
		Quality: &QualityScore{
			Score: score,
			Name:  "sharpness",
		},
	}, nil
}

// toGray converts any image.Image to *image.Gray. If img is already *image.Gray
// it is returned directly without copying.
func toGray(img image.Image) *image.Gray {
	if g, ok := img.(*image.Gray); ok {
		return g
	}

	bounds := img.Bounds()
	gray := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray.Set(x, y, color.GrayModel.Convert(img.At(x, y)))
		}
	}
	return gray
}
