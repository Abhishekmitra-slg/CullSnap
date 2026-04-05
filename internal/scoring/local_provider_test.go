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

func TestParseSelectedBoxes_Empty3D(t *testing.T) {
	// [1, 0, 16] — batch=1, N=0, 16 cols.
	faces := parseSelectedBoxes([]float32{}, []int64{1, 0, 16})
	if len(faces) != 0 {
		t.Errorf("expected 0 faces, got %d", len(faces))
	}
}

func TestParseSelectedBoxes_Single2D(t *testing.T) {
	// [1, 16] — 1 face detected, 16 values: 4 bbox + 12 landmarks.
	data := make([]float32, 16)
	data[0] = 0.1 // x1
	data[1] = 0.2 // y1
	data[2] = 0.5 // x2
	data[3] = 0.6 // y2
	// Remaining 12 values are landmarks (don't affect bbox parsing).

	faces := parseSelectedBoxes(data, []int64{1, 16})
	if len(faces) != 1 {
		t.Fatalf("expected 1 face, got %d", len(faces))
	}

	// Confidence should be 1.0 (model already filtered).
	if faces[0].Confidence != 1.0 {
		t.Errorf("confidence = %f, want 1.0", faces[0].Confidence)
	}

	// Bbox scaled from normalized [0,1] to [0,128].
	bb := faces[0].BoundingBox
	if bb.Min.X < 10 || bb.Min.X > 15 {
		t.Errorf("bbox Min.X = %d, expected ~13", bb.Min.X)
	}
	if bb.Max.X < 60 || bb.Max.X > 68 {
		t.Errorf("bbox Max.X = %d, expected ~64", bb.Max.X)
	}
}

func TestParseSelectedBoxes_Multi3D(t *testing.T) {
	// [1, 2, 16] — batch=1, 2 faces, 16 values each.
	data := make([]float32, 32)
	data[0] = 10 // face 1 bbox
	data[1] = 20
	data[2] = 50
	data[3] = 60
	data[16] = 70 // face 2 bbox
	data[17] = 30
	data[18] = 110
	data[19] = 90

	faces := parseSelectedBoxes(data, []int64{1, 2, 16})
	if len(faces) != 2 {
		t.Fatalf("expected 2 faces, got %d", len(faces))
	}

	// Pixel coords (>1.0) should NOT be scaled.
	if faces[0].BoundingBox.Min.X != 10 {
		t.Errorf("face 0 bbox Min.X = %d, want 10", faces[0].BoundingBox.Min.X)
	}
	if faces[1].BoundingBox.Min.X != 70 {
		t.Errorf("face 1 bbox Min.X = %d, want 70", faces[1].BoundingBox.Min.X)
	}
}

func TestParseSelectedBoxes_ZeroPadded(t *testing.T) {
	// [1, 2, 16] — one real face, one all-zeros.
	data := make([]float32, 32)
	data[0] = 10
	data[1] = 20
	data[2] = 50
	data[3] = 60
	// data[16..31] = all zeros (padded row).

	faces := parseSelectedBoxes(data, []int64{1, 2, 16})
	if len(faces) != 1 {
		t.Errorf("expected 1 face (zero-padded skipped), got %d", len(faces))
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
