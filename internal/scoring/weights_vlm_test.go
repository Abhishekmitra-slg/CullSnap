package scoring

import "testing"

func TestDefaultWeightsWithComposition(t *testing.T) {
	w := DefaultWeights()
	sum := w.Aesthetic + w.Sharpness + w.Face + w.Eyes + w.Composition
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("default weights sum to %v, want ~1.0", sum)
	}
}

func TestNormalizeWithComposition(t *testing.T) {
	w := ScoreWeights{Aesthetic: 2, Sharpness: 1, Face: 1, Eyes: 1, Composition: 1}
	n := w.Normalize()
	sum := n.Aesthetic + n.Sharpness + n.Face + n.Eyes + n.Composition
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("normalized sum = %v, want ~1.0", sum)
	}
}

func TestApplyWithVLMComposition(t *testing.T) {
	w := ScoreWeights{Aesthetic: 0.25, Sharpness: 0.20, Face: 0.20, Eyes: 0.10, Composition: 0.25}
	cs := CompositeScore{AestheticScore: 0.8, SharpnessScore: 0.7, BestFaceSharp: 0.9, EyeOpenness: 0.6, FaceCount: 1, VLMComposition: 0.85}
	score := w.Apply(cs)
	// Expected: 0.7925 from weighted sum of aesthetic/sharpness/face/eyes/composition.
	if score < 0.785 || score > 0.800 {
		t.Errorf("score = %v, want ~0.7925", score)
	}
}

func TestApplyNoFacesRedistributesComposition(t *testing.T) {
	w := ScoreWeights{Aesthetic: 0.25, Sharpness: 0.20, Face: 0.20, Eyes: 0.10, Composition: 0.25}
	cs := CompositeScore{AestheticScore: 0.8, SharpnessScore: 0.7, FaceCount: 0, VLMComposition: 0.85}
	score := w.Apply(cs)
	if score < 0 || score > 1 {
		t.Errorf("score %v out of range", score)
	}
	if score == 0 {
		t.Error("score should not be zero")
	}
}
