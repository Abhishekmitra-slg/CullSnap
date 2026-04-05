package scoring

import (
	"context"
	"image"
)

// ScoringProvider is the interface for AI-based image quality scoring.
// Implementations may use local models (ONNX), cloud APIs, or custom logic.
type ScoringProvider interface {
	// Name returns the display name of this provider (e.g., "Local ONNX", "OpenAI Vision").
	Name() string

	// Available reports whether this provider is ready to score images.
	// For local providers, this means models are downloaded.
	// For cloud providers, this means an API key is configured.
	Available() bool

	// RequiresAPIKey reports whether this provider needs a user-provided API key.
	RequiresAPIKey() bool

	// RequiresDownload reports whether this provider needs to download models before use.
	RequiresDownload() bool

	// Score analyzes an image and returns face detection results and quality scores.
	// imgData is JPEG-encoded image bytes (typically from a 300px thumbnail).
	// Returns nil result with nil error if no analysis is possible.
	Score(ctx context.Context, imgData []byte) (*ScoreResult, error)
}

// ScoreResult contains the AI analysis of a single image.
type ScoreResult struct {
	// Faces contains detected face regions with per-face metrics.
	Faces []FaceRegion

	// OverallScore is the composite quality score (0.0 = worst, 1.0 = best).
	OverallScore float64

	// Confidence is the model's confidence in the overall assessment.
	Confidence float64
}

// HasFaces reports whether any faces were detected.
func (r *ScoreResult) HasFaces() bool {
	return len(r.Faces) > 0
}

// BestFace returns the face with the highest eye sharpness, or nil if no faces.
func (r *ScoreResult) BestFace() *FaceRegion {
	if len(r.Faces) == 0 {
		return nil
	}
	best := &r.Faces[0]
	for i := 1; i < len(r.Faces); i++ {
		if r.Faces[i].EyeSharpness > best.EyeSharpness {
			best = &r.Faces[i]
		}
	}
	return best
}

// FaceRegion describes a single detected face and its quality metrics.
type FaceRegion struct {
	// BoundingBox is the face location in the image.
	BoundingBox image.Rectangle

	// EyeSharpness is the Laplacian variance of the eye regions (higher = sharper).
	EyeSharpness float64

	// EyesOpen indicates whether both eyes appear open.
	EyesOpen bool

	// Expression is an expression quality score (0.0 = neutral, 1.0 = positive/smiling).
	Expression float64

	// Confidence is the detection confidence for this face (0.0 - 1.0).
	Confidence float64
}
