//go:build !windows

package scoring

import (
	"image"
	"image/color"
	"testing"
)

func TestPreprocessImage(t *testing.T) {
	// Create a 10x10 red image.
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := range 10 {
		for x := range 10 {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	tensor := preprocessImage(img, 4, 4)

	// Shape: [1, 3, 4, 4] = 48 floats.
	if len(tensor) != 1*3*4*4 {
		t.Fatalf("tensor length = %d, want 48", len(tensor))
	}

	// R channel should be ~1.0, G and B should be ~0.0.
	// Layout: [batch=0][channel][height][width]
	rStart := 0         // R channel offset
	gStart := 4 * 4     // G channel offset
	bStart := 2 * 4 * 4 // B channel offset

	if tensor[rStart] < 0.9 {
		t.Errorf("R channel pixel = %f, want ~1.0", tensor[rStart])
	}
	if tensor[gStart] > 0.1 {
		t.Errorf("G channel pixel = %f, want ~0.0", tensor[gStart])
	}
	if tensor[bStart] > 0.1 {
		t.Errorf("B channel pixel = %f, want ~0.0", tensor[bStart])
	}
}

func TestPreprocessImage_NonSquare(t *testing.T) {
	// Create a 20x10 image — should resize to target without distortion.
	img := image.NewRGBA(image.Rect(0, 0, 20, 10))
	tensor := preprocessImage(img, 128, 128)

	if len(tensor) != 1*3*128*128 {
		t.Fatalf("tensor length = %d, want %d", len(tensor), 1*3*128*128)
	}
}

func TestParseSelectedBoxes_Empty(t *testing.T) {
	faces := parseSelectedBoxes([]float32{}, []int64{0, 5})
	if len(faces) != 0 {
		t.Errorf("expected 0 faces, got %d", len(faces))
	}
}

func TestParseSelectedBoxes_Single(t *testing.T) {
	// One detection: [x1, y1, x2, y2, confidence, ...landmarks]
	data := []float32{
		0.1, 0.2, 0.5, 0.6, 0.95, // bbox + confidence (normalized)
		0.2, 0.25, 0.4, 0.25, // landmarks
		0.3, 0.4, 0.3, 0.5,
	}

	faces := parseSelectedBoxes(data, []int64{1, 13})
	if len(faces) != 1 {
		t.Fatalf("expected 1 face, got %d", len(faces))
	}

	face := faces[0]
	if face.Confidence < 0.94 || face.Confidence > 0.96 {
		t.Errorf("confidence = %f, want ~0.95", face.Confidence)
	}

	// Bounding box should be scaled to 128x128 input space.
	// x1=0.1*128=12.8, y1=0.2*128=25.6, x2=0.5*128=64, y2=0.6*128=76.8
	bb := face.BoundingBox
	if bb.Min.X < 10 || bb.Min.X > 15 {
		t.Errorf("bbox Min.X = %d, expected ~12", bb.Min.X)
	}
	if bb.Max.X < 60 || bb.Max.X > 68 {
		t.Errorf("bbox Max.X = %d, expected ~64", bb.Max.X)
	}
}

func TestParseSelectedBoxes_ZeroPadded(t *testing.T) {
	// Two rows: one real detection, one zero-padded.
	data := []float32{
		10, 20, 50, 60, 0.9, // real face (pixel coords)
		0, 0, 0, 0, 0, // zero-padded
	}

	faces := parseSelectedBoxes(data, []int64{2, 5})
	if len(faces) != 1 {
		t.Errorf("expected 1 face (zero-padded row skipped), got %d", len(faces))
	}
}

func TestParseSelectedBoxes_PixelCoords(t *testing.T) {
	// Pixel coordinates (values > 1.0).
	data := []float32{10, 20, 60, 80, 0.85}

	faces := parseSelectedBoxes(data, []int64{1, 5})
	if len(faces) != 1 {
		t.Fatalf("expected 1 face, got %d", len(faces))
	}

	// Should NOT multiply by 128 since coords are already pixels.
	bb := faces[0].BoundingBox
	if bb.Min.X != 10 || bb.Min.Y != 20 || bb.Max.X != 60 || bb.Max.Y != 80 {
		t.Errorf("bbox = %v, expected (10,20)-(60,80)", bb)
	}
}

func TestParseSelectedBoxes_TooFewColumns(t *testing.T) {
	// Less than 5 columns — not enough for bbox + confidence.
	faces := parseSelectedBoxes([]float32{1, 2, 3, 4}, []int64{1, 4})
	if len(faces) != 0 {
		t.Errorf("expected 0 faces with <5 columns, got %d", len(faces))
	}
}

func TestLocalProvider_Interface(t *testing.T) {
	// Verify LocalProvider implements ScoringProvider.
	var _ ScoringProvider = (*LocalProvider)(nil)
}

func TestLocalProvider_Name(t *testing.T) {
	p := &LocalProvider{}
	if p.Name() != "Local (ONNX)" {
		t.Errorf("Name = %q, want %q", p.Name(), "Local (ONNX)")
	}
}

func TestLocalProvider_RequiresAPIKey(t *testing.T) {
	p := &LocalProvider{}
	if p.RequiresAPIKey() {
		t.Error("local provider should not require API key")
	}
}

func TestLocalProvider_NotAvailableWithoutRuntime(t *testing.T) {
	tmp := t.TempDir()
	mm, _ := NewModelManager(tmp)
	p := &LocalProvider{modelManager: mm}
	if p.Available() {
		t.Error("should not be available without ONNX runtime")
	}
}
