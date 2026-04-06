package scoring

import (
	"context"
	"cullsnap/internal/logger"
	"image"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// Pipeline orchestrates the multi-stage AI scoring pipeline for a single image.
// Stage 1 runs Detection and Quality plugins concurrently; Stage 2 runs
// Recognition plugins sequentially on detected faces; Stage 3 merges results.
type Pipeline struct {
	registry *Registry
	weights  ScoreWeights
	mu       sync.RWMutex
}

// NewPipeline returns a Pipeline backed by registry with default weights.
func NewPipeline(registry *Registry) *Pipeline {
	return &Pipeline{
		registry: registry,
		weights:  DefaultWeights(),
	}
}

// SetWeights replaces the current blending weights.
func (p *Pipeline) SetWeights(w ScoreWeights) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.weights = w
	logger.Log.Debug("scoring: pipeline: weights updated",
		"aesthetic", w.Aesthetic,
		"sharpness", w.Sharpness,
		"face", w.Face,
		"eyes", w.Eyes,
	)
}

// Weights returns a snapshot of the current blending weights.
func (p *Pipeline) Weights() ScoreWeights {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.weights
}

// Execute runs the full three-stage scoring pipeline on img.
//
// Stage 1 (parallel): Detection and Quality plugins run concurrently via
// errgroup. A plugin failure is non-fatal — it is logged and execution
// continues. Context cancellation stops the pipeline immediately.
//
// Stage 2 (sequential): If at least one face was detected, Recognition plugins
// (those implementing RecognitionPlugin) are called with ProcessRegions.
//
// Stage 3: Results are merged into a CompositeScore.
func (p *Pipeline) Execute(ctx context.Context, img image.Image) (*CompositeScore, error) {
	logger.Log.Debug("scoring: pipeline: execute start")

	detectors := p.registry.GetByCategory(CategoryDetection)
	qualityPlugins := p.registry.GetByCategory(CategoryQuality)
	recognitionPlugins := p.registry.GetByCategory(CategoryRecognition)

	logger.Log.Debug("scoring: pipeline: plugins found",
		"detectors", len(detectors),
		"quality", len(qualityPlugins),
		"recognition", len(recognitionPlugins),
	)

	// ── Stage 1 ─────────────────────────────────────────────────────────────
	// Run Detection and Quality plugins concurrently. Non-fatal failures are
	// logged and the plugin's contribution is simply omitted.

	var (
		facesMu        sync.Mutex
		faces          []FaceRegion
		qualMu         sync.Mutex
		qualityResults []PluginResult
	)

	eg, egCtx := errgroup.WithContext(ctx)

	// Detection plugins.
	for _, det := range detectors {
		if !det.Available() {
			logger.Log.Debug("scoring: pipeline: skipping unavailable detector", "name", det.Name())
			continue
		}
		det := det // capture
		eg.Go(func() error {
			result, err := det.Process(egCtx, img)
			if err != nil {
				if egCtx.Err() != nil {
					return egCtx.Err()
				}
				logger.Log.Warn("scoring: pipeline: detector failed, continuing",
					"name", det.Name(),
					"error", err,
				)
				return nil
			}
			logger.Log.Debug("scoring: pipeline: detector result",
				"name", det.Name(),
				"face_count", len(result.Faces),
			)
			facesMu.Lock()
			faces = append(faces, result.Faces...)
			facesMu.Unlock()
			return nil
		})
	}

	// Quality plugins.
	for _, q := range qualityPlugins {
		if !q.Available() {
			logger.Log.Debug("scoring: pipeline: skipping unavailable quality plugin", "name", q.Name())
			continue
		}
		q := q // capture
		eg.Go(func() error {
			result, err := q.Process(egCtx, img)
			if err != nil {
				if egCtx.Err() != nil {
					return egCtx.Err()
				}
				logger.Log.Warn("scoring: pipeline: quality plugin failed, continuing",
					"name", q.Name(),
					"error", err,
				)
				return nil
			}
			logger.Log.Debug("scoring: pipeline: quality result",
				"name", q.Name(),
				"has_quality", result.Quality != nil,
			)
			qualMu.Lock()
			qualityResults = append(qualityResults, result)
			qualMu.Unlock()
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		// Only context cancellation propagates out.
		logger.Log.Debug("scoring: pipeline: stage 1 cancelled", "error", err)
		return nil, err
	}

	// Check outer context after stage 1.
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// ── Stage 2 ─────────────────────────────────────────────────────────────
	// If any faces were detected, run Recognition plugins sequentially.

	var embeddings []FaceEmbedding

	if len(faces) > 0 {
		for _, rp := range recognitionPlugins {
			if !rp.Available() {
				logger.Log.Debug("scoring: pipeline: skipping unavailable recognition plugin", "name", rp.Name())
				continue
			}
			rec, ok := rp.(RecognitionPlugin)
			if !ok {
				logger.Log.Debug("scoring: pipeline: plugin does not implement RecognitionPlugin, skipping", "name", rp.Name())
				continue
			}

			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			result, err := rec.ProcessRegions(ctx, img, faces)
			if err != nil {
				logger.Log.Warn("scoring: pipeline: recognition plugin failed, continuing",
					"name", rp.Name(),
					"error", err,
				)
				continue
			}
			logger.Log.Debug("scoring: pipeline: recognition result",
				"name", rp.Name(),
				"embedding_count", len(result.Embeddings),
			)
			embeddings = append(embeddings, result.Embeddings...)
		}
	}

	// ── Stage 3 ─────────────────────────────────────────────────────────────
	cs := p.mergeResults(faces, embeddings, qualityResults, img)

	logger.Log.Debug("scoring: pipeline: execute complete",
		"face_count", cs.FaceCount,
		"aesthetic", cs.AestheticScore,
		"sharpness", cs.SharpnessScore,
		"best_face_sharp", cs.BestFaceSharp,
		"eye_openness", cs.EyeOpenness,
	)

	return cs, nil
}

// mergeResults combines detection, recognition, and quality outputs into a
// single CompositeScore. It is free of I/O and always succeeds.
func (p *Pipeline) mergeResults(
	faces []FaceRegion,
	embeddings []FaceEmbedding,
	qualityResults []PluginResult,
	img image.Image,
) *CompositeScore {
	cs := &CompositeScore{
		FaceCount: len(faces),
		Faces:     faces,
		ScoredAt:  time.Now(),
		Provider:  "local-pipeline",
	}

	// Flatten embeddings from FaceEmbedding slice into [][]float32.
	if len(embeddings) > 0 {
		embVecs := make([][]float32, len(embeddings))
		for i, e := range embeddings {
			embVecs[i] = e.Embedding
		}
		cs.Embeddings = embVecs
	}

	// Extract named quality scores.
	for _, qr := range qualityResults {
		if qr.Quality == nil {
			continue
		}
		switch qr.Quality.Name {
		case "aesthetic":
			cs.AestheticScore = qr.Quality.Score
			logger.Log.Debug("scoring: pipeline: merge: aesthetic score", "score", cs.AestheticScore)
		case "sharpness":
			cs.SharpnessScore = qr.Quality.Score
			logger.Log.Debug("scoring: pipeline: merge: sharpness score", "score", cs.SharpnessScore)
		default:
			logger.Log.Debug("scoring: pipeline: merge: unknown quality metric", "name", qr.Quality.Name)
		}
	}

	// Compute per-face eye-region sharpness; track the best face.
	if len(faces) > 0 {
		gray := toGray(img)
		bestIdx := 0
		bestSharp := -1.0

		for i, face := range faces {
			rawVar := EyeSharpnessFromFace(gray, face)
			sharp := NormalizeLaplacian(rawVar)
			logger.Log.Debug("scoring: pipeline: face sharpness",
				"face_index", i,
				"raw_variance", rawVar,
				"normalized", sharp,
			)
			if sharp > bestSharp {
				bestSharp = sharp
				bestIdx = i
			}
		}

		cs.BestFaceSharp = bestSharp
		cs.BestFaceIdx = bestIdx

		// Eye openness for the best face.
		cs.EyeOpenness = estimateEyeOpenness(faces[bestIdx])
		logger.Log.Debug("scoring: pipeline: best face",
			"index", bestIdx,
			"sharp", bestSharp,
			"eye_openness", cs.EyeOpenness,
		)
	}

	return cs
}

// estimateEyeOpenness estimates how open the eyes are based on landmark
// positions. The result is in [0, 1] where 1 means fully open.
//
// When no landmarks are available (all zero), 0.5 is returned as a neutral
// default so the score is not penalised for missing data.
func estimateEyeOpenness(face FaceRegion) float64 {
	lm := face.Landmarks

	// Check whether any landmark is non-zero.
	hasLandmarks := false
	for _, pt := range lm {
		if pt[0] != 0 || pt[1] != 0 {
			hasLandmarks = true
			break
		}
	}
	if !hasLandmarks {
		return 0.5
	}

	// Landmark layout (BlazeFace / SCRFD 5-point):
	//   [0] left eye, [1] right eye, [2] nose tip, [3] left mouth, [4] right mouth
	// All values are normalised to [0, 1] relative to the face bounding box.
	leftEyeY := float64(lm[0][1])
	rightEyeY := float64(lm[1][1])
	noseY := float64(lm[2][1])

	avgEyeY := (leftEyeY + rightEyeY) / 2.0

	bb := face.BoundingBox
	faceHeight := float64(bb.Dy())
	if faceHeight == 0 {
		// Fall back to landmark-based height estimation.
		// Use nose-to-eye ratio within the [0,1] normalised space.
		ratio := noseY - avgEyeY
		// Normalise [0.15, 0.35] → [0, 1].
		openness := (ratio - 0.15) / (0.35 - 0.15)
		return clamp01(openness)
	}

	// When bounding box is available, compute ratio in pixel space using
	// normalised Y coordinates scaled by face height.
	eyeYPx := avgEyeY * faceHeight
	noseYPx := noseY * faceHeight

	ratio := (noseYPx - eyeYPx) / faceHeight
	// Normalise [0.15, 0.35] → [0, 1].
	openness := (ratio - 0.15) / (0.35 - 0.15)
	return clamp01(openness)
}
