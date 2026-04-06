package scoring

import (
	"context"
	"image"
	"time"
)

// PluginCategory classifies what a scoring plugin analyses.
type PluginCategory int

const (
	// CategoryDetection plugins locate faces or other regions in an image.
	CategoryDetection PluginCategory = iota
	// CategoryRecognition plugins identify or embed detected regions.
	CategoryRecognition
	// CategoryQuality plugins measure perceptual or technical image quality.
	CategoryQuality
)

// String returns a human-readable name for the category.
func (c PluginCategory) String() string {
	switch c {
	case CategoryDetection:
		return "detection"
	case CategoryRecognition:
		return "recognition"
	case CategoryQuality:
		return "quality"
	default:
		return "unknown"
	}
}

// ModelSpec describes a single model file that a plugin depends on.
type ModelSpec struct {
	Name     string
	Filename string
	URL      string
	SHA256   string
	Size     int64
}

// ScoringPlugin is the interface every scoring plugin must implement.
type ScoringPlugin interface {
	// Name returns the display name of this plugin (e.g., "BlazeFace", "MobileNet").
	Name() string

	// Category returns the type of analysis this plugin performs.
	Category() PluginCategory

	// Models returns the list of model files required by this plugin.
	Models() []ModelSpec

	// Available reports whether all required models are present and ready.
	Available() bool

	// Init loads models from libPath and prepares the plugin for use.
	Init(libPath string) error

	// Close releases any resources held by the plugin.
	Close() error

	// Process analyses img and returns a PluginResult.
	Process(ctx context.Context, img image.Image) (PluginResult, error)
}

// RecognitionPlugin extends ScoringPlugin with region-level processing,
// used by face recognition and embedding plugins that operate on pre-detected
// face crops rather than the full image.
type RecognitionPlugin interface {
	ScoringPlugin

	// ProcessRegions analyses the sub-regions described by faces within img.
	ProcessRegions(ctx context.Context, img image.Image, faces []FaceRegion) (PluginResult, error)
}

// FaceRegion describes a single detected face.
type FaceRegion struct {
	// BoundingBox is the face location within the image.
	BoundingBox image.Rectangle

	// Landmarks holds up to 5 facial landmark points (x, y) normalised to [0,1].
	Landmarks [5][2]float32

	// Confidence is the detection confidence for this face (0.0–1.0).
	Confidence float64
}

// FaceEmbedding pairs a face crop with its embedding vector for clustering.
type FaceEmbedding struct {
	// PhotoPath is the absolute path of the source image.
	PhotoPath string

	// DetectionID is an opaque identifier that ties the embedding back to a
	// specific FaceRegion from the same detection pass.
	DetectionID int64

	// Embedding is the raw float32 feature vector produced by the recognition model.
	Embedding []float32
}

// QualityScore represents a single named quality metric.
type QualityScore struct {
	Score float64
	Name  string
}

// PluginResult is the output of a single plugin's Process or ProcessRegions call.
// Fields that are irrelevant to the plugin's category will be zero/nil.
type PluginResult struct {
	Faces      []FaceRegion
	Embeddings []FaceEmbedding
	Quality    *QualityScore
}

// CompositeScore aggregates the results from all plugins for a single image.
type CompositeScore struct {
	// AestheticScore is the overall visual quality score (0.0–1.0).
	AestheticScore float64

	// SharpnessScore is the global image sharpness score (0.0–1.0).
	SharpnessScore float64

	// FaceCount is the number of detected faces.
	FaceCount int

	// Faces is the list of detected face regions.
	Faces []FaceRegion

	// Embeddings contains embedding vectors for each detected face,
	// grouped per face (outer slice index matches Faces index).
	Embeddings [][]float32

	// BestFaceSharp is the sharpness score of the best face crop (0.0–1.0).
	BestFaceSharp float64

	// EyeOpenness is the openness score for the best face's eyes (0.0–1.0).
	EyeOpenness float64

	// BestFaceIdx is the index into Faces of the highest-quality face.
	BestFaceIdx int

	// Provider is the name of the scoring provider that produced this result.
	Provider string

	// ScoredAt is when the scoring was performed.
	ScoredAt time.Time
}

// OverallScore returns a single weighted score for the image using w.
func (c CompositeScore) OverallScore(w ScoreWeights) float64 {
	return w.Apply(c)
}
