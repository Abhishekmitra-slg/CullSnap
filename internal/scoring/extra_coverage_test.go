//go:build !windows

package scoring

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// ──────────────────────────────────────────────────────────────────────────────
// face_detector.go — parseSCRFDGeneric, generateSCRFDAnchors,
//                    tensorNumElements, matchesScoreShape, matchesBoxShape,
//                    matchesKpsShape
// ──────────────────────────────────────────────────────────────────────────────

// TestParseSCRFDGeneric_EmptyTensors verifies nil is returned for empty input.
func TestParseSCRFDGeneric_EmptyTensors(t *testing.T) {
	result := parseSCRFDGeneric(nil, 640, 480, 0.5)
	if result != nil {
		t.Errorf("parseSCRFDGeneric(nil) = %v, want nil", result)
	}
}

// TestParseSCRFDGeneric_BelowConfidence verifies detections below confThresh are filtered.
func TestParseSCRFDGeneric_BelowConfidence(t *testing.T) {
	// One row: [x1, y1, x2, y2, conf] all with conf below threshold.
	tensors := []scrfdTensor{
		{
			name:  "output",
			shape: []int64{1, 5},
			data:  []float32{10, 10, 50, 50, 0.1}, // conf 0.1 < thresh 0.5
		},
	}
	result := parseSCRFDGeneric(tensors, 640, 480, 0.5)
	if len(result) != 0 {
		t.Errorf("parseSCRFDGeneric(low conf) = %d faces, want 0", len(result))
	}
}

// TestParseSCRFDGeneric_AboveConfidence verifies detections above confThresh are returned.
func TestParseSCRFDGeneric_AboveConfidence(t *testing.T) {
	// Two rows: first above threshold, second below.
	tensors := []scrfdTensor{
		{
			name:  "output",
			shape: []int64{2, 5},
			data: []float32{
				10, 10, 50, 50, 0.9, // above threshold
				100, 100, 150, 150, 0.1, // below threshold
			},
		},
	}
	result := parseSCRFDGeneric(tensors, 640, 480, 0.5)
	if len(result) != 1 {
		t.Errorf("parseSCRFDGeneric(one above thresh) = %d faces, want 1", len(result))
	}
	if math.Abs(result[0].Confidence-0.9) > 1e-5 {
		t.Errorf("Confidence = %f, want ~0.9", result[0].Confidence)
	}
}

// TestParseSCRFDGeneric_NormalizedCoords verifies coords in [0,1] are scaled to input size.
func TestParseSCRFDGeneric_NormalizedCoords(t *testing.T) {
	// Normalized coords — should be scaled by scrfdInputSize before scaleX/scaleY.
	tensors := []scrfdTensor{
		{
			name:  "output",
			shape: []int64{1, 5},
			data:  []float32{0.1, 0.1, 0.5, 0.5, 0.8},
		},
	}
	result := parseSCRFDGeneric(tensors, 640, 480, 0.5)
	if len(result) != 1 {
		t.Fatalf("parseSCRFDGeneric(normalized) = %d faces, want 1", len(result))
	}
	// Coords should be non-trivially scaled — just verify bounding box is non-zero.
	bb := result[0].BoundingBox
	if bb.Dx() == 0 || bb.Dy() == 0 {
		t.Errorf("bounding box has zero size: %v", bb)
	}
}

// TestParseSCRFDGeneric_ZeroCoords verifies all-zero bounding boxes are skipped.
func TestParseSCRFDGeneric_ZeroCoords(t *testing.T) {
	tensors := []scrfdTensor{
		{
			name:  "output",
			shape: []int64{1, 5},
			data:  []float32{0, 0, 0, 0, 0.9}, // zero box but high conf
		},
	}
	result := parseSCRFDGeneric(tensors, 640, 480, 0.5)
	if len(result) != 0 {
		t.Errorf("parseSCRFDGeneric(zero coords) = %d faces, want 0", len(result))
	}
}

// TestParseSCRFDGeneric_NoShape uses a tensor without a shape hint (cols inferred as 5).
func TestParseSCRFDGeneric_NoShape(t *testing.T) {
	tensors := []scrfdTensor{
		{
			name:  "output",
			shape: nil, // no shape — should fall back to cols=5
			data:  []float32{20, 20, 80, 80, 0.7},
		},
	}
	result := parseSCRFDGeneric(tensors, 640, 480, 0.5)
	if len(result) != 1 {
		t.Errorf("parseSCRFDGeneric(no shape) = %d faces, want 1", len(result))
	}
}

// TestParseSCRFDGeneric_PicksLargestTensor verifies the function chooses the tensor
// with the most elements when multiple tensors are provided.
func TestParseSCRFDGeneric_PicksLargestTensor(t *testing.T) {
	tensors := []scrfdTensor{
		{
			name:  "small",
			shape: []int64{1, 5},
			data:  []float32{10, 10, 50, 50, 0.1}, // below threshold
		},
		{
			name:  "large",
			shape: []int64{2, 5},
			data: []float32{
				100, 100, 200, 200, 0.9, // above threshold
				300, 300, 400, 400, 0.8,
			},
		},
	}
	result := parseSCRFDGeneric(tensors, 640, 480, 0.5)
	if len(result) != 2 {
		t.Errorf("parseSCRFDGeneric(largest tensor) = %d faces, want 2", len(result))
	}
}

// TestParseSCRFDStridedOutputs_NameBasedMatching verifies that tensors are matched
// by name to avoid cross-stride collisions where element counts are ambiguous
// (e.g. stride-8 scores = 12800 elements = stride-16 boxes).
func TestParseSCRFDStridedOutputs_NameBasedMatching(t *testing.T) {
	// Simulate SCRFD 2.5G output tensors with realistic naming and sizes.
	// Only populate stride-8 with a detectable face; other strides are zeros.
	const inputSize = 640
	numAnchors := 2

	type strideSpec struct {
		stride  int
		numLocs int
	}
	strides := []strideSpec{
		{8, (inputSize / 8) * (inputSize / 8) * numAnchors},    // 12800
		{16, (inputSize / 16) * (inputSize / 16) * numAnchors}, // 3200
		{32, (inputSize / 32) * (inputSize / 32) * numAnchors}, // 800
	}

	var tensors []scrfdTensor
	for _, s := range strides {
		scores := make([]float32, s.numLocs)
		boxes := make([]float32, s.numLocs*4)

		// Put a single high-confidence detection in stride 8 only.
		if s.stride == 8 {
			scores[0] = 0.95 // high confidence
			// Distance-based bbox: [left, top, right, bottom] distances from anchor.
			boxes[0] = 2.0 // left
			boxes[1] = 2.0 // top
			boxes[2] = 2.0 // right
			boxes[3] = 2.0 // bottom
		}

		tensors = append(tensors,
			scrfdTensor{
				name:  fmt.Sprintf("score_%d", s.stride),
				shape: []int64{1, int64(s.numLocs), 1},
				data:  scores,
			},
			scrfdTensor{
				name:  fmt.Sprintf("bbox_%d", s.stride),
				shape: []int64{1, int64(s.numLocs), 4},
				data:  boxes,
			},
		)
	}

	faces := parseSCRFDStridedOutputs(tensors, 300, 200, scrfdConfThresh)

	// With correct name-based matching, only 1 face should be detected.
	// The old size-only matching could produce thousands of false detections
	// because box data (large floats) would be misread as confidence scores.
	if len(faces) != 1 {
		t.Errorf("parseSCRFDStridedOutputs() detected %d faces, want 1", len(faces))
	}
	if len(faces) > 0 && faces[0].Confidence < 0.9 {
		t.Errorf("face confidence = %f, want >= 0.9", faces[0].Confidence)
	}
}

// TestParseSCRFDStridedOutputs_FallbackSizeBased verifies that unnamed tensors
// still work via the size-based fallback with used-tensor tracking.
func TestParseSCRFDStridedOutputs_FallbackSizeBased(t *testing.T) {
	// Use unnamed tensors (no stride info in name) for stride 32 only (simplest).
	numLocs := (640 / 32) * (640 / 32) * 2 // 800

	scores := make([]float32, numLocs)
	boxes := make([]float32, numLocs*4)
	scores[0] = 0.85
	boxes[0] = 1.0
	boxes[1] = 1.0
	boxes[2] = 1.0
	boxes[3] = 1.0

	tensors := []scrfdTensor{
		{name: "output_a", shape: []int64{1, int64(numLocs), 1}, data: scores},
		{name: "output_b", shape: []int64{1, int64(numLocs), 4}, data: boxes},
	}

	faces := parseSCRFDStridedOutputs(tensors, 300, 200, scrfdConfThresh)
	if len(faces) != 1 {
		t.Errorf("parseSCRFDStridedOutputs(unnamed) detected %d faces, want 1", len(faces))
	}
}

// TestGenerateSCRFDAnchors_Count verifies the total number of anchors.
func TestGenerateSCRFDAnchors_Count(t *testing.T) {
	featH, featW, stride, numAnchors := 3, 4, 8, 2
	anchors := generateSCRFDAnchors(featH, featW, stride, numAnchors)
	want := featH * featW * numAnchors
	if len(anchors) != want {
		t.Errorf("generateSCRFDAnchors count = %d, want %d", len(anchors), want)
	}
}

// TestGenerateSCRFDAnchors_Values verifies the first anchor center for a 2×2 feat map.
func TestGenerateSCRFDAnchors_Values(t *testing.T) {
	// feat 1×1, stride 8, 1 anchor → cx = 0.5*8 = 4, cy = 0.5*8 = 4.
	anchors := generateSCRFDAnchors(1, 1, 8, 1)
	if len(anchors) != 1 {
		t.Fatalf("expected 1 anchor, got %d", len(anchors))
	}
	if anchors[0][0] != 4.0 || anchors[0][1] != 4.0 {
		t.Errorf("anchor[0] = %v, want [4, 4]", anchors[0])
	}
}

// TestTensorNumElements_EmptyShape verifies 0 for nil shape.
func TestTensorNumElements_EmptyShape(t *testing.T) {
	if n := tensorNumElements(nil); n != 0 {
		t.Errorf("tensorNumElements(nil) = %d, want 0", n)
	}
}

// TestTensorNumElements_SingleDim verifies simple case.
func TestTensorNumElements_SingleDim(t *testing.T) {
	if n := tensorNumElements([]int64{10}); n != 10 {
		t.Errorf("tensorNumElements([10]) = %d, want 10", n)
	}
}

// TestTensorNumElements_MultiDim verifies product of dimensions.
func TestTensorNumElements_MultiDim(t *testing.T) {
	if n := tensorNumElements([]int64{2, 3, 4}); n != 24 {
		t.Errorf("tensorNumElements([2,3,4]) = %d, want 24", n)
	}
}

// TestMatchesScoreShape verifies matchesScoreShape returns true for exact-count shapes.
func TestMatchesScoreShape(t *testing.T) {
	// shape [5] with numLocs=5 → true
	if !matchesScoreShape([]int64{5}, 5) {
		t.Error("matchesScoreShape([5], 5) = false, want true")
	}
	// shape [2, 3] = 6 elements with numLocs=6 → true
	if !matchesScoreShape([]int64{2, 3}, 6) {
		t.Error("matchesScoreShape([2,3], 6) = false, want true")
	}
	// mismatch → false
	if matchesScoreShape([]int64{5}, 4) {
		t.Error("matchesScoreShape([5], 4) = true, want false")
	}
}

// TestMatchesBoxShape verifies matchesBoxShape checks n*4.
func TestMatchesBoxShape(t *testing.T) {
	if !matchesBoxShape([]int64{5, 4}, 5) {
		t.Error("matchesBoxShape([5,4], 5) = false, want true")
	}
	if matchesBoxShape([]int64{5, 4}, 4) {
		t.Error("matchesBoxShape([5,4], 4) = true, want false")
	}
}

// TestMatchesKpsShape verifies matchesKpsShape checks n*10.
func TestMatchesKpsShape(t *testing.T) {
	if !matchesKpsShape([]int64{5, 10}, 5) {
		t.Error("matchesKpsShape([5,10], 5) = false, want true")
	}
	if matchesKpsShape([]int64{5, 10}, 4) {
		t.Error("matchesKpsShape([5,10], 4) = true, want false")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// face_embedder.go — cropAndAlignFace edge cases
// ──────────────────────────────────────────────────────────────────────────────

// TestCropAndAlignFace_ClampNegative verifies that a face box starting outside the
// image boundary is clamped to the image edge without panicking.
func TestCropAndAlignFace_ClampNegative(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := range 100 {
		for x := range 100 {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}

	face := FaceRegion{
		BoundingBox: image.Rect(-10, -10, 30, 30), // partially outside
	}
	cropped := cropAndAlignFace(img, face, 64)
	b := cropped.Bounds()
	if b.Dx() != 64 || b.Dy() != 64 {
		t.Errorf("cropped size = %dx%d, want 64x64", b.Dx(), b.Dy())
	}
}

// TestCropAndAlignFace_OutsideBounds verifies zero-size edge cases don't panic.
func TestCropAndAlignFace_OutsideBounds(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	// Face bounding box entirely outside the image — clamping should produce 1×1.
	face := FaceRegion{
		BoundingBox: image.Rect(20, 20, 5, 5), // inverted / outside
	}
	// Should not panic.
	cropped := cropAndAlignFace(img, face, 32)
	b := cropped.Bounds()
	if b.Dx() != 32 || b.Dy() != 32 {
		t.Errorf("cropped size = %dx%d, want 32x32", b.Dx(), b.Dy())
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// pipeline.go — estimateEyeOpenness with landmark data, mergeResults via pipeline
// ──────────────────────────────────────────────────────────────────────────────

// TestEstimateEyeOpenness_WithBoundingBox verifies computation when both
// bounding box and landmarks are set.
func TestEstimateEyeOpenness_WithBoundingBox(t *testing.T) {
	face := FaceRegion{
		BoundingBox: image.Rect(0, 0, 100, 100),
		// Landmark layout: [leftEye, rightEye, nose, leftMouth, rightMouth]
		// Use normalised values so nose is ~0.15–0.35 below eyes.
		Landmarks: [5][2]float32{
			{0.3, 0.3},  // left eye
			{0.7, 0.3},  // right eye
			{0.5, 0.55}, // nose — ratio = 0.55 - 0.30 = 0.25 → openness = (0.25-0.15)/(0.20) = 0.5
			{0.3, 0.8},  // left mouth
			{0.7, 0.8},  // right mouth
		},
	}
	got := estimateEyeOpenness(face)
	// ratio = 0.25, normalised (0.25-0.15)/0.20 = 0.5
	if math.Abs(got-0.5) > 0.05 {
		t.Errorf("estimateEyeOpenness = %f, want ~0.5", got)
	}
}

// TestEstimateEyeOpenness_ZeroHeight verifies the no-bounding-box fallback path.
func TestEstimateEyeOpenness_ZeroHeight(t *testing.T) {
	face := FaceRegion{
		BoundingBox: image.Rect(0, 0, 0, 0), // zero height
		Landmarks: [5][2]float32{
			{0.3, 0.25}, // left eye
			{0.7, 0.25}, // right eye
			{0.5, 0.50}, // nose — ratio = 0.25 → openness = (0.25-0.15)/0.20 = 0.5
			{0.3, 0.80},
			{0.7, 0.80},
		},
	}
	got := estimateEyeOpenness(face)
	if math.Abs(got-0.5) > 0.05 {
		t.Errorf("estimateEyeOpenness(zeroHeight) = %f, want ~0.5", got)
	}
}

// TestEstimateEyeOpenness_Clamped verifies extreme landmark positions are clamped to [0,1].
func TestEstimateEyeOpenness_Clamped(t *testing.T) {
	face := FaceRegion{
		BoundingBox: image.Rect(0, 0, 100, 100),
		Landmarks: [5][2]float32{
			{0.5, 0.0}, // eyes at very top
			{0.5, 0.0},
			{0.5, 0.0}, // nose same as eyes → ratio 0 → clamped to 0
			{0.5, 0.9},
			{0.5, 0.9},
		},
	}
	got := estimateEyeOpenness(face)
	if got < 0 || got > 1 {
		t.Errorf("estimateEyeOpenness out of [0,1]: %f", got)
	}
}

// TestPipeline_Execute_MultipleFaces verifies BestFaceIdx is selected for the
// sharpest face and EyeOpenness is computed.
func TestPipeline_Execute_MultipleFaces(t *testing.T) {
	reg := NewRegistry()

	faces := []FaceRegion{
		{BoundingBox: image.Rect(10, 10, 40, 40), Confidence: 0.9},
		{BoundingBox: image.Rect(60, 60, 90, 90), Confidence: 0.8},
	}
	reg.Register(newMockDetector("mock-detector", true, faces))
	reg.Register(newMockQuality("mock-aesthetic", true, "aesthetic", 0.7))

	pl := NewPipeline(reg)
	img := image.NewGray(image.Rect(0, 0, 100, 100))

	cs, err := pl.Execute(context.Background(), img)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if cs.FaceCount != 2 {
		t.Errorf("FaceCount = %d, want 2", cs.FaceCount)
	}
	if cs.BestFaceIdx < 0 || cs.BestFaceIdx > 1 {
		t.Errorf("BestFaceIdx = %d, out of range [0, 1]", cs.BestFaceIdx)
	}
}

// TestPipeline_Execute_UnknownQualityMetric verifies that an unrecognised quality
// metric name does not cause a crash and doesn't overwrite known scores.
func TestPipeline_Execute_UnknownQualityMetric(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockDetector("mock-detector", true, nil))
	reg.Register(newMockQuality("mock-x", true, "unknown-metric", 0.5))
	reg.Register(newMockQuality("mock-sharpness", true, "sharpness", 0.8))

	pl := NewPipeline(reg)
	cs, err := pl.Execute(context.Background(), makeTestImage(64, 64))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if cs.SharpnessScore != 0.8 {
		t.Errorf("SharpnessScore = %f, want 0.8", cs.SharpnessScore)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// constructors.go — NewFaceDetectorPlugin, NewFaceEmbedderPlugin, NewAestheticPlugin
// ──────────────────────────────────────────────────────────────────────────────

// TestNewFaceDetectorPlugin_Valid verifies plugin is created and models are registered.
func TestNewFaceDetectorPlugin_Valid(t *testing.T) {
	tmp := t.TempDir()
	p, err := NewFaceDetectorPlugin(tmp)
	if err != nil {
		t.Fatalf("NewFaceDetectorPlugin: %v", err)
	}
	if p == nil {
		t.Fatal("NewFaceDetectorPlugin returned nil plugin")
	}
	if p.Name() != "face-detector" {
		t.Errorf("Name() = %q, want face-detector", p.Name())
	}
	if p.modelManager == nil {
		t.Error("modelManager should be set")
	}
	// Models should be registered.
	models := p.modelManager.RegisteredModels()
	if len(models) == 0 {
		t.Error("no models registered after NewFaceDetectorPlugin")
	}
}

// TestNewFaceDetectorPlugin_BadDir verifies error returned for unwritable path.
func TestNewFaceDetectorPlugin_BadDir(t *testing.T) {
	// Use a file as cacheDir so MkdirAll fails.
	tmp := t.TempDir()
	badPath := filepath.Join(tmp, "notadir")
	if err := os.WriteFile(badPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	badCacheDir := filepath.Join(badPath, "sub")
	_, err := NewFaceDetectorPlugin(badCacheDir)
	if err == nil {
		t.Error("NewFaceDetectorPlugin(bad dir) should return error")
	}
}

// TestNewFaceEmbedderPlugin_Valid verifies plugin is created with models registered.
func TestNewFaceEmbedderPlugin_Valid(t *testing.T) {
	tmp := t.TempDir()
	p, err := NewFaceEmbedderPlugin(tmp)
	if err != nil {
		t.Fatalf("NewFaceEmbedderPlugin: %v", err)
	}
	if p == nil {
		t.Fatal("NewFaceEmbedderPlugin returned nil plugin")
	}
	if p.Name() != "face-embedder" {
		t.Errorf("Name() = %q, want face-embedder", p.Name())
	}
}

// TestNewFaceEmbedderPlugin_BadDir verifies error for unwritable path.
func TestNewFaceEmbedderPlugin_BadDir(t *testing.T) {
	tmp := t.TempDir()
	badPath := filepath.Join(tmp, "notadir")
	if err := os.WriteFile(badPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewFaceEmbedderPlugin(filepath.Join(badPath, "sub"))
	if err == nil {
		t.Error("NewFaceEmbedderPlugin(bad dir) should return error")
	}
}

// TestNewAestheticPlugin_Valid verifies plugin is created with models registered.
func TestNewAestheticPlugin_Valid(t *testing.T) {
	tmp := t.TempDir()
	p, err := NewAestheticPlugin(tmp)
	if err != nil {
		t.Fatalf("NewAestheticPlugin: %v", err)
	}
	if p == nil {
		t.Fatal("NewAestheticPlugin returned nil plugin")
	}
	if p.Name() != "aesthetic" {
		t.Errorf("Name() = %q, want aesthetic", p.Name())
	}
}

// TestNewAestheticPlugin_BadDir verifies error for unwritable path.
func TestNewAestheticPlugin_BadDir(t *testing.T) {
	tmp := t.TempDir()
	badPath := filepath.Join(tmp, "notadir")
	if err := os.WriteFile(badPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewAestheticPlugin(filepath.Join(badPath, "sub"))
	if err == nil {
		t.Error("NewAestheticPlugin(bad dir) should return error")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// model_manager.go — DownloadAll with empty-URL skip and already-downloaded skip
// ──────────────────────────────────────────────────────────────────────────────

// TestModelManager_DownloadAll_EmptyURL verifies that a model with no URL is skipped.
func TestModelManager_DownloadAll_EmptyURL(t *testing.T) {
	tmp := t.TempDir()
	mm, err := NewModelManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	mm.Register(ModelSpec{
		Name:     "no-url-model",
		Filename: "nourl.onnx",
		URL:      "", // intentionally empty
		SHA256:   "abc",
	})

	// Should not return an error — the model is silently skipped.
	if err := mm.DownloadAll(context.Background(), nil); err != nil {
		t.Errorf("DownloadAll(empty URL) error = %v, want nil", err)
	}
}

// TestModelManager_DownloadAll_AlreadyDownloaded verifies that present models are skipped.
func TestModelManager_DownloadAll_AlreadyDownloaded(t *testing.T) {
	tmp := t.TempDir()
	mm, err := NewModelManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	mm.Register(ModelSpec{
		Name:     "present",
		Filename: "present.onnx",
		URL:      "http://localhost:1/should-not-be-fetched.onnx",
		SHA256:   "abc",
	})

	// Create the file so IsDownloaded returns true.
	if err := os.WriteFile(filepath.Join(mm.modelsDir, "present.onnx"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	// DownloadAll should skip the download entirely — no network call.
	if err := mm.DownloadAll(context.Background(), nil); err != nil {
		t.Errorf("DownloadAll(already downloaded) error = %v, want nil", err)
	}
}

// TestModelManager_DownloadAll_ProgressCallback verifies progressFn is called for
// a model that is already present (skipped model does not call progressFn).
func TestModelManager_DownloadAll_ProgressCallback(t *testing.T) {
	tmp := t.TempDir()
	mm, err := NewModelManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	// Register two models: one present (skip), one with empty URL (also skip).
	mm.Register(ModelSpec{
		Name:     "present",
		Filename: "present.onnx",
		URL:      "http://localhost:1/should-not-be-fetched.onnx",
		SHA256:   "abc",
	})
	mm.Register(ModelSpec{
		Name:     "no-url",
		Filename: "nourl.onnx",
		URL:      "",
		SHA256:   "xyz",
	})

	if err := os.WriteFile(filepath.Join(mm.modelsDir, "present.onnx"), []byte("d"), 0o600); err != nil {
		t.Fatal(err)
	}

	calls := 0
	progressFn := func(_ string, _, _ int64) { calls++ }

	if err := mm.DownloadAll(context.Background(), progressFn); err != nil {
		t.Errorf("DownloadAll error = %v, want nil", err)
	}

	// Neither model should trigger a download — progressFn should not be called.
	if calls != 0 {
		t.Errorf("progressFn called %d times, want 0 (both models skipped)", calls)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// aesthetic.go — softmax edge cases, parseNIMAOutput (no ONNX, just logic tests)
// ──────────────────────────────────────────────────────────────────────────────

// TestSoftmax_AllEqual verifies that equal logits produce a uniform distribution.
func TestSoftmax_AllEqual(t *testing.T) {
	logits := []float32{1.0, 1.0, 1.0, 1.0}
	probs := softmax(logits)
	if len(probs) != 4 {
		t.Fatalf("len(probs) = %d, want 4", len(probs))
	}
	for i, p := range probs {
		if math.Abs(float64(p)-0.25) > 1e-5 {
			t.Errorf("probs[%d] = %f, want 0.25", i, p)
		}
	}
}

// TestSoftmax_Empty verifies nil is returned for empty input.
func TestSoftmax_Empty(t *testing.T) {
	if got := softmax(nil); got != nil {
		t.Errorf("softmax(nil) = %v, want nil", got)
	}
}

// TestSoftmax_SumsToOne verifies the output is a valid probability distribution.
func TestSoftmax_SumsToOne(t *testing.T) {
	logits := []float32{2.0, 1.0, 0.1, -1.0, 3.0}
	probs := softmax(logits)
	var sum float64
	for _, p := range probs {
		sum += float64(p)
	}
	if math.Abs(sum-1.0) > 1e-5 {
		t.Errorf("softmax sum = %f, want 1.0", sum)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// weights.go — Apply edge case when Aesthetic+Sharpness are both zero
// ──────────────────────────────────────────────────────────────────────────────

// TestScoreWeights_Apply_NoFaces_BasePoolZero verifies the "split evenly" path
// when Aesthetic and Sharpness weights are both zero and there are no faces.
func TestScoreWeights_Apply_NoFaces_BasePoolZero(t *testing.T) {
	w := ScoreWeights{Aesthetic: 0, Sharpness: 0, Face: 0.5, Eyes: 0.5}
	cs := CompositeScore{
		FaceCount:      0,
		AestheticScore: 0.8,
		SharpnessScore: 0.6,
	}
	got := w.Apply(cs)
	// aestheticEff = 0.5, sharpnessEff = 0.5 → 0.5*0.8 + 0.5*0.6 = 0.70
	if math.Abs(got-0.70) > 1e-5 {
		t.Errorf("Apply(basePoolZero) = %f, want 0.70", got)
	}
}

// TestScoreWeights_Apply_WithFacesAllOnes verifies the with-faces path uses all four weights.
func TestScoreWeights_Apply_WithFacesAllOnes(t *testing.T) {
	w := ScoreWeights{Aesthetic: 0.35, Sharpness: 0.25, Face: 0.25, Eyes: 0.15}
	cs := CompositeScore{
		FaceCount:      1,
		AestheticScore: 1.0,
		SharpnessScore: 1.0,
		BestFaceSharp:  1.0,
		EyeOpenness:    1.0,
	}
	got := w.Apply(cs)
	if math.Abs(got-1.0) > 1e-5 {
		t.Errorf("Apply(all ones) = %f, want 1.0", got)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// runtime_provisioner.go — ONNXRuntimeLibName (exported wrapper)
// ──────────────────────────────────────────────────────────────────────────────

// TestONNXRuntimeLibName_Exported verifies the exported function returns a non-empty name.
func TestONNXRuntimeLibName_Exported(t *testing.T) {
	name := ONNXRuntimeLibName()
	if name == "" {
		t.Error("ONNXRuntimeLibName() returned empty string")
	}
	// Should be same as the unexported version.
	if name != onnxRuntimeLibName() {
		t.Errorf("ONNXRuntimeLibName() = %q, onnxRuntimeLibName() = %q", name, onnxRuntimeLibName())
	}
}

func TestRegistry_InitAll_RecognitionSkipped(t *testing.T) {
	r := NewRegistry()
	r.Register(newDetector("det", true))
	r.Register(newQuality("qual", true))
	r.Register(newRecognition("rec", true))
	// InitAll should not fail even though recognition plugin has no real runtime
	// Recognition failures are non-fatal (logged and skipped)
	_ = r.InitAll("")
}
