package app

import (
	"cullsnap/internal/scoring"
	"cullsnap/internal/storage"
)

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
type AIScoringStatus struct {
	Enabled   bool                     `json:"enabled"`
	Providers []scoring.ProviderStatus `json:"providers"`
}
