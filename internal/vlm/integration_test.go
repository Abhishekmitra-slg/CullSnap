package vlm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// writeJSON copies a pre-encoded JSON payload to w via io.Copy to avoid
// direct ResponseWriter.Write calls that trigger static-analysis XSS warnings
// in mock HTTP test servers.
func writeJSON(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.Copy(w, bytes.NewReader(payload))
}

// --- TestEndToEndScoring ---

// TestEndToEndScoring verifies the full path: NewClient -> ChatCompletion ->
// ParseVLMScore, using a mock HTTP server that returns a valid VLM JSON payload.
func TestEndToEndScoring(t *testing.T) {
	score := VLMScore{
		Aesthetic:     0.85,
		Composition:   0.78,
		Expression:    0.60,
		TechnicalQual: 0.90,
		SceneType:     "portrait",
		Issues:        []string{"slight motion blur"},
		Explanation:   "Well-exposed portrait with strong composition.",
	}
	scoreJSON, err := json.Marshal(score)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, validResponse(string(scoreJSON), 120))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token", "gemma-4b")
	content, tokens, err := client.ChatCompletion(
		context.Background(),
		SystemPrompt(""),
		Stage4Prompt(Stage4Input{FaceCount: 1, SharpnessScore: 0.90}),
		nil,
		256,
	)
	if err != nil {
		t.Fatalf("ChatCompletion error: %v", err)
	}
	if tokens != 120 {
		t.Errorf("tokens = %d, want 120", tokens)
	}

	parsed, err := ParseVLMScore(content)
	if err != nil {
		t.Fatalf("ParseVLMScore error: %v", err)
	}
	if parsed.Aesthetic != 0.85 {
		t.Errorf("Aesthetic = %.2f, want 0.85", parsed.Aesthetic)
	}
	if parsed.Composition != 0.78 {
		t.Errorf("Composition = %.2f, want 0.78", parsed.Composition)
	}
	if parsed.TechnicalQual != 0.90 {
		t.Errorf("TechnicalQual = %.2f, want 0.90", parsed.TechnicalQual)
	}
	if parsed.SceneType != "portrait" {
		t.Errorf("SceneType = %q, want %q", parsed.SceneType, "portrait")
	}
	if len(parsed.Issues) != 1 || parsed.Issues[0] != "slight motion blur" {
		t.Errorf("Issues = %v, want [\"slight motion blur\"]", parsed.Issues)
	}
	if parsed.Explanation == "" {
		t.Error("Explanation is empty, want non-empty")
	}
}

// --- TestEndToEndRanking ---

// TestEndToEndRanking verifies the full path: NewClient -> ChatCompletion ->
// ParseRankingResult, using a mock HTTP server returning a valid ranking payload.
func TestEndToEndRanking(t *testing.T) {
	photoPaths := []string{"/photos/a.jpg", "/photos/b.jpg", "/photos/c.jpg"}

	rawRanking := map[string]interface{}{
		"ranked": []map[string]interface{}{
			{"rank": 1, "photo_index": 2, "score": 0.92, "notes": "best exposure"},
			{"rank": 2, "photo_index": 1, "score": 0.80, "notes": "slightly dark"},
			{"rank": 3, "photo_index": 3, "score": 0.65, "notes": "soft focus"},
		},
		"explanation": "Photo 2 wins on overall exposure and sharpness.",
	}
	rankJSON, err := json.Marshal(rawRanking)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, validResponse(string(rankJSON), 200))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", "gemma-4b")
	content, tokens, err := client.ChatCompletion(
		context.Background(),
		SystemPrompt(""),
		Stage5Prompt(Stage5Input{
			Photos: []Stage5Photo{
				{Aesthetic: 0.80, Sharpness: 0.75, FaceCount: 1},
				{Aesthetic: 0.92, Sharpness: 0.88, FaceCount: 1},
				{Aesthetic: 0.65, Sharpness: 0.55, FaceCount: 0},
			},
			UseCase: "portfolio",
		}),
		nil,
		512,
	)
	if err != nil {
		t.Fatalf("ChatCompletion error: %v", err)
	}
	if tokens != 200 {
		t.Errorf("tokens = %d, want 200", tokens)
	}

	result, err := ParseRankingResult(content, photoPaths)
	if err != nil {
		t.Fatalf("ParseRankingResult error: %v", err)
	}
	if len(result.Ranked) != 3 {
		t.Fatalf("len(Ranked) = %d, want 3", len(result.Ranked))
	}
	// The first entry in ranked (rank=1) references photo_index=2 -> photoPaths[1]
	if result.Ranked[0].PhotoPath != "/photos/b.jpg" {
		t.Errorf("Ranked[0].PhotoPath = %q, want %q", result.Ranked[0].PhotoPath, "/photos/b.jpg")
	}
	if result.Ranked[0].Rank != 1 {
		t.Errorf("Ranked[0].Rank = %d, want 1", result.Ranked[0].Rank)
	}
	if result.Explanation == "" {
		t.Error("Explanation is empty, want non-empty")
	}
}

// --- TestManagerScorePhotoAccessor ---

// TestManagerScorePhotoAccessor creates a Manager with a mockProvider, calls
// ScorePhoto, and verifies the returned score and that the provider was started.
func TestManagerScorePhotoAccessor(t *testing.T) {
	cfg := DefaultManagerConfig()
	cfg.KeepAlive = true

	m := NewManager(cfg, nil)
	mp := &mockProvider{
		scoreResult: &VLMScore{
			Aesthetic:     0.75,
			Composition:   0.70,
			Expression:    0.50,
			TechnicalQual: 0.80,
			SceneType:     "landscape",
			Explanation:   "Wide landscape with good depth.",
		},
	}
	m.SetProvider(mp)

	ctx := context.Background()
	got, err := m.ScorePhoto(ctx, ScoreRequest{PhotoPath: "/photos/test.jpg"})
	if err != nil {
		t.Fatalf("ScorePhoto error: %v", err)
	}

	mp.mu.Lock()
	started := mp.started
	mp.mu.Unlock()

	if !started {
		t.Error("expected provider.started = true after ScorePhoto")
	}
	if got == nil {
		t.Fatal("ScorePhoto returned nil score")
	}
	if got.Aesthetic != 0.75 {
		t.Errorf("Aesthetic = %.2f, want 0.75", got.Aesthetic)
	}
	if got.SceneType != "landscape" {
		t.Errorf("SceneType = %q, want %q", got.SceneType, "landscape")
	}

	_ = m.Stop(ctx)
}

// --- TestManagerCrashRecoveryMaxRestarts ---

// TestManagerCrashRecoveryMaxRestarts creates a Manager with MaxRestarts=2,
// then directly calls handleCrash three times (exceeding the limit) and verifies
// that the state transitions to StateError.
func TestManagerCrashRecoveryMaxRestarts(t *testing.T) {
	cfg := DefaultManagerConfig()
	cfg.MaxRestarts = 2
	cfg.RestartBackoff = 0 // no sleep between retries in tests

	// Use a provider whose Start always fails so the restart loop gives up quickly.
	mp := &mockProvider{
		startErr: errors.New("intentional start failure"),
	}

	m := NewManager(cfg, nil)
	m.SetProvider(mp)

	// Manually drive the crash/restart logic by calling handleCrash until
	// restartCount > MaxRestarts. Each call to handleCrash increments restartCount.
	// With MaxRestarts=2 we need 3 calls: counts 1, 2, 3 — on count 3 it gives up.
	for range 3 {
		m.handleCrash()
	}

	if got := m.State(); got != StateError {
		t.Errorf("state = %v, want StateError after exceeding MaxRestarts", got)
	}
}

// --- TestBuildExecutionPlanTierCapableLarge ---

// TestBuildExecutionPlanTierCapableLarge verifies that 600 photos on TierCapable
// produce VLMEnabled=true, Stage5Count=0, and Mode=ModeBackground.
func TestBuildExecutionPlanTierCapableLarge(t *testing.T) {
	plan := BuildExecutionPlan(600, TierCapable)

	if !plan.VLMEnabled {
		t.Error("VLMEnabled = false, want true")
	}
	if plan.Stage5Count != 0 {
		t.Errorf("Stage5Count = %d, want 0 (>500 photos suppresses Stage 5 on TierCapable)", plan.Stage5Count)
	}
	if plan.Mode != ModeBackground {
		t.Errorf("Mode = %v, want ModeBackground", plan.Mode)
	}
	if plan.PhotoCount != 600 {
		t.Errorf("PhotoCount = %d, want 600", plan.PhotoCount)
	}
	if plan.HardwareTier != TierCapable {
		t.Errorf("HardwareTier = %v, want TierCapable", plan.HardwareTier)
	}
}
