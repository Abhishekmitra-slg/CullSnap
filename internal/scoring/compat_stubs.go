package scoring

// compat_stubs.go contains legacy type stubs that keep the old engine,
// cloud_provider and local_provider files compiling while the new
// plugin-based architecture is being migrated in.  These types will be
// removed in Task 12 (dead-code cleanup) once all callers have been
// rewritten.

import "context"

// ScoringProvider is the legacy scoring interface used by Engine,
// CloudProvider and LocalProvider.
type ScoringProvider interface {
	Name() string
	Available() bool
	RequiresAPIKey() bool
	RequiresDownload() bool
	Score(ctx context.Context, imgData []byte) (*ScoreResult, error)
}

// ScoreResult is the legacy result type returned by ScoringProvider.Score.
type ScoreResult struct {
	Faces        []FaceRegion
	OverallScore float64
	Confidence   float64
}

// ScoreWeights holds the weighting factors used to combine individual
// quality metrics into a single overall score.
type ScoreWeights struct {
	Aesthetic float64
	Sharpness float64
	FaceBonus float64
	FaceSharp float64
	EyeOpen   float64
}

// Apply returns a weighted composite score in [0, 1].
func (w ScoreWeights) Apply(c CompositeScore) float64 {
	total := w.Aesthetic + w.Sharpness
	if total == 0 {
		return 0
	}
	score := w.Aesthetic*c.AestheticScore + w.Sharpness*c.SharpnessScore
	if c.FaceCount > 0 {
		faceTotal := w.FaceBonus + w.FaceSharp + w.EyeOpen
		if faceTotal > 0 {
			faceScore := (w.FaceSharp*c.BestFaceSharp + w.EyeOpen*c.EyeOpenness) / faceTotal
			score = score*(1-w.FaceBonus) + faceScore*w.FaceBonus
			return score
		}
	}
	return score / total
}

// DefaultWeights returns a reasonable default ScoreWeights.
func DefaultWeights() ScoreWeights {
	return ScoreWeights{
		Aesthetic: 0.4,
		Sharpness: 0.4,
		FaceBonus: 0.2,
		FaceSharp: 0.5,
		EyeOpen:   0.5,
	}
}
