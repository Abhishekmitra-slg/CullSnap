package scoring

import (
	"context"
	"image"
	"image/color"
	"testing"
)

func newGrayImage(width, height int, fill uint8) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetGray(x, y, color.Gray{Y: fill})
		}
	}
	return img
}

func TestLaplacianVariance_SolidColor(t *testing.T) {
	// Solid color image: zero variance (no edges).
	img := newGrayImage(32, 32, 128)
	v := LaplacianVariance(img, img.Bounds())
	if v > 0.01 {
		t.Errorf("solid color variance = %f, want ~0", v)
	}
}

func TestLaplacianVariance_Edges(t *testing.T) {
	// Checkerboard pattern: high variance (many edges).
	img := image.NewGray(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			if (x+y)%2 == 0 {
				img.SetGray(x, y, color.Gray{Y: 255})
			} else {
				img.SetGray(x, y, color.Gray{Y: 0})
			}
		}
	}

	v := LaplacianVariance(img, img.Bounds())
	if v < 100 {
		t.Errorf("checkerboard variance = %f, want high value", v)
	}
}

func TestLaplacianVariance_Subregion(t *testing.T) {
	// Create image with edges only in one corner.
	img := newGrayImage(64, 64, 128)

	// Add edges in top-left 16x16.
	for y := range 16 {
		for x := range 16 {
			if (x+y)%2 == 0 {
				img.SetGray(x, y, color.Gray{Y: 255})
			} else {
				img.SetGray(x, y, color.Gray{Y: 0})
			}
		}
	}

	// Subregion with edges should have higher variance than subregion without.
	edgeRegion := image.Rect(0, 0, 16, 16)
	smoothRegion := image.Rect(32, 32, 48, 48)

	edgeVar := LaplacianVariance(img, edgeRegion)
	smoothVar := LaplacianVariance(img, smoothRegion)

	if edgeVar <= smoothVar {
		t.Errorf("edge region (%f) should have higher variance than smooth region (%f)", edgeVar, smoothVar)
	}
}

func TestLaplacianVariance_TooSmall(t *testing.T) {
	// Region smaller than 3x3 kernel should return 0.
	img := newGrayImage(2, 2, 128)
	v := LaplacianVariance(img, img.Bounds())
	if v != 0 {
		t.Errorf("too-small region variance = %f, want 0", v)
	}
}

func TestEyeSharpnessFromFace_NoFaces(t *testing.T) {
	img := newGrayImage(100, 100, 128)
	face := FaceRegion{BoundingBox: image.Rect(10, 10, 60, 80)}
	score := EyeSharpnessFromFace(img, face)
	// Should return some value (eye region is top 40% of face).
	if score < 0 {
		t.Errorf("sharpness score should be non-negative, got %f", score)
	}
}

func TestNormalizeLaplacian_BlurryImage(t *testing.T) {
	// Very low variance (blurry) should map to < 0.15.
	score := NormalizeLaplacian(30)
	if score >= 0.15 {
		t.Errorf("blurry variance=30: NormalizeLaplacian = %f, want < 0.15", score)
	}
}

func TestNormalizeLaplacian_SharpImage(t *testing.T) {
	// High variance (sharp) should map to > 0.95.
	score := NormalizeLaplacian(600)
	if score <= 0.95 {
		t.Errorf("sharp variance=600: NormalizeLaplacian = %f, want > 0.95", score)
	}
}

func TestNormalizeLaplacian_MidRange(t *testing.T) {
	// Sigmoid midpoint at variance=200 should be ~0.5.
	score := NormalizeLaplacian(200)
	if score < 0.49 || score > 0.51 {
		t.Errorf("mid-range variance=200: NormalizeLaplacian = %f, want ~0.5", score)
	}
}

func TestSharpnessPlugin_Process(t *testing.T) {
	p := &SharpnessPlugin{}

	// Metadata checks.
	if p.Name() != "sharpness" {
		t.Errorf("Name() = %q, want %q", p.Name(), "sharpness")
	}
	if p.Category() != CategoryQuality {
		t.Errorf("Category() = %v, want CategoryQuality", p.Category())
	}
	if !p.Available() {
		t.Error("Available() = false, want true")
	}
	if len(p.Models()) != 0 {
		t.Errorf("Models() len = %d, want 0", len(p.Models()))
	}

	// Process a simple RGBA image — mix of light and dark pixels for measurable sharpness.
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			if (x+y)%4 == 0 {
				img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			} else {
				img.Set(x, y, color.RGBA{R: 50, G: 50, B: 50, A: 255})
			}
		}
	}

	result, err := p.Process(context.Background(), img)
	if err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
	if result.Quality == nil {
		t.Fatal("Process returned nil Quality")
	}
	if result.Quality.Name != "sharpness" {
		t.Errorf("Quality.Name = %q, want %q", result.Quality.Name, "sharpness")
	}
	if result.Quality.Score < 0 || result.Quality.Score > 1 {
		t.Errorf("Quality.Score = %f, want in [0,1]", result.Quality.Score)
	}
}

func TestEyeSharpnessFromFace_SharpEyes(t *testing.T) {
	img := newGrayImage(100, 100, 128)
	face := FaceRegion{BoundingBox: image.Rect(10, 10, 60, 80)}

	// Add sharp edges in the eye region (top 40% of face = y:10 to y:38).
	for y := 10; y < 38; y++ {
		for x := 10; x < 60; x++ {
			if (x+y)%2 == 0 {
				img.SetGray(x, y, color.Gray{Y: 255})
			} else {
				img.SetGray(x, y, color.Gray{Y: 0})
			}
		}
	}

	score := EyeSharpnessFromFace(img, face)
	if score < 10 {
		t.Errorf("sharp eyes should have high score, got %f", score)
	}
}
