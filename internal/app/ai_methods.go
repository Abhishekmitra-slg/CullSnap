package app

import (
	"context"
	"cullsnap/internal/logger"
	"cullsnap/internal/model"
	"cullsnap/internal/scanner"
	"cullsnap/internal/scoring"
	"cullsnap/internal/storage"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	stdruntime "runtime"
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
			Aesthetic: dw.Aesthetic,
			Sharpness: dw.Sharpness,
			Face:      dw.Face,
			Eyes:      dw.Eyes,
		}
	}
	w := a.pipeline.Weights()
	return AIWeightsConfig{
		Aesthetic: w.Aesthetic,
		Sharpness: w.Sharpness,
		Face:      w.Face,
		Eyes:      w.Eyes,
	}
}

// SetAIWeights updates the score blending weights and persists them.
func (a *App) SetAIWeights(weights AIWeightsConfig) error {
	logger.Log.Info("app: setting AI weights",
		"aesthetic", weights.Aesthetic,
		"sharpness", weights.Sharpness,
		"face", weights.Face,
		"eyes", weights.Eyes,
	)
	sw := scoring.ScoreWeights{
		Aesthetic: weights.Aesthetic,
		Sharpness: weights.Sharpness,
		Face:      weights.Face,
		Eyes:      weights.Eyes,
	}.Normalize()

	if a.pipeline != nil {
		a.pipeline.SetWeights(sw)
	}

	// Persist individual weights.
	_ = a.store.SetConfig("ai_weight_aesthetic", strconv.FormatFloat(sw.Aesthetic, 'f', 4, 64))
	_ = a.store.SetConfig("ai_weight_sharpness", strconv.FormatFloat(sw.Sharpness, 'f', 4, 64))
	_ = a.store.SetConfig("ai_weight_face", strconv.FormatFloat(sw.Face, 'f', 4, 64))
	_ = a.store.SetConfig("ai_weight_eyes", strconv.FormatFloat(sw.Eyes, 'f', 4, 64))
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
	w = w.Normalize()
	a.pipeline.SetWeights(w)
	logger.Log.Debug("app: loaded AI weights",
		"aesthetic", w.Aesthetic,
		"sharpness", w.Sharpness,
		"face", w.Face,
		"eyes", w.Eyes,
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
