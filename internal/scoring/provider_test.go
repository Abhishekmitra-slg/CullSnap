package scoring

import (
	"context"
	"image"
	"testing"
)

// mockProvider is a test implementation of ScoringProvider.
type mockProvider struct {
	name      string
	available bool
	scores    *ScoreResult
	err       error
}

func (m *mockProvider) Name() string           { return m.name }
func (m *mockProvider) Available() bool        { return m.available }
func (m *mockProvider) RequiresAPIKey() bool   { return false }
func (m *mockProvider) RequiresDownload() bool { return false }
func (m *mockProvider) Score(_ context.Context, _ []byte) (*ScoreResult, error) {
	return m.scores, m.err
}

func TestMockProviderImplementsInterface(t *testing.T) {
	var _ ScoringProvider = (*mockProvider)(nil)
}

func TestScoreResult_BestFace(t *testing.T) {
	result := &ScoreResult{
		Faces: []FaceRegion{
			{BoundingBox: image.Rect(0, 0, 100, 100), EyeSharpness: 0.5, Confidence: 0.9},
			{BoundingBox: image.Rect(200, 200, 300, 300), EyeSharpness: 0.9, Confidence: 0.95},
		},
		OverallScore: 0.8,
	}

	best := result.BestFace()
	if best == nil {
		t.Fatal("BestFace returned nil")
	}
	if best.EyeSharpness != 0.9 {
		t.Errorf("BestFace().EyeSharpness = %f, want 0.9", best.EyeSharpness)
	}
}

func TestScoreResult_BestFace_Empty(t *testing.T) {
	result := &ScoreResult{Faces: nil}
	if result.BestFace() != nil {
		t.Error("BestFace should return nil for empty faces")
	}
}

func TestScoreResult_HasFaces(t *testing.T) {
	withFaces := &ScoreResult{Faces: []FaceRegion{{Confidence: 0.9}}}
	if !withFaces.HasFaces() {
		t.Error("HasFaces should return true")
	}

	withoutFaces := &ScoreResult{Faces: nil}
	if withoutFaces.HasFaces() {
		t.Error("HasFaces should return false")
	}
}
