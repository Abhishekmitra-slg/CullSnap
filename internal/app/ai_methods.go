package app

import (
	"context"
	"cullsnap/internal/logger"
	"fmt"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// GetAIScoringStatus returns the current AI scoring configuration and provider status.
func (a *App) GetAIScoringStatus() (*AIScoringStatus, error) {
	logger.Log.Debug("app: getting AI scoring status")
	status := &AIScoringStatus{
		Enabled: a.aiEnabled,
		// TODO(ai): populate Providers from scoring.Engine.Providers() once implemented
		Providers: []interface{}{},
	}
	return status, nil
}

// SetAIScoringEnabled enables or disables AI scoring and persists the setting.
func (a *App) SetAIScoringEnabled(enabled bool) error {
	logger.Log.Info("app: setting AI scoring enabled", "enabled", enabled)
	a.aiEnabled = enabled
	return a.store.SetConfig("ai_scoring_enabled", fmt.Sprintf("%t", enabled))
}

// GetAIResults returns all AI scores and face clusters for a folder.
func (a *App) GetAIResults(folderPath string) (*AIResults, error) {
	logger.Log.Debug("app: getting AI results", "folder", folderPath)

	scores, err := a.store.GetAIScoresForFolder(folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI scores: %w", err)
	}

	clusters, err := a.store.GetFaceClusters(folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get face clusters: %w", err)
	}

	logger.Log.Debug("app: AI results loaded",
		"folder", folderPath,
		"scoreCount", len(scores),
		"clusterCount", len(clusters),
	)

	return &AIResults{Scores: scores, Clusters: clusters}, nil
}

// GetPhotoAIScore returns the AI score and face detections for a single photo.
func (a *App) GetPhotoAIScore(photoPath string) (*PhotoAIScore, error) {
	logger.Log.Debug("app: getting photo AI score", "path", photoPath)

	score, err := a.store.GetAIScore(photoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI score: %w", err)
	}

	detections, err := a.store.GetFaceDetections(photoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get face detections: %w", err)
	}

	return &PhotoAIScore{Score: score, Detections: detections}, nil
}

// RenameFaceCluster updates the label for a face cluster.
func (a *App) RenameFaceCluster(clusterID int, label string) error {
	logger.Log.Info("app: renaming face cluster", "clusterID", clusterID, "label", label)
	return a.store.RenameFaceCluster(int64(clusterID), label)
}

// MergeFaceClusters merges sourceID into targetID.
func (a *App) MergeFaceClusters(sourceID, targetID int) error {
	logger.Log.Info("app: merging face clusters", "source", sourceID, "target", targetID)
	return a.store.MergeFaceClusters(int64(sourceID), int64(targetID))
}

// HideFaceCluster sets the hidden flag on a cluster.
func (a *App) HideFaceCluster(clusterID int, hidden bool) error {
	logger.Log.Info("app: hiding face cluster", "clusterID", clusterID, "hidden", hidden)
	return a.store.HideFaceCluster(int64(clusterID), hidden)
}

// RunAIAnalysis starts AI scoring and clustering for all photos in a folder.
// Emits ai:progress, ai:photo-scored, and ai:clustering-complete events.
// Full implementation will be wired to scoring.Engine once backend plan Tasks 3-5 are complete.
func (a *App) RunAIAnalysis(folderPath string) error {
	a.analysisMu.Lock()
	if a.analysisCancel != nil {
		a.analysisCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.analysisCancel = cancel
	a.analysisMu.Unlock()

	logger.Log.Info("app: starting AI analysis", "folder", folderPath)

	go func() {
		defer cancel()
		// TODO(ai): Wire to scoring.Engine pipeline once backend plan Tasks 3-5 are available.
		// Pipeline: list photos → score each (emit ai:photo-scored) → cluster → emit ai:clustering-complete.

		wailsRuntime.EventsEmit(ctx, "ai:clustering-complete", map[string]interface{}{
			"clusters": []interface{}{},
		})
		logger.Log.Debug("app: AI analysis stub complete", "folder", folderPath)
	}()

	return nil
}

// CancelAIAnalysis cancels any in-progress AI analysis.
func (a *App) CancelAIAnalysis() error {
	a.analysisMu.Lock()
	defer a.analysisMu.Unlock()
	if a.analysisCancel != nil {
		logger.Log.Info("app: cancelling AI analysis")
		a.analysisCancel()
		a.analysisCancel = nil
	}
	return nil
}

// DownloadAIModels triggers download of ONNX models for the local AI provider.
// Emits ai:model-download-progress events.
// Full implementation delegates to scoring.ModelManager (backend plan Task 3).
func (a *App) DownloadAIModels() error {
	logger.Log.Info("app: downloading AI models")
	// TODO(ai): Delegate to scoring.ModelManager once backend plan Task 3 is implemented.
	return nil
}

// SetCloudAPIKey stores a cloud AI provider API key in the OS keychain.
func (a *App) SetCloudAPIKey(provider, apiKey string) error {
	logger.Log.Info("app: setting cloud API key", "provider", provider)
	// TODO(ai): Use go-keyring (same pattern as OAuth token store) once cloud AI provider is implemented.
	return nil
}

// TestCloudConnection validates a cloud AI provider API key.
func (a *App) TestCloudConnection(provider string) error {
	logger.Log.Info("app: testing cloud connection", "provider", provider)
	// TODO(ai): Implement once cloud AI provider is available.
	return nil
}
