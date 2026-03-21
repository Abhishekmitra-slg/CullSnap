package dedupe

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestComputeHistogramMetrics_NormalImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 128, G: 128, B: 128, A: 255})
		}
	}

	m := ComputeHistogramMetrics(img)

	if m.HighlightClip != 0 {
		t.Errorf("expected no highlight clipping, got %f", m.HighlightClip)
	}
	if m.ShadowClip != 0 {
		t.Errorf("expected no shadow clipping, got %f", m.ShadowClip)
	}
	if math.Abs(m.MeanLuminance-128) > 1 {
		t.Errorf("expected mean luminance ~128, got %f", m.MeanLuminance)
	}
	// Uniform image should have near-zero contrast
	if m.RMSContrast > 0.001 {
		t.Errorf("expected near-zero RMS contrast for uniform image, got %f", m.RMSContrast)
	}
}

func TestComputeHistogramMetrics_Overexposed(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	m := ComputeHistogramMetrics(img)

	if m.HighlightClip != 1.0 {
		t.Errorf("expected 100%% highlight clipping, got %f", m.HighlightClip)
	}
	if m.ShadowClip != 0 {
		t.Errorf("expected no shadow clipping, got %f", m.ShadowClip)
	}
	if math.Abs(m.MeanLuminance-255) > 1 {
		t.Errorf("expected mean luminance ~255, got %f", m.MeanLuminance)
	}
}

func TestComputeHistogramMetrics_Underexposed(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 2, G: 2, B: 2, A: 255})
		}
	}

	m := ComputeHistogramMetrics(img)

	if m.ShadowClip != 1.0 {
		t.Errorf("expected 100%% shadow clipping, got %f", m.ShadowClip)
	}
	if m.HighlightClip != 0 {
		t.Errorf("expected no highlight clipping, got %f", m.HighlightClip)
	}
	if m.MeanLuminance > 5 {
		t.Errorf("expected very low mean luminance, got %f", m.MeanLuminance)
	}
}

func TestEstimateNoise_UniformImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 128, G: 128, B: 128, A: 255})
		}
	}

	noise := EstimateNoise(img)
	if noise > 0.01 {
		t.Errorf("expected near-zero noise for uniform image, got %f", noise)
	}
}

func TestExposureScore_WellExposed(t *testing.T) {
	m := ImageMetrics{
		HighlightClip: 0.0,
		ShadowClip:    0.0,
		MeanLuminance: 128,
	}
	score := ExposureScore(m, 128)
	if score < 0.9 {
		t.Errorf("expected well-exposed score near 1.0, got %f", score)
	}
}

func TestExposureScore_Overexposed(t *testing.T) {
	wellExposed := ImageMetrics{
		HighlightClip: 0.0,
		ShadowClip:    0.0,
		MeanLuminance: 128,
	}
	overExposed := ImageMetrics{
		HighlightClip: 0.3,
		ShadowClip:    0.0,
		MeanLuminance: 220,
	}

	wellScore := ExposureScore(wellExposed, 128)
	overScore := ExposureScore(overExposed, 128)

	if overScore >= wellScore {
		t.Errorf("expected overexposed score (%f) to be lower than well-exposed score (%f)", overScore, wellScore)
	}
}
