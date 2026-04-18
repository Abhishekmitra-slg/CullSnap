package vlm

import (
	"testing"
)

func TestParseVLMScoreCleanJSON(t *testing.T) {
	raw := `{
		"aesthetic": 0.75,
		"composition": 0.80,
		"expression": 0.70,
		"technical_quality": 0.65,
		"scene_type": "portrait",
		"issues": ["slight overexposure"],
		"explanation": "Well-composed portrait with good lighting.",
		"tokens_used": 200
	}`
	score, err := ParseVLMScore(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score.Aesthetic != 0.75 {
		t.Errorf("aesthetic: got %.2f, want 0.75", score.Aesthetic)
	}
	if score.SceneType != "portrait" {
		t.Errorf("scene_type: got %q, want %q", score.SceneType, "portrait")
	}
	if len(score.Issues) != 1 {
		t.Errorf("issues count: got %d, want 1", len(score.Issues))
	}
}

func TestParseVLMScoreMarkdownWrapped(t *testing.T) {
	raw := "Here is the analysis:\n```json\n{\n\t\"aesthetic\": 0.5,\n\t\"composition\": 0.6,\n\t\"expression\": 0.4,\n\t\"technical_quality\": 0.55,\n\t\"scene_type\": \"landscape\",\n\t\"issues\": [],\n\t\"explanation\": \"Decent landscape shot.\",\n\t\"tokens_used\": 150\n}\n```\nEnd of response."
	score, err := ParseVLMScore(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score.Aesthetic != 0.5 {
		t.Errorf("aesthetic: got %.2f, want 0.50", score.Aesthetic)
	}
	if score.SceneType != "landscape" {
		t.Errorf("scene_type: got %q, want %q", score.SceneType, "landscape")
	}
}

func TestParseVLMScoreMalformed(t *testing.T) {
	raw := "This is just plain text with no JSON whatsoever."
	_, err := ParseVLMScore(raw)
	if err == nil {
		t.Fatal("expected error for malformed input, got nil")
	}
}

func TestParseRankingResultCleanJSON(t *testing.T) {
	raw := `{
		"ranked": [
			{"photo_index": 1, "rank": 1, "score": 0.9, "notes": "Best shot"},
			{"photo_index": 2, "rank": 2, "score": 0.7, "notes": "Good but busy"}
		],
		"explanation": "Photo 1 is sharper and better exposed.",
		"tokens_used": 300
	}`
	paths := []string{"/photos/img001.jpg", "/photos/img002.jpg"}
	result, err := ParseRankingResult(raw, paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Ranked) != 2 {
		t.Fatalf("ranked count: got %d, want 2", len(result.Ranked))
	}
	first := result.Ranked[0]
	if first.PhotoPath != "/photos/img001.jpg" {
		t.Errorf("photo_path: got %q, want %q", first.PhotoPath, "/photos/img001.jpg")
	}
	if first.Rank != 1 {
		t.Errorf("rank: got %d, want 1", first.Rank)
	}
}

func TestParseRankingResultMarkdownWrapped(t *testing.T) {
	raw := "Ranking result:\n```json\n{\n\t\"ranked\": [\n\t\t{\"photo_index\": 1, \"rank\": 1, \"score\": 0.85, \"notes\": \"Top pick\"}\n\t],\n\t\"explanation\": \"Only one photo evaluated.\",\n\t\"tokens_used\": 100\n}\n```"
	paths := []string{"/photos/only.jpg"}
	result, err := ParseRankingResult(raw, paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Ranked) != 1 {
		t.Fatalf("ranked count: got %d, want 1", len(result.Ranked))
	}
	if result.Ranked[0].PhotoPath != "/photos/only.jpg" {
		t.Errorf("photo_path: got %q, want %q", result.Ranked[0].PhotoPath, "/photos/only.jpg")
	}
}
