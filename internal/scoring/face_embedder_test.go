//go:build !windows

package scoring

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// TestFaceEmbedderPlugin_Interface verifies FaceEmbedderPlugin implements RecognitionPlugin.
func TestFaceEmbedderPlugin_Interface(t *testing.T) {
	var _ RecognitionPlugin = (*FaceEmbedderPlugin)(nil)
}

// TestFaceEmbedderPlugin_Metadata verifies name, category, and models count.
func TestFaceEmbedderPlugin_Metadata(t *testing.T) {
	p := &FaceEmbedderPlugin{}

	if got := p.Name(); got != "face-embedder" {
		t.Errorf("Name() = %q, want %q", got, "face-embedder")
	}

	if got := p.Category(); got != CategoryRecognition {
		t.Errorf("Category() = %v, want CategoryRecognition", got)
	}

	models := p.Models()
	if len(models) != 1 {
		t.Fatalf("Models() returned %d models, want 1", len(models))
	}
	if models[0].Name != arcfaceModelName {
		t.Errorf("Models()[0].Name = %q, want %q", models[0].Name, arcfaceModelName)
	}
	if models[0].Filename != arcfaceModelFile {
		t.Errorf("Models()[0].Filename = %q, want %q", models[0].Filename, arcfaceModelFile)
	}
}

// TestL2Normalize verifies that [3, 4] is normalized to [0.6, 0.8].
func TestL2Normalize(t *testing.T) {
	v := []float32{3, 4}
	got := l2Normalize(v)

	wantX := float32(0.6)
	wantY := float32(0.8)

	if math.Abs(float64(got[0]-wantX)) > 1e-6 {
		t.Errorf("l2Normalize[0] = %f, want %f", got[0], wantX)
	}
	if math.Abs(float64(got[1]-wantY)) > 1e-6 {
		t.Errorf("l2Normalize[1] = %f, want %f", got[1], wantY)
	}
}

// TestL2Normalize_ZeroVector verifies that a zero vector is returned unchanged.
func TestL2Normalize_ZeroVector(t *testing.T) {
	v := []float32{0, 0, 0}
	got := l2Normalize(v)

	for i, x := range got {
		if x != 0 {
			t.Errorf("l2Normalize zero vector[%d] = %f, want 0", i, x)
		}
	}
}

// TestPreprocessForArcFace_OutputShape verifies a 112×112 image yields a
// 3*112*112 element tensor.
func TestPreprocessForArcFace_OutputShape(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, arcfaceInputSize, arcfaceInputSize))
	tensor := preprocessForArcFace(img, arcfaceInputSize)

	want := 3 * arcfaceInputSize * arcfaceInputSize
	if len(tensor) != want {
		t.Errorf("tensor length = %d, want %d", len(tensor), want)
	}
}

// TestPreprocessForArcFace_ValuesInRange verifies all output values are in [-1.01, 1.01].
func TestPreprocessForArcFace_ValuesInRange(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			img.Set(x, y, color.RGBA{R: 255, G: 128, B: 0, A: 255})
		}
	}

	tensor := preprocessForArcFace(img, arcfaceInputSize)
	for i, v := range tensor {
		if v < -1.01 || v > 1.01 {
			t.Errorf("tensor[%d] = %f, out of [-1.01, 1.01] range", i, v)
			break
		}
	}
}

// TestCropAndAlignFace_OutputSize verifies a 640×480 image with a 100×100 face
// produces a 112×112 crop.
func TestCropAndAlignFace_OutputSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 640, 480))
	face := FaceRegion{
		BoundingBox: image.Rect(200, 150, 300, 250),
		Confidence:  0.95,
	}

	crop := cropAndAlignFace(img, face, arcfaceInputSize)
	if crop.Bounds().Dx() != arcfaceInputSize || crop.Bounds().Dy() != arcfaceInputSize {
		t.Errorf("crop size = %dx%d, want %dx%d",
			crop.Bounds().Dx(), crop.Bounds().Dy(),
			arcfaceInputSize, arcfaceInputSize)
	}
}

// TestCropAndAlignFace_EdgeFace verifies that a face at the image boundary
// does not panic and still produces the correct output size.
func TestCropAndAlignFace_EdgeFace(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	// Face bbox at the very top-left corner — padding will be clamped.
	face := FaceRegion{
		BoundingBox: image.Rect(0, 0, 20, 20),
		Confidence:  0.9,
	}

	// Should not panic.
	crop := cropAndAlignFace(img, face, arcfaceInputSize)
	if crop.Bounds().Dx() != arcfaceInputSize || crop.Bounds().Dy() != arcfaceInputSize {
		t.Errorf("edge crop size = %dx%d, want %dx%d",
			crop.Bounds().Dx(), crop.Bounds().Dy(),
			arcfaceInputSize, arcfaceInputSize)
	}
}
