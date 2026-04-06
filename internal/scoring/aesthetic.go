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
	aestheticModelName   = "nima_aesthetic"
	aestheticModelURL    = "" // placeholder — will be populated in Task 21
	aestheticModelSHA256 = "" // placeholder — will be populated in Task 21
	aestheticModelFile   = "nima_aesthetic.onnx"
	aestheticInputSize   = 224

	// aestheticModelSizeMB is the approximate model size in bytes (~10 MB).
	aestheticModelSizeMB = 10 * 1024 * 1024
)

// imagenet normalization constants (mean and std per channel).
var (
	imagenetMean = [3]float32{0.485, 0.456, 0.406}
	imagenetStd  = [3]float32{0.229, 0.224, 0.225}
)

// AestheticPlugin is a ScoringPlugin that runs NIMA aesthetic quality scoring via ONNX.
// NIMA (Neural Image Assessment) outputs a 10-level quality distribution whose
// weighted mean is used as the aesthetic score.
type AestheticPlugin struct {
	modelManager *ModelManager
	runtime      *onnxruntime.Runtime
	env          *onnxruntime.Env
	session      *onnxruntime.Session
	mu           sync.Mutex
	initialized  bool
}

// Name returns the plugin identifier.
func (p *AestheticPlugin) Name() string { return "aesthetic" }

// Category returns CategoryQuality.
func (p *AestheticPlugin) Category() PluginCategory { return CategoryQuality }

// Models returns the single NIMA model spec required by this plugin.
func (p *AestheticPlugin) Models() []ModelSpec {
	return []ModelSpec{
		{
			Name:     aestheticModelName,
			Filename: aestheticModelFile,
			URL:      aestheticModelURL,
			SHA256:   aestheticModelSHA256,
			Size:     aestheticModelSizeMB,
		},
	}
}

// Available reports whether the plugin is initialized and the model is downloaded.
func (p *AestheticPlugin) Available() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initialized && p.modelManager != nil && p.modelManager.IsDownloaded(aestheticModelName)
}

// Init loads the ONNX runtime from libPath and registers the NIMA model.
// If the model is already downloaded, the session is loaded immediately.
func (p *AestheticPlugin) Init(libPath string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		logger.Log.Debug("scoring: aesthetic already initialized")
		return nil
	}

	logger.Log.Info("scoring: aesthetic init", "libPath", libPath)

	rt, err := onnxruntime.NewRuntime(libPath, 23)
	if err != nil {
		return fmt.Errorf("aesthetic: load ONNX runtime: %w", err)
	}

	env, err := rt.NewEnv("cullsnap-nima", onnxruntime.LoggingLevelWarning)
	if err != nil {
		rt.Close() //nolint:errcheck,gosec // best effort
		return fmt.Errorf("aesthetic: create ONNX environment: %w", err)
	}

	p.runtime = rt
	p.env = env
	p.initialized = true

	logger.Log.Info("scoring: aesthetic ONNX runtime initialized")

	// Load session immediately if the model is already on disk.
	if p.modelManager != nil && p.modelManager.IsDownloaded(aestheticModelName) {
		return p.loadSessionLocked()
	}

	return nil
}

// loadSessionLocked creates an ONNX inference session. Must be called with p.mu held.
func (p *AestheticPlugin) loadSessionLocked() error {
	modelPath := p.modelManager.ModelPath(aestheticModelName)
	logger.Log.Info("scoring: aesthetic loading NIMA model", "path", modelPath)

	sess, err := p.runtime.NewSession(p.env, modelPath, &onnxruntime.SessionOptions{
		IntraOpNumThreads: 2,
	})
	if err != nil {
		return fmt.Errorf("aesthetic: create ONNX session: %w", err)
	}

	p.session = sess

	logger.Log.Info("scoring: aesthetic NIMA model loaded",
		"inputs", sess.InputNames(),
		"outputs", sess.OutputNames(),
	)

	return nil
}

// Close releases ONNX runtime resources.
func (p *AestheticPlugin) Close() error {
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

	logger.Log.Debug("scoring: aesthetic ONNX runtime closed")
	return nil
}

// Process runs NIMA aesthetic scoring on img and returns an aesthetic score in [0,1].
func (p *AestheticPlugin) Process(ctx context.Context, img image.Image) (PluginResult, error) {
	p.mu.Lock()
	if !p.initialized || p.session == nil {
		p.mu.Unlock()
		return PluginResult{}, fmt.Errorf("aesthetic: not initialized or model not loaded")
	}
	session := p.session
	rt := p.runtime
	p.mu.Unlock()

	imgW := img.Bounds().Dx()
	imgH := img.Bounds().Dy()

	logger.Log.Debug("scoring: aesthetic processing image",
		"width", imgW,
		"height", imgH,
	)

	// Preprocess: resize to 224×224, NCHW, ImageNet normalization.
	inputData := preprocessForNIMA(img, aestheticInputSize)

	inputTensor, err := onnxruntime.NewTensorValue(rt, inputData, []int64{1, 3, aestheticInputSize, aestheticInputSize})
	if err != nil {
		return PluginResult{}, fmt.Errorf("aesthetic: create input tensor: %w", err)
	}
	defer inputTensor.Close()

	// NIMA uses a single input named "input" or "images".
	inputName := "input"
	if names := session.InputNames(); len(names) > 0 {
		inputName = names[0]
	}

	outputs, err := session.Run(ctx, map[string]*onnxruntime.Value{
		inputName: inputTensor,
	})
	if err != nil {
		return PluginResult{}, fmt.Errorf("aesthetic: ONNX inference: %w", err)
	}
	defer func() {
		for _, v := range outputs {
			v.Close()
		}
	}()

	score := parseNIMAOutput(outputs)

	logger.Log.Debug("scoring: aesthetic scoring complete",
		"aestheticScore", score,
	)

	return PluginResult{Quality: &QualityScore{Name: "aesthetic", Score: score}}, nil
}

// preprocessForNIMA resizes img to targetSize×targetSize using nearest-neighbor
// and returns a flat NCHW float32 tensor with ImageNet normalization:
// mean=[0.485,0.456,0.406], std=[0.229,0.224,0.225].
func preprocessForNIMA(img image.Image, targetSize int) []float32 {
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
			// Normalize from [0, 65535] to [0.0, 1.0], then apply ImageNet normalization.
			rf := float32(r) / 65535.0
			gf := float32(g) / 65535.0
			bf := float32(b) / 65535.0

			tensor[0*targetSize*targetSize+idx] = (rf - imagenetMean[0]) / imagenetStd[0]
			tensor[1*targetSize*targetSize+idx] = (gf - imagenetMean[1]) / imagenetStd[1]
			tensor[2*targetSize*targetSize+idx] = (bf - imagenetMean[2]) / imagenetStd[2]
		}
	}

	return tensor
}

// parseNIMAOutput extracts and interprets NIMA's 10-level quality distribution.
// NIMA outputs 10 probabilities (possibly as raw logits) corresponding to quality
// levels 1–10. After softmax the weighted mean is computed and normalized to [0,1].
func parseNIMAOutput(outputs map[string]*onnxruntime.Value) float64 {
	for _, val := range outputs {
		data, _, err := onnxruntime.GetTensorData[float32](val)
		if err != nil {
			logger.Log.Warn("scoring: aesthetic failed to get tensor data", "error", err)
			continue
		}

		// Accept the first output with exactly 10 elements (or take the first 10).
		var logits []float32
		if len(data) >= 10 {
			logits = data[:10]
		}
		if logits == nil {
			logger.Log.Warn("scoring: aesthetic output tensor has fewer than 10 elements",
				"dataLen", len(data),
			)
			continue
		}

		probs := softmax(logits)

		// Compute weighted mean: levels are 1..10, normalize to [0,1] via (mean-1)/9.
		var weightedMean float64
		for i, p := range probs {
			level := float64(i + 1)
			weightedMean += level * float64(p)
		}

		score := (weightedMean - 1.0) / 9.0
		return clamp01(score)
	}

	logger.Log.Warn("scoring: aesthetic no valid output tensor found, returning 0")
	return 0
}

// softmax converts a slice of logits into a probability distribution.
// It is numerically stable: the maximum logit is subtracted before exponentiation.
func softmax(logits []float32) []float32 {
	if len(logits) == 0 {
		return nil
	}

	// Find max for numerical stability.
	maxVal := logits[0]
	for _, v := range logits[1:] {
		if v > maxVal {
			maxVal = v
		}
	}

	probs := make([]float32, len(logits))
	var sum float64
	for i, v := range logits {
		e := math.Exp(float64(v - maxVal))
		probs[i] = float32(e)
		sum += e
	}

	if sum == 0 {
		// Uniform fallback to avoid division by zero.
		uniform := float32(1.0 / float64(len(logits)))
		for i := range probs {
			probs[i] = uniform
		}
		return probs
	}

	for i := range probs {
		probs[i] = float32(float64(probs[i]) / sum)
	}

	return probs
}
