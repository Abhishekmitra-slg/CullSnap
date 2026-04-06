//go:build !windows

package scoring

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// TestFaceDetectorPlugin_Interface verifies FaceDetectorPlugin implements ScoringPlugin.
func TestFaceDetectorPlugin_Interface(t *testing.T) {
	var _ ScoringPlugin = (*FaceDetectorPlugin)(nil)
}

// TestFaceDetectorPlugin_Metadata verifies name, category, and models count.
func TestFaceDetectorPlugin_Metadata(t *testing.T) {
	p := &FaceDetectorPlugin{}

	if got := p.Name(); got != "face-detector" {
		t.Errorf("Name() = %q, want %q", got, "face-detector")
	}

	if got := p.Category(); got != CategoryDetection {
		t.Errorf("Category() = %v, want CategoryDetection", got)
	}

	models := p.Models()
	if len(models) != 1 {
		t.Fatalf("Models() returned %d models, want 1", len(models))
	}
	if models[0].Name != scrfdModelName {
		t.Errorf("Models()[0].Name = %q, want %q", models[0].Name, scrfdModelName)
	}
	if models[0].Filename != scrfdModelFile {
		t.Errorf("Models()[0].Filename = %q, want %q", models[0].Filename, scrfdModelFile)
	}
}

// TestFaceDetectorPlugin_NotAvailableWithoutInit verifies Available() is false
// when the plugin has not been initialized.
func TestFaceDetectorPlugin_NotAvailableWithoutInit(t *testing.T) {
	tmp := t.TempDir()
	mm, err := NewModelManager(tmp)
	if err != nil {
		t.Fatalf("NewModelManager: %v", err)
	}
	p := &FaceDetectorPlugin{modelManager: mm}
	if p.Available() {
		t.Error("Available() = true, want false when not initialized")
	}
}

// TestIOU_NoOverlap checks that iou returns 0 for non-overlapping rectangles.
func TestIOU_NoOverlap(t *testing.T) {
	a := image.Rect(0, 0, 10, 10)
	b := image.Rect(20, 20, 30, 30)
	if got := iou(a, b); got != 0 {
		t.Errorf("iou(non-overlapping) = %f, want 0", got)
	}
}

// TestIOU_FullOverlap checks that iou returns 1 for identical rectangles.
func TestIOU_FullOverlap(t *testing.T) {
	a := image.Rect(0, 0, 10, 10)
	b := image.Rect(0, 0, 10, 10)
	if got := iou(a, b); got != 1.0 {
		t.Errorf("iou(identical) = %f, want 1.0", got)
	}
}

// TestIOU_PartialOverlap checks iou for a 5×5 intersection of two 10×10 boxes
// offset by 5 in both axes.
//
//	a = Rect(0,0,10,10)  area=100
//	b = Rect(5,5,15,15)  area=100
//	intersection = Rect(5,5,10,10)  area=25
//	union = 100+100-25 = 175
//	iou = 25/175 ≈ 0.1429
func TestIOU_PartialOverlap(t *testing.T) {
	a := image.Rect(0, 0, 10, 10)
	b := image.Rect(5, 5, 15, 15)

	want := 25.0 / 175.0
	got := iou(a, b)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("iou(partial) = %f, want %f", got, want)
	}
}

// TestNMS_RemovesDuplicates verifies that NMS removes a heavily overlapping box
// while keeping a non-overlapping one.
//
// Input: 3 faces — face0 and face1 overlap (iou > threshold), face2 does not.
// Expected: face0 (highest confidence) and face2 are kept; face1 is suppressed.
func TestNMS_RemovesDuplicates(t *testing.T) {
	faces := []FaceRegion{
		{BoundingBox: image.Rect(0, 0, 10, 10), Confidence: 0.9},   // face0
		{BoundingBox: image.Rect(1, 1, 11, 11), Confidence: 0.8},   // face1 — overlaps face0
		{BoundingBox: image.Rect(50, 50, 60, 60), Confidence: 0.7}, // face2 — separate
	}

	result := nms(faces, 0.4)
	if len(result) != 2 {
		t.Fatalf("nms returned %d faces, want 2", len(result))
	}
	// Highest confidence face should be first.
	if result[0].Confidence != 0.9 {
		t.Errorf("result[0].Confidence = %f, want 0.9", result[0].Confidence)
	}
}

// TestNMS_EmptyInput verifies that NMS handles an empty slice gracefully.
func TestNMS_EmptyInput(t *testing.T) {
	result := nms([]FaceRegion{}, 0.4)
	if len(result) != 0 {
		t.Errorf("nms(empty) returned %d faces, want 0", len(result))
	}
}

// TestPreprocessForSCRFD_OutputShape verifies a 300×200 image yields a
// 3*640*640 element tensor.
func TestPreprocessForSCRFD_OutputShape(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 300, 200))
	tensor := preprocessForSCRFD(img, scrfdInputSize)

	want := 3 * scrfdInputSize * scrfdInputSize
	if len(tensor) != want {
		t.Errorf("tensor length = %d, want %d", len(tensor), want)
	}
}

// TestPreprocessForSCRFD_ValuesNormalized verifies that all output values are in [-1,1]
// after mean=[127.5] std=[128] normalization.
func TestPreprocessForSCRFD_ValuesNormalized(t *testing.T) {
	// Create a bright-red 50×50 image.
	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	for y := range 50 {
		for x := range 50 {
			img.Set(x, y, color.RGBA{R: 255, G: 128, B: 0, A: 255})
		}
	}

	tensor := preprocessForSCRFD(img, scrfdInputSize)
	for i, v := range tensor {
		if v < -1.01 || v > 1.01 {
			t.Errorf("tensor[%d] = %f, out of [-1,1] range", i, v)
			break
		}
	}
}
