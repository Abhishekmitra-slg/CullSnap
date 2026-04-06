package app

import (
	"cullsnap/internal/scoring"
	"cullsnap/internal/storage"
)

// AIScoringStatus contains the current state of AI scoring.
type AIScoringStatus struct {
	Enabled   bool                   `json:"enabled"`
	Plugins   []scoring.PluginStatus `json:"plugins"`
	Ready     bool                   `json:"ready"`
	HasModels bool                   `json:"hasModels"`
}

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

// AIStepEvent is emitted during AI analysis to report progress on a step.
type AIStepEvent struct {
	Step      string `json:"step"`
	Current   int    `json:"current"`
	Total     int    `json:"total"`
	PhotoPath string `json:"photoPath,omitempty"`
}

// AICompleteEvent is emitted when AI analysis finishes successfully.
type AICompleteEvent struct {
	Scored    int     `json:"scored"`
	Faces     int     `json:"faces"`
	Clusters  int     `json:"clusters"`
	ElapsedMs int64   `json:"elapsedMs"`
	TopPhoto  string  `json:"topPhoto"`
	TopScore  float64 `json:"topScore"`
}

// AIWeightsConfig exposes score weights to the frontend for the Settings UI.
type AIWeightsConfig struct {
	Aesthetic float64 `json:"aesthetic"`
	Sharpness float64 `json:"sharpness"`
	Face      float64 `json:"face"`
	Eyes      float64 `json:"eyes"`
}
