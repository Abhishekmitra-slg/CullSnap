package vlm

import (
	"strings"
	"testing"
)

func TestSystemPrompt(t *testing.T) {
	p := SystemPrompt("")
	if p == "" {
		t.Fatal("SystemPrompt returned empty string")
	}
	if len(p) < 100 {
		t.Fatalf("SystemPrompt too short: got %d chars, want >= 100", len(p))
	}
}

func TestSystemPromptWithCustomInstructions(t *testing.T) {
	custom := "prefer natural light photography"
	p := SystemPrompt(custom)
	if !strings.Contains(p, custom) {
		t.Fatalf("SystemPrompt missing custom instruction %q", custom)
	}
	if !strings.Contains(p, "Additional instructions from user:") {
		t.Fatal("SystemPrompt missing expected prefix for custom instructions")
	}
}

func TestStage4Prompt(t *testing.T) {
	p := Stage4Prompt(Stage4Input{})
	if p == "" {
		t.Fatal("Stage4Prompt returned empty string")
	}
	if !strings.Contains(p, "Evaluate this photograph") {
		t.Fatalf("Stage4Prompt missing expected phrase, got: %s", p)
	}
}

func TestStage4PromptWithContext(t *testing.T) {
	input := Stage4Input{
		Context:        "golden hour portrait",
		FaceCount:      2,
		SharpnessScore: 0.85,
		FocalLength:    85,
	}
	p := Stage4Prompt(input)

	if !strings.Contains(p, "golden hour portrait") {
		t.Errorf("Stage4Prompt missing context string, got: %s", p)
	}
	if !strings.Contains(p, "Detected faces: 2") {
		t.Errorf("Stage4Prompt missing face count, got: %s", p)
	}
	if !strings.Contains(p, "0.85") {
		t.Errorf("Stage4Prompt missing sharpness score, got: %s", p)
	}
	if !strings.Contains(p, "85mm") {
		t.Errorf("Stage4Prompt missing focal length, got: %s", p)
	}
}

func TestStage5Prompt(t *testing.T) {
	input := Stage5Input{
		Photos: []Stage5Photo{
			{Aesthetic: 0.8, Sharpness: 0.9, FaceCount: 1, Issues: nil},
			{Aesthetic: 0.6, Sharpness: 0.7, FaceCount: 0, Issues: []string{"overexposed"}},
		},
		UseCase: "wedding album",
	}
	p := Stage5Prompt(input)

	if !strings.Contains(p, "Compare 2 photographs") {
		t.Errorf("Stage5Prompt missing photo count, got: %s", p)
	}
	if !strings.Contains(p, "wedding album") {
		t.Errorf("Stage5Prompt missing use case, got: %s", p)
	}
}

func TestPromptVersion(t *testing.T) {
	if PromptVersion < 1 {
		t.Fatalf("PromptVersion must be >= 1, got %d", PromptVersion)
	}
}
