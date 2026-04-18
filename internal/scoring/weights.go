package scoring

// ScoreWeights holds the per-metric weighting factors used to blend
// individual quality scores into a single overall score for an image.
//
// The five fields correspond to:
//   - Aesthetic:   overall visual / compositional quality
//   - Sharpness:   global image sharpness
//   - Face:        sharpness of the best detected face crop
//   - Eyes:        openness of the best face's eyes
//   - Composition: VLM-derived composition quality score
//
// Weights are stored in [0, 1] and are expected to sum to 1.0.  Use
// Normalize to enforce that invariant after user edits.
type ScoreWeights struct {
	Aesthetic   float64 `json:"aesthetic"`   // default 0.25
	Sharpness   float64 `json:"sharpness"`   // default 0.20
	Face        float64 `json:"face"`        // default 0.20
	Eyes        float64 `json:"eyes"`        // default 0.10
	Composition float64 `json:"composition"` // default 0.25 (VLM)
}

// DefaultWeights returns the recommended starting weights.
// They sum to exactly 1.0: 0.25+0.20+0.20+0.10+0.25 = 1.00.
func DefaultWeights() ScoreWeights {
	return ScoreWeights{
		Aesthetic:   0.25,
		Sharpness:   0.20,
		Face:        0.20,
		Eyes:        0.10,
		Composition: 0.25,
	}
}

// Normalize scales all weights so that they sum to 1.0.
// If all weights are zero, it returns equal weights {0.2, 0.2, 0.2, 0.2, 0.2}.
func (w ScoreWeights) Normalize() ScoreWeights {
	sum := w.Aesthetic + w.Sharpness + w.Face + w.Eyes + w.Composition
	if sum == 0 {
		return ScoreWeights{0.2, 0.2, 0.2, 0.2, 0.2}
	}
	return ScoreWeights{
		Aesthetic:   w.Aesthetic / sum,
		Sharpness:   w.Sharpness / sum,
		Face:        w.Face / sum,
		Eyes:        w.Eyes / sum,
		Composition: w.Composition / sum,
	}
}

// Apply blends the individual scores in cs into a single overall score.
//
// When faces are present (cs.FaceCount > 0) all five weights are used directly.
// When no faces are detected the Face and Eyes weights are redistributed to
// Aesthetic, Sharpness, and Composition proportionally, so the result is still
// meaningful.
//
// The returned value is clamped to [0, 1].
func (w ScoreWeights) Apply(cs CompositeScore) float64 {
	if cs.FaceCount > 0 {
		// All five weights apply.
		score := w.Aesthetic*cs.AestheticScore +
			w.Sharpness*cs.SharpnessScore +
			w.Face*cs.BestFaceSharp +
			w.Eyes*cs.EyeOpenness +
			w.Composition*cs.VLMComposition
		return clamp01(score)
	}

	// No faces: redistribute Face+Eyes to Aesthetic, Sharpness, and Composition
	// proportionally based on their relative weights.
	noFacePool := w.Face + w.Eyes
	basePool := w.Aesthetic + w.Sharpness + w.Composition

	var aestheticEff, sharpnessEff, compositionEff float64
	if basePool == 0 {
		// Edge case: Aesthetic, Sharpness, and Composition are all zero.
		// Split the pool evenly.
		aestheticEff = noFacePool / 3
		sharpnessEff = noFacePool / 3
		compositionEff = noFacePool / 3
	} else {
		aestheticEff = w.Aesthetic + noFacePool*(w.Aesthetic/basePool)
		sharpnessEff = w.Sharpness + noFacePool*(w.Sharpness/basePool)
		compositionEff = w.Composition + noFacePool*(w.Composition/basePool)
	}

	score := aestheticEff*cs.AestheticScore +
		sharpnessEff*cs.SharpnessScore +
		compositionEff*cs.VLMComposition
	return clamp01(score)
}

// clamp01 returns v clamped to the closed interval [0, 1].
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
