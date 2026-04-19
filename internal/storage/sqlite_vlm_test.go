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

// TestDeleteAIDataForFolderClearsVLM is the regression test for #116:
// "Clear AI Data" (which calls DeleteAIDataForFolder) must wipe every AI
// artifact for a folder, including VLM scores, rankings, and ranking groups.
// Before this test was added, VLM data survived the clear and the UI
// continued showing stale explanations after re-analysis.
func TestDeleteAIDataForFolderClearsVLM(t *testing.T) {
	store := newTestStore(t)

	folder := "/photos/vacation"
	keepFolder := "/photos/other"

	// --- Seed every table DeleteAIDataForFolder should touch. ---
	if err := store.SaveAIScore(&AIScore{
		PhotoPath: folder + "/a.jpg", OverallScore: 0.9, FaceCount: 1, Provider: "Local ONNX",
	}); err != nil {
		t.Fatalf("SaveAIScore target: %v", err)
	}
	if err := store.SaveAIScore(&AIScore{
		PhotoPath: keepFolder + "/x.jpg", OverallScore: 0.5, FaceCount: 0, Provider: "Local ONNX",
	}); err != nil {
		t.Fatalf("SaveAIScore other: %v", err)
	}

	if _, err := store.SaveFaceDetection(&FaceDetection{
		PhotoPath: folder + "/a.jpg", BboxX: 0, BboxY: 0, BboxW: 10, BboxH: 10, Confidence: 0.99,
	}); err != nil {
		t.Fatalf("SaveFaceDetection target: %v", err)
	}
	if _, err := store.SaveFaceDetection(&FaceDetection{
		PhotoPath: keepFolder + "/x.jpg", BboxX: 0, BboxY: 0, BboxW: 10, BboxH: 10, Confidence: 0.99,
	}); err != nil {
		t.Fatalf("SaveFaceDetection other: %v", err)
	}

	if _, err := store.SaveFaceCluster(&FaceCluster{
		FolderPath: folder, Label: "Person 1", PhotoCount: 1,
	}); err != nil {
		t.Fatalf("SaveFaceCluster target: %v", err)
	}
	if _, err := store.SaveFaceCluster(&FaceCluster{
		FolderPath: keepFolder, Label: "Person A", PhotoCount: 1,
	}); err != nil {
		t.Fatalf("SaveFaceCluster other: %v", err)
	}

	if err := store.SaveVLMScore(VLMScoreRow{
		PhotoPath: folder + "/a.jpg", FolderPath: folder,
		Aesthetic: 0.8, PromptVersion: 1, ScoredAt: "2026-04-09T10:00:00Z",
	}); err != nil {
		t.Fatalf("SaveVLMScore target: %v", err)
	}
	if err := store.SaveVLMScore(VLMScoreRow{
		PhotoPath: keepFolder + "/x.jpg", FolderPath: keepFolder,
		Aesthetic: 0.5, PromptVersion: 1, ScoredAt: "2026-04-09T10:00:00Z",
	}); err != nil {
		t.Fatalf("SaveVLMScore other: %v", err)
	}

	if err := store.SaveVLMRanking(VLMRankingGroupRow{
		FolderPath: folder, GroupLabel: "burst-1", PhotoCount: 2,
		Explanation: "target", ModelName: "gemma-4", PromptVersion: 1,
		Rankings: []VLMRankingRow{
			{PhotoPath: folder + "/a.jpg", Rank: 1, RelativeScore: 0.9, TokensUsed: 50},
		},
	}); err != nil {
		t.Fatalf("SaveVLMRanking target: %v", err)
	}
	if err := store.SaveVLMRanking(VLMRankingGroupRow{
		FolderPath: keepFolder, GroupLabel: "burst-2", PhotoCount: 1,
		Explanation: "other", ModelName: "gemma-4", PromptVersion: 1,
		Rankings: []VLMRankingRow{
			{PhotoPath: keepFolder + "/x.jpg", Rank: 1, RelativeScore: 0.8, TokensUsed: 50},
		},
	}); err != nil {
		t.Fatalf("SaveVLMRanking other: %v", err)
	}

	// --- Execute the clear. ---
	if err := store.DeleteAIDataForFolder(folder); err != nil {
		t.Fatalf("DeleteAIDataForFolder: %v", err)
	}

	// --- Target folder: every AI+VLM artifact must be gone. ---
	if scores, err := store.GetAIScoresForFolder(folder); err != nil {
		t.Fatalf("GetAIScoresForFolder target: %v", err)
	} else if len(scores) != 0 {
		t.Errorf("ai_scores for target folder: got %d, want 0", len(scores))
	}
	if dets, err := store.GetFaceDetections(folder + "/a.jpg"); err != nil {
		t.Fatalf("GetFaceDetections target: %v", err)
	} else if len(dets) != 0 {
		t.Errorf("face_detections for target photo: got %d, want 0", len(dets))
	}
	if clusters, err := store.GetFaceClusters(folder); err != nil {
		t.Fatalf("GetFaceClusters target: %v", err)
	} else if len(clusters) != 0 {
		t.Errorf("face_clusters for target folder: got %d, want 0", len(clusters))
	}
	if vlm, err := store.GetVLMScoresForFolder(folder); err != nil {
		t.Fatalf("GetVLMScoresForFolder target: %v", err)
	} else if len(vlm) != 0 {
		t.Errorf("vlm_scores for target folder: got %d, want 0 — #116 regression", len(vlm))
	}
	if rankings, err := store.GetVLMRankingsForFolder(folder); err != nil {
		t.Fatalf("GetVLMRankingsForFolder target: %v", err)
	} else if len(rankings) != 0 {
		t.Errorf("vlm_rankings for target folder: got %d, want 0 — #116 regression", len(rankings))
	}

	// --- Other folder: nothing may have been collateral damage. ---
	if scores, err := store.GetAIScoresForFolder(keepFolder); err != nil {
		t.Fatalf("GetAIScoresForFolder other: %v", err)
	} else if len(scores) != 1 {
		t.Errorf("ai_scores for other folder: got %d, want 1 (collateral damage)", len(scores))
	}
	if vlm, err := store.GetVLMScoresForFolder(keepFolder); err != nil {
		t.Fatalf("GetVLMScoresForFolder other: %v", err)
	} else if len(vlm) != 1 {
		t.Errorf("vlm_scores for other folder: got %d, want 1 (collateral damage)", len(vlm))
	}
	if rankings, err := store.GetVLMRankingsForFolder(keepFolder); err != nil {
		t.Fatalf("GetVLMRankingsForFolder other: %v", err)
	} else if len(rankings) != 1 {
		t.Errorf("vlm_rankings for other folder: got %d, want 1 (collateral damage)", len(rankings))
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

	// Folder with prompt_version=2 (current) AND empty hash — fresh.
	if err := store.SaveVLMScore(VLMScoreRow{
		PhotoPath:     "/photos/new/photo.jpg",
		FolderPath:    "/photos/new",
		PromptVersion: 2,
		ScoredAt:      "2026-04-09T10:00:00Z",
	}); err != nil {
		t.Fatalf("SaveVLMScore(new) failed: %v", err)
	}

	// Folder with current prompt_version but mismatched custom-instructions hash.
	if err := store.SaveVLMScore(VLMScoreRow{
		PhotoPath:              "/photos/customised/photo.jpg",
		FolderPath:             "/photos/customised",
		PromptVersion:          2,
		CustomInstructionsHash: "abc123def4567890",
		ScoredAt:               "2026-04-09T10:00:00Z",
	}); err != nil {
		t.Fatalf("SaveVLMScore(customised) failed: %v", err)
	}

	// Current hash is empty — folders with mismatching hash OR older version are stale.
	stale, err := store.GetStaleVLMFolders(2, "")
	if err != nil {
		t.Fatalf("GetStaleVLMFolders failed: %v", err)
	}
	if len(stale) != 2 {
		t.Fatalf("expected 2 stale folders, got %d: %v", len(stale), stale)
	}
	wantStale := map[string]bool{"/photos/old": false, "/photos/customised": false}
	for _, f := range stale {
		if _, ok := wantStale[f]; !ok {
			t.Errorf("unexpected stale folder %q", f)
			continue
		}
		wantStale[f] = true
	}
	for f, found := range wantStale {
		if !found {
			t.Errorf("expected %q in stale set, missing", f)
		}
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

// TestGetVLMRankingsForFolderMultipleGroups asserts that several ranking
// groups with different photo counts and rank orderings round-trip through
// the rewritten LEFT JOIN query. Previously each group was fetched in its
// own follow-up query — this test is the contract that the aggregator
// behind the single JOIN produces the same shape.
func TestGetVLMRankingsForFolderMultipleGroups(t *testing.T) {
	store := newTestStore(t)

	folder := "/photos/event"

	// Group 1: three ranked photos, out-of-insertion order to exercise
	// the ORDER BY r.rank clause.
	group1 := VLMRankingGroupRow{
		FolderPath: folder, GroupLabel: "cluster-A", PhotoCount: 3,
		Explanation: "primary cluster", ModelName: "gemma-4", PromptVersion: 1,
		Rankings: []VLMRankingRow{
			{PhotoPath: folder + "/a3.jpg", Rank: 3, RelativeScore: 0.6, TokensUsed: 40},
			{PhotoPath: folder + "/a1.jpg", Rank: 1, RelativeScore: 0.9, Notes: "sharpest", TokensUsed: 50},
			{PhotoPath: folder + "/a2.jpg", Rank: 2, RelativeScore: 0.75, TokensUsed: 45},
		},
	}
	// Group 2: two ranked photos in a different cluster, same folder.
	group2 := VLMRankingGroupRow{
		FolderPath: folder, GroupLabel: "cluster-B", PhotoCount: 2,
		Explanation: "secondary cluster", ModelName: "gemma-4", PromptVersion: 1,
		Rankings: []VLMRankingRow{
			{PhotoPath: folder + "/b1.jpg", Rank: 1, RelativeScore: 0.85, Notes: "clean", TokensUsed: 40},
			{PhotoPath: folder + "/b2.jpg", Rank: 2, RelativeScore: 0.70, TokensUsed: 40},
		},
	}
	// Group in a different folder — must not appear in results.
	other := VLMRankingGroupRow{
		FolderPath: "/photos/other", GroupLabel: "cluster-X", PhotoCount: 1,
		Explanation: "noise", ModelName: "gemma-4", PromptVersion: 1,
		Rankings: []VLMRankingRow{
			{PhotoPath: "/photos/other/x1.jpg", Rank: 1, RelativeScore: 0.5, TokensUsed: 30},
		},
	}

	for _, g := range []VLMRankingGroupRow{group1, group2, other} {
		if err := store.SaveVLMRanking(g); err != nil {
			t.Fatalf("SaveVLMRanking %s: %v", g.GroupLabel, err)
		}
	}

	got, err := store.GetVLMRankingsForFolder(folder)
	if err != nil {
		t.Fatalf("GetVLMRankingsForFolder: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 groups, got %d (%v)", len(got), got)
	}

	byLabel := map[string]VLMRankingGroupRow{}
	for _, g := range got {
		byLabel[g.GroupLabel] = g
	}

	a := byLabel["cluster-A"]
	if len(a.Rankings) != 3 {
		t.Fatalf("cluster-A: expected 3 rankings, got %d", len(a.Rankings))
	}
	for i, want := range []int{1, 2, 3} {
		if a.Rankings[i].Rank != want {
			t.Errorf("cluster-A rankings[%d].Rank = %d, want %d (ORDER BY rank lost)",
				i, a.Rankings[i].Rank, want)
		}
	}
	if a.Rankings[0].Notes != "sharpest" {
		t.Errorf("cluster-A rankings[0].Notes = %q, want \"sharpest\"", a.Rankings[0].Notes)
	}

	b := byLabel["cluster-B"]
	if len(b.Rankings) != 2 {
		t.Fatalf("cluster-B: expected 2 rankings, got %d", len(b.Rankings))
	}
	if b.Rankings[0].PhotoPath != folder+"/b1.jpg" {
		t.Errorf("cluster-B rankings[0].PhotoPath = %q, want %q",
			b.Rankings[0].PhotoPath, folder+"/b1.jpg")
	}
}

// TestGetVLMRankingsForFolderEmptyGroup covers the LEFT JOIN edge case: a
// group row with no matching ranking rows produces one tuple with NULL
// ranking columns. The aggregator must leave Rankings nil rather than
// appending a phantom zero-valued VLMRankingRow.
func TestGetVLMRankingsForFolderEmptyGroup(t *testing.T) {
	store := newTestStore(t)

	folder := "/photos/empty-group"
	// Insert the group row directly without any ranking rows so the
	// LEFT JOIN has to synthesize NULLs on the right side.
	store.mu.Lock()
	_, err := store.db.Exec(
		`INSERT INTO vlm_ranking_groups (folder_path, group_label, photo_count, explanation, model_name, prompt_version)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		folder, "orphan", 0, "seeded for edge-case test", "gemma-4", 1,
	)
	store.mu.Unlock()
	if err != nil {
		t.Fatalf("seed orphan group: %v", err)
	}

	got, err := store.GetVLMRankingsForFolder(folder)
	if err != nil {
		t.Fatalf("GetVLMRankingsForFolder: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 group, got %d", len(got))
	}
	if len(got[0].Rankings) != 0 {
		t.Errorf("empty group produced %d rankings, want 0 (LEFT JOIN NULL leaked)",
			len(got[0].Rankings))
	}
}

// TestGetVLMRankingsForFolderNoGroups asserts an empty result for a folder
// with no ranking groups at all — not an error, just a zero-length slice.
func TestGetVLMRankingsForFolderNoGroups(t *testing.T) {
	store := newTestStore(t)

	got, err := store.GetVLMRankingsForFolder("/no/such/folder")
	if err != nil {
		t.Fatalf("GetVLMRankingsForFolder: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 groups, got %d", len(got))
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
