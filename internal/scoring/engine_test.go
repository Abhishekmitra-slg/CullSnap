package scoring

import (
	"context"
	"errors"
	"image"
	"testing"
)

// mockProvider is a test double for ScoringProvider used in engine tests.
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

func TestEngine_RegisterAndScore(t *testing.T) {
	engine := NewEngine()

	mock := &mockProvider{
		name:      "test",
		available: true,
		scores: &ScoreResult{
			Faces:        []FaceRegion{{BoundingBox: image.Rect(10, 10, 50, 50), Confidence: 0.95}},
			OverallScore: 0.85,
		},
	}

	engine.Register(mock)

	result, err := engine.Score(context.Background(), []byte("fake-jpeg"))
	if err != nil {
		t.Fatalf("Score failed: %v", err)
	}
	if result == nil {
		t.Fatal("Score returned nil result")
	}
	if result.OverallScore != 0.85 {
		t.Errorf("OverallScore = %f, want 0.85", result.OverallScore)
	}
}

func TestEngine_NoProviders(t *testing.T) {
	engine := NewEngine()

	result, err := engine.Score(context.Background(), []byte("fake-jpeg"))
	if err != nil {
		t.Fatalf("Score should not error with no providers: %v", err)
	}
	if result != nil {
		t.Error("Score should return nil result with no providers")
	}
}

func TestEngine_UnavailableProvider(t *testing.T) {
	engine := NewEngine()

	mock := &mockProvider{name: "unavailable", available: false}
	engine.Register(mock)

	result, err := engine.Score(context.Background(), []byte("fake-jpeg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("should return nil when provider unavailable")
	}
}

func TestEngine_Fallback(t *testing.T) {
	engine := NewEngine()

	// First provider unavailable.
	engine.Register(&mockProvider{name: "cloud", available: false})

	// Second provider available.
	engine.Register(&mockProvider{
		name:      "local",
		available: true,
		scores:    &ScoreResult{OverallScore: 0.7},
	})

	result, err := engine.Score(context.Background(), []byte("fake-jpeg"))
	if err != nil {
		t.Fatalf("Score failed: %v", err)
	}
	if result == nil || result.OverallScore != 0.7 {
		t.Errorf("should fall back to local provider, got %v", result)
	}
}

func TestEngine_FallbackOnError(t *testing.T) {
	engine := NewEngine()

	// First provider errors.
	engine.Register(&mockProvider{
		name:      "flaky",
		available: true,
		err:       errors.New("connection timeout"),
	})

	// Second provider works.
	engine.Register(&mockProvider{
		name:      "reliable",
		available: true,
		scores:    &ScoreResult{OverallScore: 0.6},
	})

	result, err := engine.Score(context.Background(), []byte("fake-jpeg"))
	if err != nil {
		t.Fatalf("Score failed: %v", err)
	}
	if result == nil || result.OverallScore != 0.6 {
		t.Errorf("should fall back on error, got %v", result)
	}
}

func TestEngine_Enabled(t *testing.T) {
	engine := NewEngine()
	if engine.Enabled() {
		t.Error("should not be enabled with no providers")
	}

	engine.Register(&mockProvider{name: "test", available: true})
	if !engine.Enabled() {
		t.Error("should be enabled with available provider")
	}
}

func TestEngine_Providers(t *testing.T) {
	engine := NewEngine()
	engine.Register(&mockProvider{name: "local", available: true})
	engine.Register(&mockProvider{name: "cloud", available: false})

	statuses := engine.Providers()
	if len(statuses) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(statuses))
	}
	if statuses[0].Name != "local" || !statuses[0].Available {
		t.Errorf("provider 0: got %+v", statuses[0])
	}
	if statuses[1].Name != "cloud" || statuses[1].Available {
		t.Errorf("provider 1: got %+v", statuses[1])
	}
}
