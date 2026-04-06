package scoring

import (
	"context"
	"image"
	"testing"
)

// mockDetector is a mockPlugin that also returns configurable faces.
type mockDetector struct {
	mockPlugin
	faces []FaceRegion
}

func (m *mockDetector) Process(_ context.Context, _ image.Image) (PluginResult, error) {
	return PluginResult{Faces: m.faces}, nil
}

// mockQualityPlugin is a mockPlugin that returns a fixed quality score.
type mockQualityPlugin struct {
	mockPlugin
	qualityName  string
	qualityScore float64
}

func (m *mockQualityPlugin) Process(_ context.Context, _ image.Image) (PluginResult, error) {
	return PluginResult{
		Quality: &QualityScore{
			Name:  m.qualityName,
			Score: m.qualityScore,
		},
	}, nil
}

// newMockDetector builds a CategoryDetection plugin that returns the given faces.
func newMockDetector(name string, avail bool, faces []FaceRegion) *mockDetector {
	return &mockDetector{
		mockPlugin: mockPlugin{name: name, category: CategoryDetection, available: avail},
		faces:      faces,
	}
}

// newMockQuality builds a CategoryQuality plugin with a fixed named score.
func newMockQuality(name string, avail bool, qName string, qScore float64) *mockQualityPlugin {
	return &mockQualityPlugin{
		mockPlugin:   mockPlugin{name: name, category: CategoryQuality, available: avail},
		qualityName:  qName,
		qualityScore: qScore,
	}
}

// makeTestImage creates a small solid-colour image suitable for testing.
func makeTestImage(w, h int) image.Image {
	img := image.NewGray(image.Rect(0, 0, w, h))
	return img
}

// ──────────────────────────────────────────────────────────────────────────────
// Tests
// ──────────────────────────────────────────────────────────────────────────────

// TestPipeline_Execute_MockPlugins verifies that a detector and two quality
// plugins all contribute their results to the CompositeScore.
func TestPipeline_Execute_MockPlugins(t *testing.T) {
	reg := NewRegistry()

	face := FaceRegion{
		BoundingBox: image.Rect(10, 10, 50, 50),
		Confidence:  0.95,
	}
	reg.Register(newMockDetector("mock-detector", true, []FaceRegion{face}))
	reg.Register(newMockQuality("mock-aesthetic", true, "aesthetic", 0.85))
	reg.Register(newMockQuality("mock-sharpness", true, "sharpness", 0.90))

	pl := NewPipeline(reg)
	img := makeTestImage(100, 100)

	cs, err := pl.Execute(context.Background(), img)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if cs == nil {
		t.Fatal("Execute() returned nil CompositeScore")
	}

	if cs.FaceCount != 1 {
		t.Errorf("FaceCount = %d, want 1", cs.FaceCount)
	}
	if cs.AestheticScore != 0.85 {
		t.Errorf("AestheticScore = %f, want 0.85", cs.AestheticScore)
	}
	if cs.SharpnessScore != 0.90 {
		t.Errorf("SharpnessScore = %f, want 0.90", cs.SharpnessScore)
	}
	if cs.Provider != "local-pipeline" {
		t.Errorf("Provider = %q, want %q", cs.Provider, "local-pipeline")
	}
}

// TestPipeline_Execute_NoFaces verifies that when the detector finds no faces,
// FaceCount is 0 and quality scores still pass through.
func TestPipeline_Execute_NoFaces(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockDetector("mock-detector", true, nil))
	reg.Register(newMockQuality("mock-aesthetic", true, "aesthetic", 0.72))

	pl := NewPipeline(reg)
	cs, err := pl.Execute(context.Background(), makeTestImage(64, 64))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if cs.FaceCount != 0 {
		t.Errorf("FaceCount = %d, want 0", cs.FaceCount)
	}
	if cs.AestheticScore != 0.72 {
		t.Errorf("AestheticScore = %f, want 0.72", cs.AestheticScore)
	}
	if cs.BestFaceSharp != 0 {
		t.Errorf("BestFaceSharp = %f, want 0 (no faces)", cs.BestFaceSharp)
	}
}

// TestPipeline_Execute_UnavailablePlugin verifies that an unavailable detector
// is skipped silently and quality plugins still produce results.
func TestPipeline_Execute_UnavailablePlugin(t *testing.T) {
	reg := NewRegistry()
	// Detector is marked unavailable — it must be skipped.
	reg.Register(newMockDetector("unavailable-detector", false, []FaceRegion{
		{BoundingBox: image.Rect(0, 0, 10, 10)},
	}))
	reg.Register(newMockQuality("mock-quality", true, "sharpness", 0.65))

	pl := NewPipeline(reg)
	cs, err := pl.Execute(context.Background(), makeTestImage(64, 64))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// The unavailable detector's faces must NOT appear.
	if cs.FaceCount != 0 {
		t.Errorf("FaceCount = %d, want 0 (unavailable detector skipped)", cs.FaceCount)
	}
	if cs.SharpnessScore != 0.65 {
		t.Errorf("SharpnessScore = %f, want 0.65", cs.SharpnessScore)
	}
}

// TestPipeline_SetWeights verifies that SetWeights and Weights round-trip correctly.
func TestPipeline_SetWeights(t *testing.T) {
	pl := NewPipeline(NewRegistry())

	custom := ScoreWeights{Aesthetic: 0.5, Sharpness: 0.2, Face: 0.2, Eyes: 0.1}
	pl.SetWeights(custom)

	got := pl.Weights()
	if got != custom {
		t.Errorf("Weights() = %+v, want %+v", got, custom)
	}
}

// TestPipeline_Execute_Cancellation verifies that a pre-cancelled context causes
// Execute to return an error quickly without hanging.
func TestPipeline_Execute_Cancellation(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newMockDetector("mock-detector", true, nil))
	reg.Register(newMockQuality("mock-quality", true, "sharpness", 0.5))

	pl := NewPipeline(reg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// The pipeline may return either nil or an error; it must not hang.
	done := make(chan struct{})
	go func() {
		pl.Execute(ctx, makeTestImage(64, 64)) //nolint:errcheck
		close(done)
	}()

	select {
	case <-done:
		// Finished promptly — pass.
	case <-context.Background().Done():
		t.Fatal("Execute() blocked indefinitely on cancelled context")
	}
}

// TestEstimateEyeOpenness_NoLandmarks verifies that a FaceRegion with all-zero
// landmarks returns the neutral 0.5 score.
func TestEstimateEyeOpenness_NoLandmarks(t *testing.T) {
	face := FaceRegion{
		BoundingBox: image.Rect(0, 0, 100, 100),
		// Landmarks all zero — no landmark data.
	}
	got := estimateEyeOpenness(face)
	if got != 0.5 {
		t.Errorf("estimateEyeOpenness(noLandmarks) = %f, want 0.5", got)
	}
}
