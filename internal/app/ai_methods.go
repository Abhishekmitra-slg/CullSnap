package app

import (
	"context"
	"cullsnap/internal/logger"
	"cullsnap/internal/model"
	"cullsnap/internal/scanner"
	"cullsnap/internal/scoring"
	"cullsnap/internal/storage"
	"cullsnap/internal/vlm"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// GetAIScoringStatus returns the current AI scoring configuration and plugin status.
func (a *App) GetAIScoringStatus() (*AIScoringStatus, error) {
	logger.Log.Debug("app: getting AI scoring status")
	if a.registry == nil {
		return &AIScoringStatus{Enabled: a.aiEnabled}, nil
	}

	statuses := a.registry.PluginStatuses()
	allModels := a.registry.AllModels()
	hasModels := true
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		logger.Log.Warn("app: failed to get home dir for model check", "error", homeErr)
		hasModels = false
	} else {
		for _, m := range allModels {
			if m.URL == "" {
				// Model spec is a placeholder — not yet sourced.
				continue
			}
			p := filepath.Join(home, ".cullsnap", "models", m.Filename)
			if _, err := os.Stat(p); err != nil {
				hasModels = false
				break
			}
		}
	}

	status := &AIScoringStatus{
		Enabled:   a.aiEnabled,
		Plugins:   statuses,
		Ready:     a.registry.Available(),
		HasModels: hasModels,
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
func (a *App) RunAIAnalysis(folderPath string) error {
	if !a.aiEnabled {
		return fmt.Errorf("AI scoring is disabled — enable it in Settings")
	}
	if a.registry == nil || !a.registry.Available() {
		return fmt.Errorf("no AI scoring plugins available — download models in Settings")
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

// ClearAIData removes all AI scores, face detections, and face clusters
// for the given folder so the next RunAIAnalysis re-processes every photo.
func (a *App) ClearAIData(folderPath string) error {
	logger.Log.Info("app: clearing AI data", "folder", folderPath)
	if err := a.store.DeleteAIDataForFolder(folderPath); err != nil {
		logger.Log.Error("app: failed to clear AI data", "error", err)
		return fmt.Errorf("clear AI data: %w", err)
	}
	return nil
}

// DownloadAIModels provisions ONNX Runtime and downloads all model files,
// then initialises the plugins.
func (a *App) DownloadAIModels() error {
	logger.Log.Info("app: downloading AI models and runtime")
	if a.registry == nil {
		return fmt.Errorf("plugin registry not initialized")
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
	logger.Log.Info("app: ONNX Runtime provisioned", "path", libPath)

	// Step 2: Download all model files via a shared ModelManager.
	// This manager uses the same directory (~/.cullsnap/models/) that each plugin's
	// own ModelManager references, so InitAll below will find the files correctly.
	mm, mmErr := scoring.NewModelManager(cullsnapDir)
	if mmErr != nil {
		return fmt.Errorf("create model manager: %w", mmErr)
	}
	allModels := a.registry.AllModels()
	mm.RegisterAll(allModels)
	if dlErr := mm.DownloadAll(a.ctx, nil); dlErr != nil {
		return fmt.Errorf("download models: %w", dlErr)
	}

	// Verify every model file exists before proceeding to init.
	modelsDir := filepath.Join(cullsnapDir, "models")
	for _, m := range allModels {
		p := filepath.Join(modelsDir, m.Filename)
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("model file missing after download: %s: %w", m.Filename, err)
		}
	}
	logger.Log.Info("app: all models downloaded and verified", "count", len(allModels))

	// Step 3: Initialise all plugins with the provisioned runtime.
	if initErr := a.registry.InitAll(libPath); initErr != nil {
		return fmt.Errorf("init plugins: %w", initErr)
	}

	logger.Log.Info("app: AI models and runtime ready")
	return nil
}

// GetAIWeights returns the current score blending weights.
func (a *App) GetAIWeights() AIWeightsConfig {
	if a.pipeline == nil {
		dw := scoring.DefaultWeights()
		return AIWeightsConfig{
			Aesthetic:   dw.Aesthetic,
			Sharpness:   dw.Sharpness,
			Face:        dw.Face,
			Eyes:        dw.Eyes,
			Composition: dw.Composition,
		}
	}
	w := a.pipeline.Weights()
	return AIWeightsConfig{
		Aesthetic:   w.Aesthetic,
		Sharpness:   w.Sharpness,
		Face:        w.Face,
		Eyes:        w.Eyes,
		Composition: w.Composition,
	}
}

// SetAIWeights updates the score blending weights and persists them.
func (a *App) SetAIWeights(weights AIWeightsConfig) error {
	logger.Log.Info("app: setting AI weights",
		"aesthetic", weights.Aesthetic,
		"sharpness", weights.Sharpness,
		"face", weights.Face,
		"eyes", weights.Eyes,
		"composition", weights.Composition,
	)
	sw := scoring.ScoreWeights{
		Aesthetic:   weights.Aesthetic,
		Sharpness:   weights.Sharpness,
		Face:        weights.Face,
		Eyes:        weights.Eyes,
		Composition: weights.Composition,
	}.Normalize()

	if a.pipeline != nil {
		a.pipeline.SetWeights(sw)
	}

	// Persist individual weights.
	_ = a.store.SetConfig("ai_weight_aesthetic", strconv.FormatFloat(sw.Aesthetic, 'f', 4, 64))
	_ = a.store.SetConfig("ai_weight_sharpness", strconv.FormatFloat(sw.Sharpness, 'f', 4, 64))
	_ = a.store.SetConfig("ai_weight_face", strconv.FormatFloat(sw.Face, 'f', 4, 64))
	_ = a.store.SetConfig("ai_weight_eyes", strconv.FormatFloat(sw.Eyes, 'f', 4, 64))
	_ = a.store.SetConfig("ai_weight_composition", strconv.FormatFloat(sw.Composition, 'f', 4, 64))
	return nil
}

// loadAIWeights reads persisted weights from config KV and sets them on the pipeline.
func (a *App) loadAIWeights() {
	if a.pipeline == nil {
		return
	}
	w := scoring.DefaultWeights()
	if v, _ := a.store.GetConfig("ai_weight_aesthetic"); v != "" {
		w.Aesthetic, _ = strconv.ParseFloat(v, 64)
	}
	if v, _ := a.store.GetConfig("ai_weight_sharpness"); v != "" {
		w.Sharpness, _ = strconv.ParseFloat(v, 64)
	}
	if v, _ := a.store.GetConfig("ai_weight_face"); v != "" {
		w.Face, _ = strconv.ParseFloat(v, 64)
	}
	if v, _ := a.store.GetConfig("ai_weight_eyes"); v != "" {
		w.Eyes, _ = strconv.ParseFloat(v, 64)
	}
	if compStr, _ := a.store.GetConfig("ai_weight_composition"); compStr != "" {
		if v, err := strconv.ParseFloat(compStr, 64); err == nil {
			w.Composition = v
		}
	}
	w = w.Normalize()
	a.pipeline.SetWeights(w)
	logger.Log.Debug("app: loaded AI weights",
		"aesthetic", w.Aesthetic,
		"sharpness", w.Sharpness,
		"face", w.Face,
		"eyes", w.Eyes,
		"composition", w.Composition,
	)
}

// ── Internal pipeline ───────────────────────────────────────────────────────

// runAIAnalysisPipeline scans photos, scores via worker pool, then clusters faces.
func (a *App) runAIAnalysisPipeline(ctx context.Context, folderPath string) {
	start := time.Now()

	// Step 1: Scan directory for photos.
	wailsRuntime.EventsEmit(a.ctx, "ai:step-started", AIStepEvent{Step: "scanning", Total: 0})
	photos, err := scanner.ScanDirectory(folderPath)
	if err != nil {
		logger.Log.Error("app: AI analysis scan failed", "error", err)
		wailsRuntime.EventsEmit(a.ctx, "ai:error", map[string]interface{}{
			"step":  "scanning",
			"error": fmt.Sprintf("Scan failed: %v", err),
		})
		return
	}
	total := len(photos)
	if total == 0 {
		logger.Log.Info("app: no photos found for AI analysis", "folder", folderPath)
		wailsRuntime.EventsEmit(a.ctx, "ai:error", map[string]interface{}{
			"step":  "scanning",
			"error": "No photos found in this folder",
		})
		return
	}
	wailsRuntime.EventsEmit(a.ctx, "ai:step-completed", AIStepEvent{Step: "scanning", Total: total})
	logger.Log.Info("app: AI analysis scanning complete", "photos", total)

	// Step 2: Filter already-scored photos (check mtime).
	wailsRuntime.EventsEmit(a.ctx, "ai:step-started", AIStepEvent{Step: "filtering", Total: total})
	var toScore []model.Photo
	for i := range photos {
		if ctx.Err() != nil {
			return
		}
		existing, _ := a.store.GetAIScore(photos[i].Path)
		if existing != nil {
			info, statErr := os.Stat(photos[i].Path)
			if statErr == nil && !info.ModTime().After(existing.ScoredAt) {
				// Already scored and file hasn't changed — skip.
				continue
			}
		}
		toScore = append(toScore, photos[i])
	}
	wailsRuntime.EventsEmit(a.ctx, "ai:step-completed", AIStepEvent{Step: "filtering", Total: len(toScore)})
	logger.Log.Info("app: AI analysis filtering complete", "toScore", len(toScore), "skipped", total-len(toScore))

	if len(toScore) == 0 {
		// All photos already scored — skip to clustering.
		clusterCount := a.runFaceClustering(folderPath)
		elapsed := time.Since(start).Milliseconds()
		wailsRuntime.EventsEmit(a.ctx, "ai:complete", AICompleteEvent{
			Scored:    0,
			Faces:     0,
			Clusters:  clusterCount,
			ElapsedMs: elapsed,
		})
		return
	}

	// Step 3: Score photos with worker pool.
	wailsRuntime.EventsEmit(a.ctx, "ai:step-started", AIStepEvent{Step: "scoring", Total: len(toScore)})

	numWorkers := workerCount()
	var scored int64
	var totalFaces int64
	var topPhoto string
	var topScore float64
	var topMu sync.Mutex

	work := make(chan int, len(toScore))
	for i := range toScore {
		work <- i
	}
	close(work)

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range work {
				if ctx.Err() != nil {
					return
				}
				a.scoreAndSavePhoto(ctx, toScore[idx].Path, &scored, &totalFaces, len(toScore), &topPhoto, &topScore, &topMu)
			}
		}()
	}
	wg.Wait()

	if ctx.Err() != nil {
		logger.Log.Info("app: AI analysis cancelled during scoring")
		wailsRuntime.EventsEmit(a.ctx, "ai:error", map[string]interface{}{
			"step":  "scoring",
			"error": "Analysis cancelled",
		})
		return
	}

	wailsRuntime.EventsEmit(a.ctx, "ai:step-completed", AIStepEvent{Step: "scoring", Total: int(scored)})
	logger.Log.Info("app: AI scoring phase complete", "scored", scored, "faces", totalFaces, "total", len(toScore))

	// Step 4: Cluster faces.
	wailsRuntime.EventsEmit(a.ctx, "ai:step-started", AIStepEvent{Step: "clustering", Total: 0})
	clusterCount := a.runFaceClustering(folderPath)
	wailsRuntime.EventsEmit(a.ctx, "ai:step-completed", AIStepEvent{Step: "clustering", Total: clusterCount})

	// VLM Stages — probe hardware once and reuse.
	if a.vlmManager != nil {
		hwProfile := a.vlmHWProfile
		plan := vlm.BuildExecutionPlan(int(scored), hwProfile.Tier)

		// Step 5: VLM Stage 4 — Individual photo scoring.
		// Pass the full toScore slice — runVLMStage4 re-queries ONNX scores from DB
		// to apply the adaptive threshold, so it handles partial scoring correctly.
		if plan.VLMEnabled {
			a.runVLMStage4(ctx, folderPath, toScore, plan)
		}

		// Step 6: VLM Stage 5 — Pairwise ranking.
		if plan.Stage5Count > 0 {
			a.runVLMStage5(ctx, folderPath, plan)
		}
	}

	elapsed := time.Since(start).Milliseconds()
	wailsRuntime.EventsEmit(a.ctx, "ai:complete", AICompleteEvent{
		Scored:    int(scored),
		Faces:     int(totalFaces),
		Clusters:  clusterCount,
		ElapsedMs: elapsed,
		TopPhoto:  topPhoto,
		TopScore:  topScore,
	})
	logger.Log.Info("app: AI analysis complete",
		"folder", folderPath,
		"scored", scored,
		"faces", totalFaces,
		"clusters", clusterCount,
		"elapsedMs", elapsed,
	)
}

// scoreAndSavePhoto scores a single photo, saves results to DB, and emits events.
func (a *App) scoreAndSavePhoto(
	ctx context.Context,
	photoPath string,
	scored, totalFaces *int64,
	total int,
	topPhoto *string,
	topScore *float64,
	topMu *sync.Mutex,
) {
	// Read the photo data (prefer cached thumbnail).
	f, err := a.readPhotoForScoring(photoPath)
	if err != nil {
		logger.Log.Warn("app: failed to read photo for scoring", "path", photoPath, "error", err)
		return
	}
	defer f.Close() //nolint:errcheck // best-effort close

	img, _, decErr := image.Decode(f)
	if decErr != nil {
		logger.Log.Warn("app: failed to decode image for scoring", "path", photoPath, "error", decErr)
		return
	}

	// Run the pipeline.
	cs, pipeErr := a.pipeline.Execute(ctx, img)
	if pipeErr != nil {
		logger.Log.Warn("app: pipeline failed", "path", photoPath, "error", pipeErr)
		return
	}
	if cs == nil {
		logger.Log.Warn("app: pipeline returned nil score", "path", photoPath)
		return
	}

	// Compute overall score with current weights.
	weights := a.pipeline.Weights()
	overall := cs.OverallScore(weights)

	// Save AIScore to DB.
	aiScore := &storage.AIScore{
		PhotoPath:         photoPath,
		OverallScore:      overall,
		FaceCount:         cs.FaceCount,
		Provider:          cs.Provider,
		AestheticScore:    cs.AestheticScore,
		SharpnessScore:    cs.SharpnessScore,
		BestFaceSharpness: cs.BestFaceSharp,
		EyeOpenness:       cs.EyeOpenness,
	}
	if dbErr := a.store.SaveAIScore(aiScore); dbErr != nil {
		logger.Log.Warn("app: failed to save AI score", "path", photoPath, "error", dbErr)
	}

	// Save face detections and embeddings.
	faceCount := len(cs.Faces)
	for i, face := range cs.Faces {
		// Only assign sharpness to the best face; others get 0.
		eyeSharp := 0.0
		if i == cs.BestFaceIdx {
			eyeSharp = cs.BestFaceSharp
		}
		det := &storage.FaceDetection{
			PhotoPath:    photoPath,
			BboxX:        face.BoundingBox.Min.X,
			BboxY:        face.BoundingBox.Min.Y,
			BboxW:        face.BoundingBox.Dx(),
			BboxH:        face.BoundingBox.Dy(),
			Confidence:   face.Confidence,
			EyeSharpness: eyeSharp,
		}
		// Attach embedding if available.
		if i < len(cs.Embeddings) && len(cs.Embeddings[i]) > 0 {
			det.Embedding = float32SliceToBytes(cs.Embeddings[i])
		}
		detID, detErr := a.store.SaveFaceDetection(det)
		if detErr != nil {
			logger.Log.Warn("app: failed to save face detection", "error", detErr)
		}
		logger.Log.Debug("app: saved face detection", "path", photoPath, "detID", detID, "face", i)
	}

	count := atomic.AddInt64(scored, 1)
	atomic.AddInt64(totalFaces, int64(faceCount))

	// Track top scoring photo.
	topMu.Lock()
	if overall > *topScore {
		*topScore = overall
		*topPhoto = photoPath
	}
	topMu.Unlock()

	// Emit events.
	wailsRuntime.EventsEmit(a.ctx, "ai:photo-scored", map[string]interface{}{
		"path":      photoPath,
		"score":     overall,
		"faceCount": faceCount,
	})
	wailsRuntime.EventsEmit(a.ctx, "ai:step-progress", AIStepEvent{
		Step:      "scoring",
		Current:   int(count),
		Total:     total,
		PhotoPath: photoPath,
	})
}

// readPhotoForScoring opens a photo file for scoring, preferring the cached thumbnail.
func (a *App) readPhotoForScoring(photoPath string) (*os.File, error) {
	// Try the thumbnail cache first — it's a 300px JPEG that's fast to decode
	// and already handles RAW/HEIC conversion.
	if a.thumbCache != nil {
		info, err := os.Stat(photoPath)
		if err == nil {
			thumbPath := a.thumbCache.GetCachedPath(photoPath, info.ModTime())
			if thumbPath != "" {
				f, readErr := os.Open(thumbPath) //nolint:gosec // trusted internal path
				if readErr == nil {
					logger.Log.Debug("app: scoring from cached thumbnail", "path", photoPath)
					return f, nil
				}
			}
		}
	}

	// Fallback: read the original file.
	f, err := os.Open(photoPath) //nolint:gosec // trusted internal path
	if err != nil {
		return nil, err
	}

	logger.Log.Debug("app: scoring from original file", "path", photoPath)
	return f, nil
}

// forwardVLMEvents relays VLM manager events to the frontend via Wails events.
func (a *App) forwardVLMEvents() {
	for evt := range a.vlmEvents {
		wailsRuntime.EventsEmit(a.ctx, "vlm:state-change", evt)
	}
}

// runVLMStage4 scores individual photos via the VLM, selecting top-N by ONNX score.
func (a *App) runVLMStage4(ctx context.Context, folderPath string, photos []model.Photo, plan vlm.ExecutionPlan) {
	// Adaptive threshold: only send top N photos to VLM based on ONNX scores.
	vlmPhotos := photos
	if plan.Stage4Count > 0 && plan.Stage4Count < len(photos) {
		type scoredPhoto struct {
			photo model.Photo
			score float64
		}
		sp := make([]scoredPhoto, 0, len(photos))
		for i := range photos {
			p := &photos[i]
			aiScore, _ := a.store.GetAIScore(p.Path)
			var s float64
			if aiScore != nil {
				s = aiScore.OverallScore
			}
			sp = append(sp, scoredPhoto{photo: *p, score: s})
		}
		sort.Slice(sp, func(i, j int) bool { return sp[i].score > sp[j].score })
		vlmPhotos = make([]model.Photo, plan.Stage4Count)
		for i := 0; i < plan.Stage4Count; i++ {
			vlmPhotos[i] = sp[i].photo
		}
		logger.Log.Info("app: VLM adaptive threshold applied", "original", len(photos), "selected", len(vlmPhotos))
	}

	wailsRuntime.EventsEmit(a.ctx, "ai:step-started", AIStepEvent{Step: "describe", Total: len(vlmPhotos)})
	logger.Log.Info("app: VLM Stage 4 starting", "photos", len(vlmPhotos))

	modelInfo := a.vlmManager.ProviderModelInfo()
	// Snapshot custom instructions + hash once per run so the cache key, the
	// LLM input, and the saved row all agree even if the user edits the
	// suffix mid-analysis.
	customInstructions := a.vlmManager.CustomInstructions()
	customHash := vlm.HashCustomInstructions(customInstructions)
	stage4Start := time.Now()
	var totalTokens int
	var scoredCount int
	var cacheHits int

	for i := range vlmPhotos {
		photo := &vlmPhotos[i]
		if ctx.Err() != nil {
			return
		}

		// Cache hit: previous score with matching prompt version AND custom
		// instructions hash means the LLM would produce the same result —
		// skip the call to save tokens.
		if cached, _ := a.store.GetVLMScore(photo.Path); cached != nil &&
			cached.PromptVersion == vlm.PromptVersion &&
			cached.CustomInstructionsHash == customHash {
			cacheHits++
			wailsRuntime.EventsEmit(a.ctx, "ai:step-progress", AIStepEvent{
				Step: "describe", Current: i + 1, Total: len(vlmPhotos), PhotoPath: photo.Path,
			})
			continue
		}

		// Get ONNX scores for context injection.
		aiScore, _ := a.store.GetAIScore(photo.Path)
		var faceCount int
		var sharpness float64
		if aiScore != nil {
			faceCount = aiScore.FaceCount
			sharpness = aiScore.SharpnessScore
		}

		req := vlm.ScoreRequest{
			PhotoPath:          photo.Path,
			FaceCount:          faceCount,
			Sharpness:          sharpness,
			TokenBudget:        280,
			CustomInstructions: customInstructions,
		}

		score, err := a.vlmManager.ScorePhoto(ctx, req)
		if err != nil {
			logger.Log.Warn("app: VLM score failed for photo", "path", photo.Path, "error", err)
			continue
		}

		totalTokens += score.TokensUsed
		scoredCount++

		// Save to DB.
		a.store.SaveVLMScore(storage.VLMScoreRow{ //nolint:errcheck // best-effort persistence
			PhotoPath:              photo.Path,
			FolderPath:             folderPath,
			Aesthetic:              score.Aesthetic,
			Composition:            score.Composition,
			Expression:             score.Expression,
			TechnicalQual:          score.TechnicalQual,
			SceneType:              score.SceneType,
			Issues:                 mustMarshalJSON(score.Issues),
			Explanation:            score.Explanation,
			TokensUsed:             score.TokensUsed,
			ModelName:              modelInfo.Name,
			ModelVariant:           modelInfo.Variant,
			Backend:                modelInfo.Backend,
			PromptVersion:          vlm.PromptVersion,
			CustomInstructionsHash: customHash,
		})

		// Emit per-photo result.
		wailsRuntime.EventsEmit(a.ctx, "ai:photo-described", VLMPhotoResult{
			PhotoPath:   photo.Path,
			Aesthetic:   score.Aesthetic,
			Composition: score.Composition,
			SceneType:   score.SceneType,
			Explanation: score.Explanation,
		})
		wailsRuntime.EventsEmit(a.ctx, "ai:step-progress", AIStepEvent{
			Step: "describe", Current: i + 1, Total: len(vlmPhotos), PhotoPath: photo.Path,
		})
	}

	wailsRuntime.EventsEmit(a.ctx, "ai:step-completed", AIStepEvent{Step: "describe", Total: len(vlmPhotos)})
	stage4Elapsed := time.Since(stage4Start)
	logger.Log.Info("app: VLM Stage 4 complete",
		"photos", len(vlmPhotos),
		"scored", scoredCount,
		"cacheHits", cacheHits,
		"elapsed", stage4Elapsed,
	)

	// Persist calibration data for future time estimates.
	if len(vlmPhotos) > 0 {
		msPerPhoto := int(stage4Elapsed.Milliseconds()) / len(vlmPhotos)
		key := vlm.CalibrationKey(plan.HardwareTier, modelInfo.Backend, modelInfo.Name)
		_ = a.store.SetConfig(key, strconv.Itoa(msPerPhoto))
	}

	// Record token usage.
	if scoredCount > 0 {
		_ = a.store.RecordTokenUsage(storage.TokenUsageRow{
			Provider:     modelInfo.Backend + "/" + modelInfo.Name,
			FolderPath:   folderPath,
			Stage:        "stage4",
			TokensOutput: totalTokens,
			PhotoCount:   scoredCount,
		})
	}
}

// runVLMStage5 ranks photos within face clusters using the VLM.
func (a *App) runVLMStage5(ctx context.Context, folderPath string, plan vlm.ExecutionPlan) {
	clusters, err := a.store.GetFaceClusters(folderPath)
	if err != nil || len(clusters) == 0 {
		logger.Log.Debug("app: VLM Stage 5 skipped — no clusters")
		return
	}

	wailsRuntime.EventsEmit(a.ctx, "ai:step-started", AIStepEvent{Step: "rank", Total: len(clusters)})
	logger.Log.Info("app: VLM Stage 5 starting", "groups", len(clusters))

	// Snapshot custom instructions + hash once per run; same reasoning as Stage 4.
	customInstructions := a.vlmManager.CustomInstructions()
	customHash := vlm.HashCustomInstructions(customInstructions)

	// Pre-load existing rankings into a label->row map so the per-cluster cache
	// check is a hash lookup, not an extra query each iteration.
	cachedRankings := make(map[string]storage.VLMRankingGroupRow)
	if existing, errLoad := a.store.GetVLMRankingsForFolder(folderPath); errLoad == nil {
		for _, g := range existing {
			cachedRankings[g.GroupLabel] = g
		}
	}

	var stage5Tokens, stage5Groups, stage5CacheHits int

	for i, cluster := range clusters {
		if ctx.Err() != nil {
			return
		}
		if cluster.PhotoCount < 2 || cluster.PhotoCount > 5 {
			continue // Only rank groups of 2-5 photos.
		}

		// Cache hit: cluster already ranked under the current prompt version
		// and custom-instructions hash — skip the LLM call.
		if cached, ok := cachedRankings[cluster.Label]; ok &&
			cached.PromptVersion == vlm.PromptVersion &&
			cached.CustomInstructionsHash == customHash {
			stage5CacheHits++
			wailsRuntime.EventsEmit(a.ctx, "ai:step-progress", AIStepEvent{
				Step: "rank", Current: i + 1, Total: len(clusters),
			})
			continue
		}

		detections, _ := a.store.GetFaceDetectionsForCluster(cluster.ID)
		if len(detections) < 2 {
			continue
		}

		// Collect unique photo paths from detections (a photo can have multiple faces).
		seen := make(map[string]bool)
		paths := make([]string, 0, 5)
		photoScores := make([]vlm.PhotoContext, 0, 5)
		for _, d := range detections {
			if seen[d.PhotoPath] || len(paths) >= 5 {
				continue
			}
			seen[d.PhotoPath] = true
			vlmScore, _ := a.store.GetVLMScore(d.PhotoPath)
			pc := vlm.PhotoContext{}
			if vlmScore != nil {
				pc.Aesthetic = vlmScore.Aesthetic
				pc.Sharpness = vlmScore.TechnicalQual
			}
			paths = append(paths, d.PhotoPath)
			photoScores = append(photoScores, pc)
		}

		if len(paths) < 2 {
			continue
		}

		req := vlm.RankRequest{
			PhotoPaths:         paths,
			TokenBudget:        280,
			PhotoScores:        photoScores,
			CustomInstructions: customInstructions,
		}

		result, err := a.vlmManager.RankPhotos(ctx, req)
		if err != nil {
			logger.Log.Warn("app: VLM ranking failed", "cluster", cluster.Label, "error", err)
			continue
		}

		stage5Tokens += result.TokensUsed
		stage5Groups++

		// Save ranking.
		modelInfo := a.vlmManager.ProviderModelInfo()
		rankings := make([]storage.VLMRankingRow, len(result.Ranked))
		for j, r := range result.Ranked {
			rankings[j] = storage.VLMRankingRow{
				PhotoPath:     r.PhotoPath,
				Rank:          r.Rank,
				RelativeScore: r.Score,
				Notes:         r.Notes,
			}
		}
		_ = a.store.SaveVLMRanking(storage.VLMRankingGroupRow{ //nolint:errcheck // best-effort persistence
			FolderPath:             folderPath,
			GroupLabel:             cluster.Label,
			PhotoCount:             len(rankings),
			Explanation:            result.Explanation,
			ModelName:              modelInfo.Name,
			PromptVersion:          vlm.PromptVersion,
			CustomInstructionsHash: customHash,
			Rankings:               rankings,
		})

		wailsRuntime.EventsEmit(a.ctx, "ai:step-progress", AIStepEvent{
			Step: "rank", Current: i + 1, Total: len(clusters),
		})
	}

	wailsRuntime.EventsEmit(a.ctx, "ai:step-completed", AIStepEvent{Step: "rank", Total: len(clusters)})
	logger.Log.Info("app: VLM Stage 5 complete",
		"groups", stage5Groups,
		"cacheHits", stage5CacheHits,
		"tokens", stage5Tokens,
	)

	// Record token usage.
	if stage5Groups > 0 {
		modelInfo := a.vlmManager.ProviderModelInfo()
		_ = a.store.RecordTokenUsage(storage.TokenUsageRow{
			Provider:     modelInfo.Backend + "/" + modelInfo.Name,
			FolderPath:   folderPath,
			Stage:        "stage5",
			TokensOutput: stage5Tokens,
			PhotoCount:   stage5Groups,
		})
	}
}

// GetVLMStatus returns the current VLM engine status.
func (a *App) GetVLMStatus() VLMStatus {
	if a.vlmManager == nil {
		return VLMStatus{State: "unavailable"}
	}
	s := a.vlmManager.Status()
	prof := a.vlmHWProfile
	return vlmStatusFromManager(s, prof.Tier)
}

// StartVLMEngine starts the VLM inference engine.
func (a *App) StartVLMEngine() error {
	if a.vlmManager == nil {
		return fmt.Errorf("VLM not initialized")
	}
	return a.vlmManager.EnsureRunning(a.ctx)
}

// StopVLMEngine stops the VLM inference engine.
func (a *App) StopVLMEngine() error {
	if a.vlmManager == nil {
		return nil
	}
	return a.vlmManager.Stop(a.ctx)
}

// GetVLMScoresForPhoto returns VLM scores for a specific photo.
func (a *App) GetVLMScoresForPhoto(photoPath string) (*storage.VLMScoreRow, error) {
	return a.store.GetVLMScore(photoPath)
}

// GetVLMRankingsForFolder returns all VLM ranking groups for a folder.
func (a *App) GetVLMRankingsForFolder(folderPath string) ([]storage.VLMRankingGroupRow, error) {
	return a.store.GetVLMRankingsForFolder(folderPath)
}

// DownloadVLMModel provisions the VLM runtime and downloads a model.
func (a *App) DownloadVLMModel(modelName string) error {
	logger.Log.Info("app: downloading VLM model", "model", modelName)

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	cullsnapDir := filepath.Join(home, ".cullsnap")

	hwProfile := a.vlmHWProfile
	backend := vlm.RecommendBackend(hwProfile)

	entry, ok := vlm.LookupModel(modelName, backend)
	// Fall back to the other backend when (a) no entry matches the recommended
	// backend, or (b) the matching entry is marked unavailable (e.g. MLX until
	// the download pipeline redesign lands).
	if !ok || !entry.Available {
		other := "mlx"
		if backend == "mlx" {
			other = "llamacpp"
		}
		logger.Log.Debug("app: VLM backend unavailable, trying fallback",
			"requested", backend, "fallback", other, "found", ok, "available", entry.Available)
		backend = other
		entry, ok = vlm.LookupModel(modelName, backend)
		if !ok {
			return fmt.Errorf("model %q not found in registry", modelName)
		}
		if !entry.Available {
			return fmt.Errorf("model %q has no available backend (both llamacpp and mlx entries are marked unavailable)", modelName)
		}
	}

	if err := vlm.CheckDiskSpace(cullsnapDir, entry.SizeBytes); err != nil {
		return err
	}

	destPath := vlm.ModelDownloadPath(cullsnapDir, entry.Filename)
	wailsRuntime.EventsEmit(a.ctx, "vlm:download-progress", map[string]interface{}{
		"stage": "model", "model": modelName, "total": entry.SizeBytes,
	})

	_, dlErr := vlm.DownloadFileResumable(a.ctx, entry.URL, destPath, entry.SHA256,
		func(downloaded, total int64) {
			wailsRuntime.EventsEmit(a.ctx, "vlm:download-progress", map[string]interface{}{
				"stage": "model", "downloaded": downloaded, "total": total,
			})
		})
	if dlErr != nil {
		return fmt.Errorf("download model: %w", dlErr)
	}

	// Provision runtime (llama-server binary) if needed.
	if backend == "llamacpp" {
		if err := a.provisionLlamaServer(cullsnapDir); err != nil {
			return fmt.Errorf("provision llama-server: %w", err)
		}
	}

	// Configure the manager with the downloaded model.
	cfg := a.vlmManager.Config()
	cfg.ModelName = modelName
	cfg.PreferredBackend = backend
	a.vlmManager.UpdateConfig(cfg)

	// Set up the provider backend.
	var provider vlm.VLMProvider
	if backend == "llamacpp" {
		binaryPath := vlm.LlamaServerBinaryPath(cullsnapDir)
		provider = vlm.NewLlamaCppBackend(binaryPath, destPath, entry)
	} else {
		venvPath := vlm.MLXVenvPath(cullsnapDir)
		modelPath := vlm.MLXModelPath(cullsnapDir, entry.Filename)
		provider = vlm.NewMLXBackend(venvPath, modelPath, entry)
	}
	a.vlmManager.SetProvider(provider)

	// Persist setup state.
	_ = a.store.SetConfig(vlm.ConfigKeyModelName, modelName)
	_ = a.store.SetConfig(vlm.ConfigKeyModelVariant, entry.Variant)
	_ = a.store.SetConfig(vlm.ConfigKeyBackend, backend)
	_ = a.store.SetConfig(vlm.ConfigKeySetupComplete, "true")

	logger.Log.Info("app: VLM model downloaded and configured",
		"model", modelName, "backend", backend, "variant", entry.Variant)
	return nil
}

// provisionLlamaServer downloads and extracts the llama-server runtime if the
// on-disk state is not already usable.
//
// The llama.cpp release ships the binary plus its shared libraries inside a
// zip under "build/bin/". We download that zip to a dedicated staging path
// (never the binary path — a previous version of this function wrote the zip
// to the binary path, and would-be exec of the zip failed silently at runtime),
// verify its SHA256, extract the allow-listed runtime files, and delete the zip.
// See vlm.ExtractLlamaServerZip for the allowlist.
func (a *App) provisionLlamaServer(cullsnapDir string) error {
	binaryPath := vlm.LlamaServerBinaryPath(cullsnapDir)
	if vlm.LlamaServerRuntimeReady(cullsnapDir) {
		logger.Log.Debug("app: llama-server runtime already present", "path", binaryPath)
		return nil
	}

	wailsRuntime.EventsEmit(a.ctx, "vlm:download-progress", map[string]interface{}{
		"stage": "runtime", "runtime": "llama-server",
	})

	url := vlm.LlamaServerDownloadURL()
	if url == "" {
		return fmt.Errorf("no llama-server download URL for %s/%s", stdruntime.GOOS, stdruntime.GOARCH)
	}

	// A missing SHA256 entry for a supported platform is a registry bug, not a
	// reason to silently skip integrity verification. Refuse to provision.
	expectedSHA := vlm.LlamaServerSHA256()
	if expectedSHA == "" {
		return fmt.Errorf("no llama-server SHA256 registered for %s/%s — refusing to download unverified binary", stdruntime.GOOS, stdruntime.GOARCH)
	}

	binDir := filepath.Dir(binaryPath)
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("create llama-server bin dir: %w", err)
	}

	// Legacy installs wrote the zip straight to binaryPath. Remove that so the
	// new flow can cleanly write the extracted binary to the same location
	// without a hash-mismatch loop inside DownloadFileResumable.
	if isZip, _ := vlm.IsLegacyZipAtBinaryPath(binaryPath); isZip {
		logger.Log.Warn("app: removing legacy zip-as-binary before re-provisioning", "path", binaryPath)
		if err := os.Remove(binaryPath); err != nil {
			return fmt.Errorf("remove legacy zip-as-binary: %w", err)
		}
	}

	zipPath := vlm.LlamaServerZipPath(cullsnapDir)
	if _, err := vlm.DownloadFileResumable(a.ctx, url, zipPath, expectedSHA, nil); err != nil {
		return fmt.Errorf("download llama-server zip: %w", err)
	}

	if _, err := vlm.ExtractLlamaServerZip(zipPath, binDir); err != nil {
		return fmt.Errorf("extract llama-server zip: %w", err)
	}

	if err := os.Remove(zipPath); err != nil && !os.IsNotExist(err) {
		logger.Log.Warn("app: could not remove llama-server zip after extract",
			"path", zipPath, "err", err)
	}

	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("llama-server binary missing after extract: %w", err)
	}
	return nil
}

// mustMarshalJSON marshals v to JSON, returning "[]" on error.
func mustMarshalJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// runFaceClustering loads embeddings from DB, clusters them, and saves results.
// Returns the number of clusters created.
func (a *App) runFaceClustering(folderPath string) int {
	// Delete existing clusters for this folder so they don't accumulate on re-run.
	if err := a.store.DeleteFaceClustersForFolder(folderPath); err != nil {
		logger.Log.Warn("app: clustering: failed to delete old clusters", "error", err)
	}

	// Load all face detections for this folder that have embeddings.
	scores, err := a.store.GetAIScoresForFolder(folderPath)
	if err != nil {
		logger.Log.Warn("app: clustering: failed to load scores", "error", err)
		return 0
	}

	var embeddings []scoring.FaceEmbedding
	for _, score := range scores {
		dets, detErr := a.store.GetFaceDetections(score.PhotoPath)
		if detErr != nil {
			continue
		}
		for _, det := range dets {
			if len(det.Embedding) == 0 {
				continue
			}
			emb := bytesToFloat32Slice(det.Embedding)
			if len(emb) == 0 {
				continue
			}
			embeddings = append(embeddings, scoring.FaceEmbedding{
				PhotoPath:   det.PhotoPath,
				DetectionID: det.ID,
				Embedding:   emb,
			})
		}
	}

	if len(embeddings) == 0 {
		logger.Log.Debug("app: clustering: no embeddings to cluster", "folder", folderPath)
		return 0
	}

	logger.Log.Info("app: clustering faces", "embeddings", len(embeddings), "folder", folderPath)

	clusters := scoring.ClusterFacesAgglomerative(embeddings, scoring.DefaultClusterThreshold)

	// Save clusters to DB.
	for i, cluster := range clusters {
		if len(cluster.Faces) == 0 {
			continue
		}

		// Find the representative photo (highest-scored face).
		representative := cluster.Faces[0].PhotoPath
		bestScore := -1.0
		for _, face := range cluster.Faces {
			score, _ := a.store.GetAIScore(face.PhotoPath)
			if score != nil && score.OverallScore > bestScore {
				bestScore = score.OverallScore
				representative = face.PhotoPath
			}
		}

		dbCluster := &storage.FaceCluster{
			FolderPath:         folderPath,
			Label:              fmt.Sprintf("Person %d", i+1),
			RepresentativePath: representative,
			PhotoCount:         len(cluster.Faces),
		}
		clusterID, saveErr := a.store.SaveFaceCluster(dbCluster)
		if saveErr != nil {
			logger.Log.Warn("app: clustering: failed to save cluster", "error", saveErr)
			continue
		}

		// Assign detections to cluster.
		for _, face := range cluster.Faces {
			if assignErr := a.store.AssignFaceToCluster(face.DetectionID, clusterID); assignErr != nil {
				logger.Log.Warn("app: clustering: failed to assign face", "error", assignErr)
			}
		}
	}

	logger.Log.Info("app: clustering complete", "clusters", len(clusters), "folder", folderPath)
	return len(clusters)
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// workerCount returns the number of worker goroutines for the scoring pool.
// Uses NumCPU/2 clamped to [1, 8].
func workerCount() int {
	n := maxInt(1, stdruntime.NumCPU()/2)
	return minInt(n, 8)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// float32SliceToBytes converts a []float32 to []byte using little-endian encoding.
func float32SliceToBytes(fs []float32) []byte {
	buf := make([]byte, len(fs)*4)
	for i, f := range fs {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// bytesToFloat32Slice converts a []byte back to []float32 using little-endian encoding.
func bytesToFloat32Slice(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	fs := make([]float32, len(b)/4)
	for i := range fs {
		fs[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return fs
}

// GetVLMDetailedStatus returns VLM engine status with runtime stats.
func (a *App) GetVLMDetailedStatus() VLMDetailedStatus {
	if a.vlmManager == nil {
		return VLMDetailedStatus{VLMStatus: VLMStatus{State: "unavailable"}}
	}
	s := a.vlmManager.Status()
	prof := a.vlmHWProfile
	return VLMDetailedStatus{
		VLMStatus:    vlmStatusFromManager(s, prof.Tier),
		RestartCount: s.RestartCount,
		InferCount:   s.InferCount,
		RAMUsageMB:   s.RAMUsageMB,
	}
}

// GetAIStorageInfo returns disk usage for AI models, runtime, and scores DB.
func (a *App) GetAIStorageInfo() AIStorageInfo {
	home, err := os.UserHomeDir()
	if err != nil {
		return AIStorageInfo{}
	}
	cullsnapDir := filepath.Join(home, ".cullsnap")

	modelName, _ := a.store.GetConfig(vlm.ConfigKeyModelName)
	backend, _ := a.store.GetConfig(vlm.ConfigKeyBackend)

	var modelSize, runtimeSize int64
	var runtimeName string

	// Scan models directory for total model size.
	modelsDir := filepath.Join(cullsnapDir, "models")
	if entries, dirErr := os.ReadDir(modelsDir); dirErr == nil {
		for _, e := range entries {
			if info, infoErr := e.Info(); infoErr == nil {
				modelSize += info.Size()
			}
		}
	}

	// Runtime size: llama-server binary or MLX venv.
	if backend == "mlx" {
		runtimeName = "MLX venv"
		runtimeSize = dirSizeBytes(vlm.MLXVenvPath(cullsnapDir))
	} else {
		runtimeName = "llama-server"
		binaryPath := vlm.LlamaServerBinaryPath(cullsnapDir)
		if info, statErr := os.Stat(binaryPath); statErr == nil {
			runtimeSize = info.Size()
		}
	}

	// DB size.
	dbPath := filepath.Join(cullsnapDir, "cullsnap.db")
	var dbSize int64
	if info, statErr := os.Stat(dbPath); statErr == nil {
		dbSize = info.Size()
	}

	return AIStorageInfo{
		ModelSizeMB:    modelSize / (1024 * 1024),
		RuntimeSizeMB:  runtimeSize / (1024 * 1024),
		ScoresDBSizeMB: dbSize / (1024 * 1024),
		TotalMB:        (modelSize + runtimeSize + dbSize) / (1024 * 1024),
		ModelName:      modelName,
		RuntimeName:    runtimeName,
	}
}

// dirSizeBytes returns the total size of all files in a directory tree.
func dirSizeBytes(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

// ClearVLMData deletes VLM scores and rankings for a specific folder.
func (a *App) ClearVLMData(folderPath string) error {
	logger.Log.Info("app: clearing VLM data", "folder", folderPath)
	return a.store.DeleteVLMDataForFolder(folderPath)
}

// ClearAllVLMData deletes all VLM scores, rankings, and token usage data.
func (a *App) ClearAllVLMData() error {
	logger.Log.Info("app: clearing all VLM data")
	return a.store.ClearAllVLMData()
}

// DeleteVLMModel stops the engine, removes model files and runtime, and resets config.
func (a *App) DeleteVLMModel() error {
	logger.Log.Info("app: deleting VLM model and runtime")

	// Stop the engine first.
	if a.vlmManager != nil {
		_ = a.vlmManager.Stop(a.ctx)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}
	cullsnapDir := filepath.Join(home, ".cullsnap")
	backend, _ := a.store.GetConfig(vlm.ConfigKeyBackend)

	// Remove model files.
	modelsDir := filepath.Join(cullsnapDir, "models")
	if entries, dirErr := os.ReadDir(modelsDir); dirErr == nil {
		for _, e := range entries {
			// Only remove VLM models (GGUF/MLX), preserve ONNX models.
			name := e.Name()
			if filepath.Ext(name) == ".gguf" {
				_ = os.Remove(filepath.Join(modelsDir, name))
				logger.Log.Debug("app: removed model file", "name", name)
			}
		}
	}

	// Remove runtime.
	if backend == "mlx" {
		venvPath := vlm.MLXVenvPath(cullsnapDir)
		_ = os.RemoveAll(venvPath)
		logger.Log.Debug("app: removed MLX venv", "path", venvPath)
	} else {
		binaryPath := vlm.LlamaServerBinaryPath(cullsnapDir)
		_ = os.Remove(binaryPath)
		logger.Log.Debug("app: removed llama-server", "path", binaryPath)
	}

	// Remove MLX model directory if present.
	mlxModelsDir := filepath.Join(cullsnapDir, "mlx-models")
	_ = os.RemoveAll(mlxModelsDir)

	// Clear config.
	_ = a.store.SetConfig(vlm.ConfigKeyModelName, "")
	_ = a.store.SetConfig(vlm.ConfigKeyModelVariant, "")
	_ = a.store.SetConfig(vlm.ConfigKeyBackend, "")
	_ = a.store.SetConfig(vlm.ConfigKeySetupComplete, "false")

	logger.Log.Info("app: VLM model and runtime deleted")
	return nil
}

// GetStaleVLMStatus checks if any folders have outdated VLM scores against
// the current prompt version OR the current custom-instructions hash.
func (a *App) GetStaleVLMStatus() VLMStaleStatus {
	currentHash := vlm.HashCustomInstructions(a.GetVLMCustomInstructions())
	folders, err := a.store.GetStaleVLMFolders(vlm.PromptVersion, currentHash)
	if err != nil {
		logger.Log.Warn("app: failed to check stale VLM folders", "error", err)
		return VLMStaleStatus{CurrentPrompt: vlm.PromptVersion}
	}
	return VLMStaleStatus{
		Stale:         len(folders) > 0,
		StaleFolders:  folders,
		CurrentPrompt: vlm.PromptVersion,
	}
}

// GetTokenUsageSummary returns aggregated VLM token usage statistics.
func (a *App) GetTokenUsageSummary() ([]storage.TokenUsageSummary, error) {
	return a.store.GetTokenUsageSummary()
}

// GetVLMCustomInstructions returns the persisted custom instructions for the
// VLM system prompt. Empty string when none have been set.
func (a *App) GetVLMCustomInstructions() string {
	v, _ := a.store.GetConfig(vlm.ConfigKeyCustomInstructions)
	return v
}

// SetVLMCustomInstructions sanitizes the raw user input, persists it to the
// config KV, and pushes the cleaned value into the manager so subsequent
// inferences pick it up immediately. Returns the sanitized value so the UI can
// reflect any rejected/truncated content. The next analysis run will detect a
// hash mismatch on cached scores and re-score affected photos.
func (a *App) SetVLMCustomInstructions(raw string) (string, error) {
	sanitized := vlm.SanitizeCustomInstructions(raw)
	if err := a.store.SetConfig(vlm.ConfigKeyCustomInstructions, sanitized); err != nil {
		logger.Log.Error("app: failed to persist VLM custom instructions", "error", err)
		return sanitized, fmt.Errorf("persist custom instructions: %w", err)
	}
	if a.vlmManager != nil {
		a.vlmManager.SetCustomInstructions(sanitized)
	}
	logger.Log.Info("app: VLM custom instructions updated",
		"length", len(sanitized),
		"truncated", len([]rune(raw)) > vlm.MaxCustomInstructionsLen,
	)
	return sanitized, nil
}
