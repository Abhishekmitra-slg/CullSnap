//go:build !windows

package scoring

import (
	"context"
	"cullsnap/internal/logger"
	"fmt"
	"image"
	"math"
	"sync"

	"github.com/shota3506/onnxruntime-purego/onnxruntime"
)

const (
	arcfaceModelName   = "arcface_mobilefacenet"
	arcfaceModelURL    = "" // placeholder — will be populated in Task 21
	arcfaceModelSHA256 = "" // placeholder — will be populated in Task 21
	arcfaceModelFile   = "arcface_mobilefacenet.onnx"
	arcfaceInputSize   = 112
	arcfaceEmbedDim    = 512

	// arcfaceModelSizeMB is the approximate model size in bytes (~4 MB).
	arcfaceModelSizeMB = 4 * 1024 * 1024
)

// FaceEmbedderPlugin is a ScoringPlugin and RecognitionPlugin that runs ArcFace
// MobileFaceNet via ONNX to produce 512-dimensional L2-normalized face embeddings.
// It implements the RecognitionPlugin interface: use ProcessRegions for embedding;
// Process returns an error directing callers to use ProcessRegions instead.
type FaceEmbedderPlugin struct {
	modelManager *ModelManager
	runtime      *onnxruntime.Runtime
	env          *onnxruntime.Env
	session      *onnxruntime.Session
	mu           sync.Mutex
	initialized  bool
}

// Name returns the plugin identifier.
func (p *FaceEmbedderPlugin) Name() string { return "face-embedder" }

// Category returns CategoryRecognition.
func (p *FaceEmbedderPlugin) Category() PluginCategory { return CategoryRecognition }

// Models returns the single ArcFace model spec required by this plugin.
func (p *FaceEmbedderPlugin) Models() []ModelSpec {
	return []ModelSpec{
		{
			Name:     arcfaceModelName,
			Filename: arcfaceModelFile,
			URL:      arcfaceModelURL,
			SHA256:   arcfaceModelSHA256,
			Size:     arcfaceModelSizeMB,
		},
	}
}

// Available reports whether the plugin is initialized and the model is downloaded.
func (p *FaceEmbedderPlugin) Available() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initialized && p.modelManager != nil && p.modelManager.IsDownloaded(arcfaceModelName)
}

// Init loads the ONNX runtime from libPath and registers the ArcFace model.
// If the model is already downloaded, the session is loaded immediately.
func (p *FaceEmbedderPlugin) Init(libPath string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		logger.Log.Debug("scoring: face-embedder already initialized")
		return nil
	}

	logger.Log.Info("scoring: face-embedder init", "libPath", libPath)

	rt, err := onnxruntime.NewRuntime(libPath, 23)
	if err != nil {
		return fmt.Errorf("face-embedder: load ONNX runtime: %w", err)
	}

	env, err := rt.NewEnv("cullsnap-arcface", onnxruntime.LoggingLevelWarning)
	if err != nil {
		rt.Close() //nolint:errcheck,gosec // best effort
		return fmt.Errorf("face-embedder: create ONNX environment: %w", err)
	}

	p.runtime = rt
	p.env = env
	p.initialized = true

	logger.Log.Info("scoring: face-embedder ONNX runtime initialized")

	// Load session immediately if the model is already on disk.
	if p.modelManager != nil && p.modelManager.IsDownloaded(arcfaceModelName) {
		return p.loadSessionLocked()
	}

	return nil
}

// loadSessionLocked creates an ONNX inference session. Must be called with p.mu held.
func (p *FaceEmbedderPlugin) loadSessionLocked() error {
	modelPath := p.modelManager.ModelPath(arcfaceModelName)
	logger.Log.Info("scoring: face-embedder loading ArcFace model", "path", modelPath)

	sess, err := p.runtime.NewSession(p.env, modelPath, &onnxruntime.SessionOptions{
		IntraOpNumThreads: 2,
	})
	if err != nil {
		return fmt.Errorf("face-embedder: create ONNX session: %w", err)
	}

	p.session = sess

	logger.Log.Info("scoring: face-embedder ArcFace model loaded",
		"inputs", sess.InputNames(),
		"outputs", sess.OutputNames(),
	)

	return nil
}

// Close releases ONNX runtime resources.
func (p *FaceEmbedderPlugin) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.session != nil {
		p.session.Close()
		p.session = nil
	}
	if p.env != nil {
		p.env.Close()
		p.env = nil
	}
	if p.runtime != nil {
		p.runtime.Close() //nolint:errcheck,gosec // best effort cleanup
		p.runtime = nil
	}
	p.initialized = false

	logger.Log.Debug("scoring: face-embedder ONNX runtime closed")
	return nil
}

// Process always returns an error directing the caller to use ProcessRegions.
// ArcFace operates on pre-cropped face regions, not full images.
func (p *FaceEmbedderPlugin) Process(_ context.Context, _ image.Image) (PluginResult, error) {
	return PluginResult{}, fmt.Errorf("face-embedder: use ProcessRegions to embed detected faces")
}

// ProcessRegions iterates over each detected face in faces, crops and aligns it
// from img, runs ArcFace inference, and returns L2-normalized 512-dimensional
// embeddings in PluginResult.Embeddings.
func (p *FaceEmbedderPlugin) ProcessRegions(ctx context.Context, img image.Image, faces []FaceRegion) (PluginResult, error) {
	p.mu.Lock()
	if !p.initialized || p.session == nil {
		p.mu.Unlock()
		return PluginResult{}, fmt.Errorf("face-embedder: not initialized or model not loaded")
	}
	session := p.session
	rt := p.runtime
	p.mu.Unlock()

	logger.Log.Debug("scoring: face-embedder processing regions",
		"faceCount", len(faces),
	)

	embeddings := make([]FaceEmbedding, 0, len(faces))

	for i, face := range faces {
		// Check for context cancellation between faces.
		select {
		case <-ctx.Done():
			return PluginResult{}, ctx.Err()
		default:
		}

		crop := cropAndAlignFace(img, face, arcfaceInputSize)
		inputData := preprocessForArcFace(crop, arcfaceInputSize)

		inputTensor, err := onnxruntime.NewTensorValue(rt, inputData, []int64{1, 3, arcfaceInputSize, arcfaceInputSize})
		if err != nil {
			logger.Log.Warn("scoring: face-embedder create tensor failed", "faceIdx", i, "error", err)
			continue
		}

		inputName := "input"
		if names := session.InputNames(); len(names) > 0 {
			inputName = names[0]
		}

		outputs, err := session.Run(ctx, map[string]*onnxruntime.Value{
			inputName: inputTensor,
		})
		inputTensor.Close()
		if err != nil {
			logger.Log.Warn("scoring: face-embedder ONNX inference failed", "faceIdx", i, "error", err)
			continue
		}

		var embedding []float32
		for _, v := range outputs {
			data, _, err := onnxruntime.GetTensorData[float32](v)
			v.Close()
			if err != nil {
				logger.Log.Warn("scoring: face-embedder get tensor data failed", "faceIdx", i, "error", err)
				continue
			}
			if len(data) == arcfaceEmbedDim {
				embedding = l2Normalize(data)
				break
			}
			// Accept any output with sufficient elements and take first arcfaceEmbedDim.
			if len(data) >= arcfaceEmbedDim {
				embedding = l2Normalize(data[:arcfaceEmbedDim])
				break
			}
		}

		if embedding == nil {
			logger.Log.Warn("scoring: face-embedder no valid embedding output", "faceIdx", i)
			continue
		}

		embeddings = append(embeddings, FaceEmbedding{
			DetectionID: int64(i),
			Embedding:   embedding,
		})

		logger.Log.Debug("scoring: face-embedder embedded face",
			"faceIdx", i,
			"embedDim", len(embedding),
		)
	}

	logger.Log.Debug("scoring: face-embedder complete",
		"inputFaces", len(faces),
		"embeddedFaces", len(embeddings),
	)

	return PluginResult{Embeddings: embeddings}, nil
}

// cropAndAlignFace crops the face region from img with 20% padding on each side
// and resizes it to targetSize×targetSize using nearest-neighbor interpolation.
func cropAndAlignFace(img image.Image, face FaceRegion, targetSize int) image.Image {
	bounds := img.Bounds()
	bb := face.BoundingBox

	w := bb.Dx()
	h := bb.Dy()

	padX := int(math.Round(float64(w) * 0.20))
	padY := int(math.Round(float64(h) * 0.20))

	x1 := max(bb.Min.X-padX, bounds.Min.X)
	y1 := max(bb.Min.Y-padY, bounds.Min.Y)
	x2 := min(bb.Max.X+padX, bounds.Max.X)
	y2 := min(bb.Max.Y+padY, bounds.Max.Y)

	// Clamp to at least 1×1 to avoid zero-size crops.
	if x2 <= x1 {
		x2 = x1 + 1
		if x2 > bounds.Max.X {
			x1 = bounds.Max.X - 1
			x2 = bounds.Max.X
		}
	}
	if y2 <= y1 {
		y2 = y1 + 1
		if y2 > bounds.Max.Y {
			y1 = bounds.Max.Y - 1
			y2 = bounds.Max.Y
		}
	}

	srcW := x2 - x1
	srcH := y2 - y1

	out := image.NewRGBA(image.Rect(0, 0, targetSize, targetSize))
	for dy := range targetSize {
		for dx := range targetSize {
			srcX := x1 + dx*srcW/targetSize
			srcY := y1 + dy*srcH/targetSize
			out.Set(dx, dy, img.At(srcX, srcY))
		}
	}

	return out
}

// preprocessForArcFace resizes img to targetSize×targetSize using nearest-neighbor
// and returns a flat NCHW float32 tensor normalized to [-1,1].
func preprocessForArcFace(img image.Image, targetSize int) []float32 {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	tensor := make([]float32, 3*targetSize*targetSize)

	for y := range targetSize {
		for x := range targetSize {
			// Nearest-neighbor resize mapping.
			srcX := bounds.Min.X + x*srcW/targetSize
			srcY := bounds.Min.Y + y*srcH/targetSize

			r, g, b, _ := img.At(srcX, srcY).RGBA()

			idx := y*targetSize + x
			// Normalize from [0, 65535] to [-1.0, 1.0].
			tensor[0*targetSize*targetSize+idx] = float32(r)/32767.5 - 1.0
			tensor[1*targetSize*targetSize+idx] = float32(g)/32767.5 - 1.0
			tensor[2*targetSize*targetSize+idx] = float32(b)/32767.5 - 1.0
		}
	}

	return tensor
}

// l2Normalize returns a unit-length copy of v. If the L2 norm is zero, v is
// returned unchanged to avoid division by zero.
func l2Normalize(v []float32) []float32 {
	var sumSq float64
	for _, x := range v {
		sumSq += float64(x) * float64(x)
	}

	norm := math.Sqrt(sumSq)
	if norm == 0 {
		out := make([]float32, len(v))
		copy(out, v)
		return out
	}

	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) / norm)
	}
	return out
}
