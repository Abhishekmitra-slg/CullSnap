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
