package app

import "cullsnap/internal/storage"

// AIResults contains the complete AI analysis results for a folder.
type AIResults struct {
	Scores   []storage.AIScore     `json:"scores"`
	Clusters []storage.FaceCluster `json:"clusters"`
}

// PhotoAIScore contains the AI score and face details for a single photo.
type PhotoAIScore struct {
	Score      *storage.AIScore        `json:"score"`
	Detections []storage.FaceDetection `json:"detections"`
}

// AIScoringStatus contains the current state of AI scoring.
// Providers is left as a generic slice until scoring.ProviderStatus is defined
// in internal/scoring/engine.go (backend plan Tasks 3-5).
type AIScoringStatus struct {
	Enabled   bool          `json:"enabled"`
	Providers []interface{} `json:"providers"`
}
