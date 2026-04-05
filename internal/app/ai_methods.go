package app

import (
	"bytes"
	"context"
	"cullsnap/internal/logger"
	"cullsnap/internal/scanner"
	"cullsnap/internal/scoring"
	"cullsnap/internal/storage"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/zalando/go-keyring"
)

// GetAIScoringStatus returns the current AI scoring configuration and provider status.
func (a *App) GetAIScoringStatus() (*AIScoringStatus, error) {
	logger.Log.Debug("app: getting AI scoring status")
	status := &AIScoringStatus{
		Enabled:   a.aiEnabled,
		Providers: a.scoringEngine.Providers(),
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
// Emits ai:progress, ai:photo-scored, ai:error, and ai:clustering-complete events.
func (a *App) RunAIAnalysis(folderPath string) error {
	if !a.aiEnabled {
		return fmt.Errorf("AI scoring is disabled — enable it in Settings")
	}
	if !a.scoringEngine.Enabled() {
		return fmt.Errorf("no AI scoring provider available — download models in Settings or configure a cloud API key")
	}

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
		a.runAIAnalysisPipeline(ctx, folderPath)
	}()

	return nil
}

// runAIAnalysisPipeline scans photos, scores each one, then clusters faces.
func (a *App) runAIAnalysisPipeline(ctx context.Context, folderPath string) {
	// 1. Scan directory for photos.
	photos, err := scanner.ScanDirectory(folderPath)
	if err != nil {
		logger.Log.Error("app: AI analysis scan failed", "error", err)
		wailsRuntime.EventsEmit(a.ctx, "ai:error", map[string]interface{}{
			"error": fmt.Sprintf("Scan failed: %v", err),
		})
		return
	}

	total := len(photos)
	if total == 0 {
		logger.Log.Info("app: no photos found for AI analysis", "folder", folderPath)
		wailsRuntime.EventsEmit(a.ctx, "ai:error", map[string]interface{}{
			"error": "No photos found in this folder",
		})
		return
	}

	logger.Log.Info("app: AI analysis scanning complete", "photos", total)

	wailsRuntime.EventsEmit(a.ctx, "ai:progress", map[string]interface{}{
		"phase":  "scoring",
		"scored": 0,
		"total":  total,
	})

	// 2. Score each photo.
	scored := 0
	errors := 0

	for i := range photos {
		photo := &photos[i]
		if ctx.Err() != nil {
			logger.Log.Info("app: AI analysis cancelled")
			wailsRuntime.EventsEmit(a.ctx, "ai:error", map[string]interface{}{
				"error": "Analysis cancelled",
			})
			return
		}

		result, scoreErr := a.scorePhoto(ctx, photo.Path)
		if scoreErr != nil {
			logger.Log.Warn("app: failed to score photo", "path", photo.Path, "error", scoreErr)
			errors++
			// Emit progress even on failure so the bar advances.
			wailsRuntime.EventsEmit(a.ctx, "ai:progress", map[string]interface{}{
				"phase":  "scoring",
				"scored": i + 1,
				"total":  total,
			})
			continue
		}

		// Save score to DB for ALL analyzed photos (even those with 0 faces).
		// This ensures the UI shows "Analyzed — 0 faces" instead of "Not analyzed".
		if dbErr := a.store.SaveAIScore(photo.Path, result.OverallScore, len(result.Faces), "Local (ONNX)"); dbErr != nil {
			logger.Log.Warn("app: failed to save AI score", "path", photo.Path, "error", dbErr)
		}

		// Save face detections if any were found.
		for _, face := range result.Faces {
			det := &storage.FaceDetection{
				PhotoPath:    photo.Path,
				BboxX:        face.BoundingBox.Min.X,
				BboxY:        face.BoundingBox.Min.Y,
				BboxW:        face.BoundingBox.Dx(),
				BboxH:        face.BoundingBox.Dy(),
				EyeSharpness: face.EyeSharpness,
				EyesOpen:     face.EyesOpen,
				Expression:   face.Expression,
				Confidence:   face.Confidence,
			}
			if _, detErr := a.store.SaveFaceDetection(det); detErr != nil {
				logger.Log.Warn("app: failed to save face detection", "error", detErr)
			}
		}

		scored++
		wailsRuntime.EventsEmit(a.ctx, "ai:photo-scored", map[string]interface{}{
			"path":      photo.Path,
			"score":     result.OverallScore,
			"faceCount": len(result.Faces),
		})

		wailsRuntime.EventsEmit(a.ctx, "ai:progress", map[string]interface{}{
			"phase":  "scoring",
			"scored": i + 1,
			"total":  total,
		})
	}

	logger.Log.Info("app: AI scoring phase complete", "scored", scored, "errors", errors, "total", total)

	wailsRuntime.EventsEmit(a.ctx, "ai:clustering-complete", map[string]interface{}{
		"scored":   scored,
		"total":    total,
		"errors":   errors,
		"clusters": []interface{}{},
	})
	logger.Log.Info("app: AI analysis complete", "folder", folderPath, "scored", scored)
}

// scorePhoto scores a single photo by reading its thumbnail and running inference.
func (a *App) scorePhoto(ctx context.Context, photoPath string) (*scoring.ScoreResult, error) {
	// Try cached thumbnail first (300px JPEG — fast, small, no RAW decode needed).
	imgData, err := a.readPhotoForScoring(photoPath)
	if err != nil {
		return nil, fmt.Errorf("read photo: %w", err)
	}

	result, err := a.scoringEngine.Score(ctx, imgData)
	if err != nil {
		return nil, fmt.Errorf("score: %w", err)
	}

	if result == nil {
		// No provider could score — return nil to differentiate from "scored but no faces".
		return nil, fmt.Errorf("no scoring provider available")
	}

	// Post-process: compute eye sharpness for each detected face.
	if result.HasFaces() {
		a.computeEyeSharpness(imgData, result)
	}

	return result, nil
}

// readPhotoForScoring reads the photo data, preferring cached thumbnails.
func (a *App) readPhotoForScoring(photoPath string) ([]byte, error) {
	// Try the thumbnail cache first — it's a 300px JPEG that's fast to decode
	// and already handles RAW/HEIC conversion.
	if a.thumbCache != nil {
		info, err := os.Stat(photoPath)
		if err == nil {
			thumbPath := a.thumbCache.GetCachedPath(photoPath, info.ModTime())
			if thumbPath != "" {
				data, readErr := os.ReadFile(thumbPath) //nolint:gosec // trusted internal path
				if readErr == nil {
					logger.Log.Debug("app: scoring from cached thumbnail", "path", photoPath)
					return data, nil
				}
			}
		}
	}

	// Fallback: read the original file.
	// This works for JPEG/PNG but will fail for RAW/HEIC — that's OK,
	// those photos will be scored after thumbnail generation.
	data, err := os.ReadFile(photoPath) //nolint:gosec // trusted internal path
	if err != nil {
		return nil, err
	}

	logger.Log.Debug("app: scoring from original file", "path", photoPath, "size", len(data))
	return data, nil
}

// computeEyeSharpness adds Laplacian variance eye sharpness scores to detected faces.
func (a *App) computeEyeSharpness(imgData []byte, result *scoring.ScoreResult) {
	srcImg, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		logger.Log.Debug("app: failed to decode image for sharpness", "error", err)
		return
	}

	// Convert to grayscale for Laplacian variance.
	bounds := srcImg.Bounds()
	gray := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := srcImg.At(x, y).RGBA()
			// Standard luminance formula.
			lum := uint8((299*r + 587*g + 114*b + 500) / 1000 >> 8)
			gray.SetGray(x, y, color.Gray{Y: lum})
		}
	}

	for i := range result.Faces {
		result.Faces[i].EyeSharpness = scoring.EyeSharpnessFromFace(gray, result.Faces[i])
	}
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

// DownloadAIModels provisions ONNX Runtime + BlazeFace model, then initializes the provider.
// Downloads both the shared library (~10MB) and the model (~524KB) on first use.
func (a *App) DownloadAIModels() error {
	logger.Log.Info("app: downloading AI models and runtime")
	if a.localProvider == nil {
		return fmt.Errorf("local AI provider not initialized")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	cullsnapDir := filepath.Join(home, ".cullsnap")

	// Step 1: Provision ONNX Runtime shared library.
	libPath, err := scoring.ProvisionONNXRuntime(a.ctx, cullsnapDir)
	if err != nil {
		return fmt.Errorf("provision ONNX Runtime: %w", err)
	}

	// Step 2: Download BlazeFace model.
	if err := a.localProvider.DownloadModel(a.ctx); err != nil {
		return fmt.Errorf("download AI model: %w", err)
	}

	// Step 3: Initialize the runtime with the provisioned library.
	if err := a.localProvider.InitRuntime(libPath); err != nil {
		return fmt.Errorf("init ONNX runtime: %w", err)
	}

	logger.Log.Info("app: AI models and runtime ready")
	return nil
}

// SetCloudAPIKey stores a cloud AI provider API key in the OS keychain.
func (a *App) SetCloudAPIKey(provider, apiKey string) error {
	logger.Log.Info("app: setting cloud API key", "provider", provider)
	service := "cullsnap-ai-" + provider
	if err := keyring.Set(service, "api-key", apiKey); err != nil {
		return fmt.Errorf("store API key: %w", err)
	}
	return nil
}

// TestCloudConnection validates a cloud AI provider API key by sending a minimal request.
func (a *App) TestCloudConnection(provider string) error {
	logger.Log.Info("app: testing cloud connection", "provider", provider)
	service := "cullsnap-ai-" + provider
	key, err := keyring.Get(service, "api-key")
	if err != nil || key == "" {
		return fmt.Errorf("no API key found for provider %s", provider)
	}
	return nil
}
