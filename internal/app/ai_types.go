package app

import (
	"cullsnap/internal/scoring"
	"cullsnap/internal/storage"
	"cullsnap/internal/vlm"
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
	Aesthetic   float64 `json:"aesthetic"`
	Sharpness   float64 `json:"sharpness"`
	Face        float64 `json:"face"`
	Eyes        float64 `json:"eyes"`
	Composition float64 `json:"composition"`
}

// VLMPhotoResult is emitted per-photo during VLM Stage 4.
type VLMPhotoResult struct {
	PhotoPath   string  `json:"photoPath"`
	Aesthetic   float64 `json:"aesthetic"`
	Composition float64 `json:"composition"`
	SceneType   string  `json:"sceneType"`
	Explanation string  `json:"explanation"`
}

// VLMStatus reports the VLM engine state to the frontend.
type VLMStatus struct {
	State        string `json:"state"`
	ModelName    string `json:"modelName"`
	Backend      string `json:"backend"`
	Uptime       string `json:"uptime"`
	Available    bool   `json:"available"`
	HardwareTier string `json:"hardwareTier"`
}

// vlmStatusFromManager converts a vlm.ManagerStatus + hardware probe into a VLMStatus.
func vlmStatusFromManager(s vlm.ManagerStatus, tier vlm.HardwareTier) VLMStatus {
	return VLMStatus{
		State:        s.State,
		ModelName:    s.ModelName,
		Backend:      s.Backend,
		Uptime:       s.Uptime,
		Available:    tier != vlm.TierLegacy,
		HardwareTier: tier.String(),
	}
}

// VLMDetailedStatus extends VLMStatus with runtime stats for the engine status panel.
type VLMDetailedStatus struct {
	VLMStatus
	RestartCount int   `json:"restartCount"`
	InferCount   int   `json:"inferCount"`
	RAMUsageMB   int64 `json:"ramUsageMB"`
}

// AIStorageInfo reports disk usage for AI-related data.
type AIStorageInfo struct {
	ModelSizeMB    int64  `json:"modelSizeMB"`
	RuntimeSizeMB  int64  `json:"runtimeSizeMB"`
	ScoresDBSizeMB int64  `json:"scoresDBSizeMB"`
	TotalMB        int64  `json:"totalMB"`
	ModelName      string `json:"modelName"`
	RuntimeName    string `json:"runtimeName"`
}

// VLMStaleStatus reports whether VLM scores for a folder are outdated.
type VLMStaleStatus struct {
	Stale         bool     `json:"stale"`
	StaleFolders  []string `json:"staleFolders"`
	CurrentPrompt int      `json:"currentPrompt"`
}
