package vlm

import (
	"fmt"
	"strings"
)

// PromptVersion is incremented whenever prompts change materially.
// Stored alongside cached VLM results so stale scores can be invalidated.
const PromptVersion = 1

// systemPromptBase is the base system prompt for CullSnap's photo analysis engine.
const systemPromptBase = `You are CullSnap's photo analysis engine. Your job is to evaluate photographs for quality and help photographers identify their best shots.

Rules:
- Score all numeric fields from 0.0 to 1.0. Use the full range: 0.0 is unusable, 0.5 is average, 1.0 is exceptional.
- Be concise and specific. Avoid generic praise.
- Report at most 3 issues per photo. Only include real, observable problems.
- Always respond with valid JSON matching the schema provided. Do not include markdown code fences or extra text.
- Differentiate quality decisively — do not cluster all scores near 0.7.`

// SystemPrompt returns the system prompt for the VLM.
// If customInstructions is non-empty, they are appended with a labelled prefix.
func SystemPrompt(customInstructions string) string {
	if customInstructions == "" {
		return systemPromptBase
	}
	return systemPromptBase + "\n\nAdditional instructions from user:\n" + customInstructions
}

// Stage4Input holds the per-photo context injected into Stage 4 scoring prompts.
type Stage4Input struct {
	Context        string  // Optional shoot context, e.g. "portrait session"
	FaceCount      int     // Number of faces detected by ONNX Stage 1
	SharpnessScore float64 // Laplacian sharpness score from ONNX Stage 1 (0–1)
	FocalLength    int     // Focal length in mm from EXIF, 0 if unknown
}

// Stage4Prompt renders the user-turn prompt for individual photo scoring (Stage 4).
func Stage4Prompt(input Stage4Input) string {
	var sb strings.Builder

	sb.WriteString("Evaluate this photograph.\n")

	if input.Context != "" {
		sb.WriteString(fmt.Sprintf("Context: %s\n", input.Context))
	}

	if input.FaceCount > 0 {
		sb.WriteString(fmt.Sprintf("Detected faces: %d\n", input.FaceCount))
	}

	sb.WriteString(fmt.Sprintf("Sharpness score: %.2f\n", input.SharpnessScore))

	if input.FocalLength > 0 {
		sb.WriteString(fmt.Sprintf("Focal length: %dmm\n", input.FocalLength))
	}

	sb.WriteString(`
Respond with JSON matching this schema exactly:
{
  "aesthetic": <float 0-1>,
  "composition": <float 0-1>,
  "expression": <float 0-1>,
  "technical_quality": <float 0-1>,
  "scene_type": <string>,
  "issues": [<string>, ...],
  "explanation": <string>
}`)

	return sb.String()
}

// Stage5Photo carries per-photo context for a Stage 5 ranking comparison.
type Stage5Photo struct {
	Aesthetic float64
	Sharpness float64
	FaceCount int
	Issues    []string
}

// Stage5Input describes a batch of photos to rank in Stage 5.
type Stage5Input struct {
	Photos  []Stage5Photo
	UseCase string // e.g. "linkedin profile", "wedding album", "portfolio"
}

// Stage5Prompt renders the user-turn prompt for pairwise photo ranking (Stage 5).
func Stage5Prompt(input Stage5Input) string {
	var sb strings.Builder

	n := len(input.Photos)
	sb.WriteString(fmt.Sprintf("Compare %d photographs and rank them from best to worst.\n", n))

	if input.UseCase != "" {
		sb.WriteString(fmt.Sprintf("Use case: %s\n", input.UseCase))
	}

	sb.WriteString("\nPer-photo scores from automated analysis:\n")
	sb.WriteString("Photo | Aesthetic | Sharpness | Faces | Issues\n")
	sb.WriteString("------|-----------|-----------|-------|-------\n")

	for i, p := range input.Photos {
		issueStr := "none"
		if len(p.Issues) > 0 {
			issueStr = strings.Join(p.Issues, ", ")
		}
		sb.WriteString(fmt.Sprintf("%d     | %.2f      | %.2f      | %d     | %s\n",
			i+1, p.Aesthetic, p.Sharpness, p.FaceCount, issueStr))
	}

	sb.WriteString(`
Respond with JSON matching this schema exactly:
{
  "ranked": [
    {"rank": <int>, "photo_index": <int 1-based>, "score": <float 0-1>, "notes": <string>},
    ...
  ],
  "explanation": <string>
}`)

	return sb.String()
}
