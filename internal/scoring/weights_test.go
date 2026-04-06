package scoring

import (
	"math"
	"testing"
)

const floatTol = 1e-9

func approxEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// TestDefaultWeights verifies that the default weights sum to exactly 1.0.
func TestDefaultWeights(t *testing.T) {
	w := DefaultWeights()
	sum := w.Aesthetic + w.Sharpness + w.Face + w.Eyes
	if !approxEqual(sum, 1.0, floatTol) {
		t.Errorf("DefaultWeights sum = %v, want 1.0", sum)
	}
	if w.Aesthetic != 0.35 {
		t.Errorf("Aesthetic = %v, want 0.35", w.Aesthetic)
	}
	if w.Sharpness != 0.25 {
		t.Errorf("Sharpness = %v, want 0.25", w.Sharpness)
	}
	if w.Face != 0.25 {
		t.Errorf("Face = %v, want 0.25", w.Face)
	}
	if w.Eyes != 0.15 {
		t.Errorf("Eyes = %v, want 0.15", w.Eyes)
	}
}

// TestScoreWeights_Normalize_ArbitraryValues checks that Normalize scales
// arbitrary positive values to sum to 1.0 while preserving ratios.
func TestScoreWeights_Normalize_ArbitraryValues(t *testing.T) {
	w := ScoreWeights{Aesthetic: 2, Sharpness: 3, Face: 1, Eyes: 4}
	n := w.Normalize()
	sum := n.Aesthetic + n.Sharpness + n.Face + n.Eyes
	if !approxEqual(sum, 1.0, floatTol) {
		t.Errorf("Normalize sum = %v, want 1.0", sum)
	}
	// Ratios must be preserved: Aesthetic/Sharpness should equal 2/3.
	ratio := n.Aesthetic / n.Sharpness
	if !approxEqual(ratio, 2.0/3.0, floatTol) {
		t.Errorf("Normalize ratio Aesthetic/Sharpness = %v, want %v", ratio, 2.0/3.0)
	}
}

// TestScoreWeights_Normalize_AllZero checks the degenerate case where all
// weights are zero — result must be equal weights.
func TestScoreWeights_Normalize_AllZero(t *testing.T) {
	w := ScoreWeights{}
	n := w.Normalize()
	want := 0.25
	for _, v := range []float64{n.Aesthetic, n.Sharpness, n.Face, n.Eyes} {
		if !approxEqual(v, want, floatTol) {
			t.Errorf("Normalize all-zero field = %v, want %v", v, want)
		}
	}
}

// TestScoreWeights_Apply_WithFaces checks the weighted blend when faces are
// present.  With default weights and scores 0.8/0.9/0.7/0.6 the result is
// 0.35*0.8 + 0.25*0.9 + 0.25*0.7 + 0.15*0.6 = 0.77.
func TestScoreWeights_Apply_WithFaces(t *testing.T) {
	w := DefaultWeights()
	cs := CompositeScore{
		AestheticScore: 0.8,
		SharpnessScore: 0.9,
		BestFaceSharp:  0.7,
		EyeOpenness:    0.6,
		FaceCount:      1,
	}
	got := w.Apply(cs)
	want := 0.77
	if !approxEqual(got, want, 1e-9) {
		t.Errorf("Apply with faces = %v, want ~%v", got, want)
	}
}

// TestScoreWeights_Apply_NoFaces checks redistribution of face/eye weights
// when FaceCount == 0.  Default weights, aesthetic=0.8, sharpness=0.6:
//
//	aestheticEff = 0.35 + 0.40*(0.35/0.60) ≈ 0.5833
//	sharpnessEff = 0.25 + 0.40*(0.25/0.60) ≈ 0.4167
//	score        = 0.5833*0.8 + 0.4167*0.6 ≈ 0.7167
func TestScoreWeights_Apply_NoFaces(t *testing.T) {
	w := DefaultWeights()
	cs := CompositeScore{
		AestheticScore: 0.8,
		SharpnessScore: 0.6,
		FaceCount:      0,
	}
	got := w.Apply(cs)
	want := 0.7167
	if math.Abs(got-want) > 1e-3 {
		t.Errorf("Apply no faces = %v, want ~%v", got, want)
	}
}

// TestScoreWeights_Apply_ClampAbove1 verifies that a result that would
// exceed 1.0 (e.g. all input scores > 1) is clamped to 1.0.
func TestScoreWeights_Apply_ClampAbove1(t *testing.T) {
	w := DefaultWeights()
	cs := CompositeScore{
		AestheticScore: 2.0,
		SharpnessScore: 2.0,
		BestFaceSharp:  2.0,
		EyeOpenness:    2.0,
		FaceCount:      1,
	}
	got := w.Apply(cs)
	if got != 1.0 {
		t.Errorf("Apply clamp above 1 = %v, want 1.0", got)
	}
}

// TestClamp01 checks boundary and interior values.
func TestClamp01(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{-1.0, 0.0},
		{0.0, 0.0},
		{0.5, 0.5},
		{1.0, 1.0},
		{1.5, 1.0},
	}
	for _, tc := range cases {
		got := clamp01(tc.in)
		if got != tc.want {
			t.Errorf("clamp01(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
