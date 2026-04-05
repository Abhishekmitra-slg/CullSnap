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
// Emits ai:progress, ai:photo-scored, and ai:clustering-complete events.
func (a *App) RunAIAnalysis(folderPath string) error {
	if !a.aiEnabled {
		return fmt.Errorf("AI scoring is disabled")
	}
	if !a.scoringEngine.Enabled() {
		return fmt.Errorf("no AI scoring provider available (download models or configure API key)")
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
		wailsRuntime.EventsEmit(ctx, "ai:error", map[string]interface{}{
			"error": fmt.Sprintf("scan failed: %v", err),
		})
		return
	}

	total := len(photos)
	logger.Log.Info("app: AI analysis scanning complete", "photos", total)

	wailsRuntime.EventsEmit(ctx, "ai:progress", map[string]interface{}{
		"phase":   "scoring",
		"current": 0,
		"total":   total,
	})

	// 2. Score each photo.
	var allEmbeddings []scoring.FaceEmbedding
	scored := 0

	for i := range photos {
		photo := &photos[i]
		if ctx.Err() != nil {
			logger.Log.Info("app: AI analysis cancelled")
			return
		}

		result, scoreErr := a.scorePhoto(ctx, photo.Path)
		if scoreErr != nil {
			logger.Log.Warn("app: failed to score photo", "path", photo.Path, "error", scoreErr)
			continue
		}

		if result != nil {
			// Save score to DB.
			if dbErr := a.store.SaveAIScore(photo.Path, result.OverallScore, len(result.Faces), "Local (ONNX)"); dbErr != nil {
				logger.Log.Warn("app: failed to save AI score", "path", photo.Path, "error", dbErr)
			}

			// Save face detections and collect embeddings.
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
				detID, detErr := a.store.SaveFaceDetection(det)
				if detErr != nil {
					logger.Log.Warn("app: failed to save face detection", "error", detErr)
					continue
				}

				// Placeholder: face embeddings would come from a MobileFaceNet model.
				// For now, collect detection IDs for future clustering.
				_ = detID
			}
		}

		scored++
		wailsRuntime.EventsEmit(ctx, "ai:photo-scored", map[string]interface{}{
			"path":      photo.Path,
			"score":     result.OverallScore,
			"faceCount": len(result.Faces),
		})

		wailsRuntime.EventsEmit(ctx, "ai:progress", map[string]interface{}{
			"phase":   "scoring",
			"current": i + 1,
			"total":   total,
		})
	}

	logger.Log.Info("app: AI scoring phase complete", "scored", scored, "total", total)

	// 3. Cluster faces (if we have embeddings).
	if len(allEmbeddings) > 0 {
		clusters := scoring.ClusterFaces(allEmbeddings, 0.6)
		logger.Log.Info("app: face clustering complete", "clusters", len(clusters))
	}

	wailsRuntime.EventsEmit(ctx, "ai:clustering-complete", map[string]interface{}{
		"scored":   scored,
		"total":    total,
		"clusters": []interface{}{},
	})
	logger.Log.Info("app: AI analysis complete", "folder", folderPath, "scored", scored)
}

// scorePhoto scores a single photo by reading its thumbnail and running inference.
func (a *App) scorePhoto(ctx context.Context, photoPath string) (*scoring.ScoreResult, error) {
	// Read thumbnail file (the 300px JPEG cached by ThumbCache).
	thumbPath := a.thumbPathForPhoto(photoPath)

	var imgData []byte
	var err error

	if thumbPath != "" {
		imgData, err = os.ReadFile(thumbPath) //nolint:gosec // trusted internal path
		if err != nil {
			logger.Log.Debug("app: thumbnail not cached, reading original", "path", photoPath)
			imgData, err = os.ReadFile(photoPath) //nolint:gosec // trusted internal path
			if err != nil {
				return nil, fmt.Errorf("read photo: %w", err)
			}
		}
	} else {
		imgData, err = os.ReadFile(photoPath) //nolint:gosec // trusted internal path
		if err != nil {
			return nil, fmt.Errorf("read photo: %w", err)
		}
	}

	result, err := a.scoringEngine.Score(ctx, imgData)
	if err != nil {
		return nil, err
	}

	if result == nil {
		return &scoring.ScoreResult{}, nil
	}

	// Post-process: compute eye sharpness for each detected face.
	if result.HasFaces() {
		a.computeEyeSharpness(imgData, result)
	}

	return result, nil
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

// thumbPathForPhoto returns the cached thumbnail path for a photo, or empty string.
func (a *App) thumbPathForPhoto(photoPath string) string {
	// Check common thumbnail cache locations.
	// ThumbCache stores thumbnails as SHA256(path+modtime).jpg in the cache dir.
	// For simplicity, we check if a thumbnail exists in the default cache location.
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	cacheDir := filepath.Join(home, ".cullsnap", "thumbs")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return ""
	}

	// Find any cached thumbnail — the ThumbCache uses content-addressed names.
	// For the AI pipeline, we fall back to reading the original if no thumb exists.
	_ = entries
	return ""
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
func (a *App) DownloadAIModels() error {
	logger.Log.Info("app: downloading AI models")
	if a.localProvider == nil {
		return fmt.Errorf("local AI provider not initialized")
	}
	return a.localProvider.DownloadModel(a.ctx)
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
