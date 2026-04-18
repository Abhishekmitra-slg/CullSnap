package storage

import (
	"testing"
)

func TestSaveAndGetVLMScore(t *testing.T) {
	store := newTestStore(t)

	score := VLMScoreRow{
		PhotoPath:     "/photos/test.jpg",
		FolderPath:    "/photos",
		Aesthetic:     0.85,
		Composition:   0.72,
		Expression:    0.60,
		TechnicalQual: 0.90,
		SceneType:     "portrait",
		Issues:        `["slight blur"]`,
		Explanation:   "Good portrait with minor blur.",
		TokensUsed:    256,
		ModelName:     "gemma-4",
		ModelVariant:  "4b",
		Backend:       "mlx",
		PromptVersion: 1,
		ScoredAt:      "2026-04-09T10:00:00Z",
	}

	if err := store.SaveVLMScore(score); err != nil {
		t.Fatalf("SaveVLMScore failed: %v", err)
	}

	got, err := store.GetVLMScore("/photos/test.jpg")
	if err != nil {
		t.Fatalf("GetVLMScore failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetVLMScore returned nil")
	}
	if got.Aesthetic != 0.85 {
		t.Errorf("Aesthetic = %f, want 0.85", got.Aesthetic)
	}
	if got.Composition != 0.72 {
		t.Errorf("Composition = %f, want 0.72", got.Composition)
	}
	if got.SceneType != "portrait" {
		t.Errorf("SceneType = %q, want \"portrait\"", got.SceneType)
	}
	if got.Issues != `["slight blur"]` {
		t.Errorf("Issues = %q, want [\"slight blur\"]", got.Issues)
	}
	if got.TokensUsed != 256 {
		t.Errorf("TokensUsed = %d, want 256", got.TokensUsed)
	}
	if got.ModelName != "gemma-4" {
		t.Errorf("ModelName = %q, want \"gemma-4\"", got.ModelName)
	}
	if got.PromptVersion != 1 {
		t.Errorf("PromptVersion = %d, want 1", got.PromptVersion)
	}
}

func TestSaveVLMScoreUpsert(t *testing.T) {
	store := newTestStore(t)

	first := VLMScoreRow{
		PhotoPath:     "/photos/upsert.jpg",
		FolderPath:    "/photos",
		Aesthetic:     0.50,
		PromptVersion: 1,
		ScoredAt:      "2026-04-09T10:00:00Z",
	}
	if err := store.SaveVLMScore(first); err != nil {
		t.Fatalf("first SaveVLMScore failed: %v", err)
	}

	second := VLMScoreRow{
		PhotoPath:     "/photos/upsert.jpg",
		FolderPath:    "/photos",
		Aesthetic:     0.95,
		PromptVersion: 2,
		ScoredAt:      "2026-04-09T11:00:00Z",
	}
	if err := store.SaveVLMScore(second); err != nil {
		t.Fatalf("second SaveVLMScore failed: %v", err)
	}

	got, err := store.GetVLMScore("/photos/upsert.jpg")
	if err != nil {
		t.Fatalf("GetVLMScore failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetVLMScore returned nil")
	}
	if got.Aesthetic != 0.95 {
		t.Errorf("Aesthetic = %f, want 0.95 (upsert should overwrite)", got.Aesthetic)
	}
	if got.PromptVersion != 2 {
		t.Errorf("PromptVersion = %d, want 2", got.PromptVersion)
	}
}

func TestGetVLMScoresForFolder(t *testing.T) {
	store := newTestStore(t)

	folder := "/photos/vacation"
	paths := []string{"/photos/vacation/a.jpg", "/photos/vacation/b.jpg", "/photos/vacation/c.jpg"}
	for i, p := range paths {
		score := VLMScoreRow{
			PhotoPath:     p,
			FolderPath:    folder,
			Aesthetic:     float64(i) * 0.1,
			PromptVersion: 1,
			ScoredAt:      "2026-04-09T10:00:00Z",
		}
		if err := store.SaveVLMScore(score); err != nil {
			t.Fatalf("SaveVLMScore(%s) failed: %v", p, err)
		}
	}

	// Also save one in a different folder — should not appear.
	if err := store.SaveVLMScore(VLMScoreRow{
		PhotoPath:  "/photos/other/x.jpg",
		FolderPath: "/photos/other",
		ScoredAt:   "2026-04-09T10:00:00Z",
	}); err != nil {
		t.Fatalf("SaveVLMScore other failed: %v", err)
	}

	scores, err := store.GetVLMScoresForFolder(folder)
	if err != nil {
		t.Fatalf("GetVLMScoresForFolder failed: %v", err)
	}
	if len(scores) != 3 {
		t.Errorf("got %d scores, want 3", len(scores))
	}
}

func TestGetVLMScoreNotFound(t *testing.T) {
	store := newTestStore(t)

	got, err := store.GetVLMScore("/nonexistent/photo.jpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing photo, got %+v", got)
	}
}

func TestDeleteVLMDataForFolder(t *testing.T) {
	store := newTestStore(t)

	folder := "/photos/delete-me"
	if err := store.SaveVLMScore(VLMScoreRow{
		PhotoPath:  "/photos/delete-me/photo.jpg",
		FolderPath: folder,
		ScoredAt:   "2026-04-09T10:00:00Z",
	}); err != nil {
		t.Fatalf("SaveVLMScore failed: %v", err)
	}

	if err := store.DeleteVLMDataForFolder(folder); err != nil {
		t.Fatalf("DeleteVLMDataForFolder failed: %v", err)
	}

	scores, err := store.GetVLMScoresForFolder(folder)
	if err != nil {
		t.Fatalf("GetVLMScoresForFolder after delete failed: %v", err)
	}
	if len(scores) != 0 {
		t.Errorf("expected 0 scores after delete, got %d", len(scores))
	}

	got, err := store.GetVLMScore("/photos/delete-me/photo.jpg")
	if err != nil {
		t.Fatalf("GetVLMScore after delete failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}

func TestGetStaleVLMFolders(t *testing.T) {
	store := newTestStore(t)

	// Folder with prompt_version=1 (stale relative to version 2).
	if err := store.SaveVLMScore(VLMScoreRow{
		PhotoPath:     "/photos/old/photo.jpg",
		FolderPath:    "/photos/old",
		PromptVersion: 1,
		ScoredAt:      "2026-04-09T10:00:00Z",
	}); err != nil {
		t.Fatalf("SaveVLMScore(old) failed: %v", err)
	}

	// Folder with prompt_version=2 (current).
	if err := store.SaveVLMScore(VLMScoreRow{
		PhotoPath:     "/photos/new/photo.jpg",
		FolderPath:    "/photos/new",
		PromptVersion: 2,
		ScoredAt:      "2026-04-09T10:00:00Z",
	}); err != nil {
		t.Fatalf("SaveVLMScore(new) failed: %v", err)
	}

	stale, err := store.GetStaleVLMFolders(2)
	if err != nil {
		t.Fatalf("GetStaleVLMFolders failed: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale folder, got %d: %v", len(stale), stale)
	}
	if stale[0] != "/photos/old" {
		t.Errorf("stale folder = %q, want \"/photos/old\"", stale[0])
	}
}

func TestSaveAndGetVLMRanking(t *testing.T) {
	store := newTestStore(t)

	group := VLMRankingGroupRow{
		FolderPath:    "/photos/burst",
		GroupLabel:    "burst-001",
		PhotoCount:    3,
		Explanation:   "Best burst selected.",
		ModelName:     "gemma-4",
		PromptVersion: 1,
		Rankings: []VLMRankingRow{
			{PhotoPath: "/photos/burst/a.jpg", Rank: 1, RelativeScore: 0.95, Notes: "sharpest", TokensUsed: 100},
			{PhotoPath: "/photos/burst/b.jpg", Rank: 2, RelativeScore: 0.80, Notes: "good", TokensUsed: 100},
			{PhotoPath: "/photos/burst/c.jpg", Rank: 3, RelativeScore: 0.65, Notes: "acceptable", TokensUsed: 100},
		},
	}

	if err := store.SaveVLMRanking(group); err != nil {
		t.Fatalf("SaveVLMRanking failed: %v", err)
	}

	groups, err := store.GetVLMRankingsForFolder("/photos/burst")
	if err != nil {
		t.Fatalf("GetVLMRankingsForFolder failed: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	got := groups[0]
	if got.GroupLabel != "burst-001" {
		t.Errorf("GroupLabel = %q, want \"burst-001\"", got.GroupLabel)
	}
	if got.PhotoCount != 3 {
		t.Errorf("PhotoCount = %d, want 3", got.PhotoCount)
	}
	if len(got.Rankings) != 3 {
		t.Fatalf("expected 3 rankings, got %d", len(got.Rankings))
	}
	if got.Rankings[0].PhotoPath != "/photos/burst/a.jpg" {
		t.Errorf("Rankings[0].PhotoPath = %q, want \"/photos/burst/a.jpg\"", got.Rankings[0].PhotoPath)
	}
	if got.Rankings[0].Rank != 1 {
		t.Errorf("Rankings[0].Rank = %d, want 1", got.Rankings[0].Rank)
	}
	if got.Rankings[0].RelativeScore != 0.95 {
		t.Errorf("Rankings[0].RelativeScore = %f, want 0.95", got.Rankings[0].RelativeScore)
	}
}

func TestRecordTokenUsage(t *testing.T) {
	store := newTestStore(t)

	usage := TokenUsageRow{
		Provider:     "mlx",
		FolderPath:   "/photos/vacation",
		Stage:        "scoring",
		TokensInput:  512,
		TokensOutput: 128,
		PhotoCount:   10,
	}
	if err := store.RecordTokenUsage(usage); err != nil {
		t.Fatalf("RecordTokenUsage failed: %v", err)
	}

	summaries, err := store.GetTokenUsageSummary()
	if err != nil {
		t.Fatalf("GetTokenUsageSummary failed: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	s := summaries[0]
	if s.Provider != "mlx" {
		t.Errorf("Provider = %q, want \"mlx\"", s.Provider)
	}
	if s.TotalInput != 512 {
		t.Errorf("TotalInput = %d, want 512", s.TotalInput)
	}
	if s.TotalOutput != 128 {
		t.Errorf("TotalOutput = %d, want 128", s.TotalOutput)
	}
	if s.TotalPhotos != 10 {
		t.Errorf("TotalPhotos = %d, want 10", s.TotalPhotos)
	}
	if s.SessionCount != 1 {
		t.Errorf("SessionCount = %d, want 1", s.SessionCount)
	}
}
