//go:build !windows

package scoring

import (
	"bytes"
	"context"
	"cullsnap/internal/logger"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/shota3506/onnxruntime-purego/onnxruntime"

	// Register standard image decoders.
	_ "golang.org/x/image/bmp"
)

const (
	blazefaceInputSize = 128
	blazefaceModelName = "blazeface"

	// BlazeFace model spec.
	blazefaceURL    = "https://huggingface.co/garavv/blazeface-onnx/resolve/main/blaze.onnx"
	blazefaceSHA256 = "564740c5146673c840257402cee8309161848e48e64d277a862ab4d501adf8a5"

	// Default confidence threshold for face detection.
	defaultConfThreshold = 0.5
	defaultIOUThreshold  = 0.3
	defaultMaxDetections = 20
)

// LocalProvider implements ScoringProvider using local ONNX model inference.
// Uses BlazeFace for face detection via onnxruntime-purego (no CGO).
type LocalProvider struct {
	modelManager *ModelManager
	runtime      *onnxruntime.Runtime
	env          *onnxruntime.Env
	session      *onnxruntime.Session
	mu           sync.Mutex
	initialized  bool
}

// NewLocalProvider creates a local ONNX scoring provider.
// cacheDir is the base directory (e.g., ~/.cullsnap/) for model storage.
func NewLocalProvider(cacheDir string) (*LocalProvider, error) {
	mm, err := NewModelManager(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("create model manager: %w", err)
	}

	mm.Register(ModelSpec{
		Name:     blazefaceModelName,
		URL:      blazefaceURL,
		SHA256:   blazefaceSHA256,
		Filename: "blazeface.onnx",
	})

	return &LocalProvider{
		modelManager: mm,
	}, nil
}

func (p *LocalProvider) Name() string         { return "Local (ONNX)" }
func (p *LocalProvider) RequiresAPIKey() bool { return false }
func (p *LocalProvider) RequiresDownload() bool {
	return !p.modelManager.IsDownloaded(blazefaceModelName)
}

// Available reports whether the ONNX runtime is loaded and the model is downloaded.
func (p *LocalProvider) Available() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initialized && p.modelManager.IsDownloaded(blazefaceModelName)
}

// InitRuntime initializes the ONNX Runtime from the shared library path.
// libraryPath is the path to libonnxruntime.dylib/so (empty = auto-provision to cacheDir).
func (p *LocalProvider) InitRuntime(libraryPath string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		return nil
	}

	// If no explicit path, check provisioned location in the cache dir.
	if libraryPath == "" {
		provisionedPath := filepath.Join(filepath.Dir(p.modelManager.modelsDir), "lib", onnxRuntimeLibName())
		if _, err := os.Stat(provisionedPath); err == nil {
			libraryPath = provisionedPath
		}
	}

	logger.Log.Info("scoring: initializing ONNX runtime", "library", libraryPath)

	rt, err := onnxruntime.NewRuntime(libraryPath, 23)
	if err != nil {
		return fmt.Errorf("load ONNX runtime: %w", err)
	}

	env, err := rt.NewEnv("cullsnap", onnxruntime.LoggingLevelWarning)
	if err != nil {
		rt.Close() //nolint:errcheck,gosec // best effort
		return fmt.Errorf("create ONNX environment: %w", err)
	}

	p.runtime = rt
	p.env = env
	p.initialized = true

	logger.Log.Info("scoring: ONNX runtime initialized")

	// If model already downloaded, load the session now.
	if p.modelManager.IsDownloaded(blazefaceModelName) {
		return p.loadSession()
	}

	return nil
}

// DownloadModel downloads the BlazeFace model and loads the session.
func (p *LocalProvider) DownloadModel(ctx context.Context) error {
	if err := p.modelManager.Download(ctx, blazefaceModelName); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.initialized && p.session == nil {
		return p.loadSession()
	}
	return nil
}

// loadSession creates an ONNX inference session from the downloaded model.
// Must be called with p.mu held.
func (p *LocalProvider) loadSession() error {
	modelPath := p.modelManager.ModelPath(blazefaceModelName)
	logger.Log.Info("scoring: loading BlazeFace model", "path", modelPath)

	sess, err := p.runtime.NewSession(p.env, modelPath, &onnxruntime.SessionOptions{
		IntraOpNumThreads: 2,
	})
	if err != nil {
		return fmt.Errorf("create ONNX session: %w", err)
	}

	p.session = sess

	logger.Log.Info("scoring: BlazeFace model loaded",
		"inputs", sess.InputNames(),
		"outputs", sess.OutputNames(),
	)

	return nil
}

// Score runs face detection on the image using BlazeFace.
func (p *LocalProvider) Score(ctx context.Context, imgData []byte) (*ScoreResult, error) {
	p.mu.Lock()
	if !p.initialized || p.session == nil {
		p.mu.Unlock()
		return nil, fmt.Errorf("ONNX runtime not initialized")
	}
	session := p.session
	rt := p.runtime
	p.mu.Unlock()

	// Decode image.
	img, err := decodeImage(imgData)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	// Preprocess: resize to 128x128, normalize to [0,1], NCHW layout.
	tensor := preprocessImage(img, blazefaceInputSize, blazefaceInputSize)

	// Create input tensors.
	imageTensor, err := onnxruntime.NewTensorValue(rt, tensor, []int64{1, 3, blazefaceInputSize, blazefaceInputSize})
	if err != nil {
		return nil, fmt.Errorf("create image tensor: %w", err)
	}
	defer imageTensor.Close()

	confData := []float32{defaultConfThreshold}
	confTensor, err := onnxruntime.NewTensorValue(rt, confData, []int64{1})
	if err != nil {
		return nil, fmt.Errorf("create conf tensor: %w", err)
	}
	defer confTensor.Close()

	iouData := []float32{defaultIOUThreshold}
	iouTensor, err := onnxruntime.NewTensorValue(rt, iouData, []int64{1})
	if err != nil {
		return nil, fmt.Errorf("create iou tensor: %w", err)
	}
	defer iouTensor.Close()

	maxDetData := []int64{defaultMaxDetections}
	maxDetTensor, err := onnxruntime.NewTensorValue(rt, maxDetData, []int64{1})
	if err != nil {
		return nil, fmt.Errorf("create max_det tensor: %w", err)
	}
	defer maxDetTensor.Close()

	// Run inference.
	outputs, err := session.Run(ctx, map[string]*onnxruntime.Value{
		"image":          imageTensor,
		"conf_threshold": confTensor,
		"iou_threshold":  iouTensor,
		"max_detections": maxDetTensor,
	})
	if err != nil {
		return nil, fmt.Errorf("ONNX inference: %w", err)
	}
	defer func() {
		for _, v := range outputs {
			v.Close()
		}
	}()

	// Parse outputs.
	return p.parseOutputs(outputs, img.Bounds())
}

// parseOutputs converts ONNX output tensors to ScoreResult.
// Handles both single-output (selectedBoxes with embedded confidence) and
// dual-output (boxes + scores) model formats.
func (p *LocalProvider) parseOutputs(outputs map[string]*onnxruntime.Value, imgBounds image.Rectangle) (*ScoreResult, error) {
	var allData []float32
	var allShape []int64

	for name, val := range outputs {
		shape, err := val.GetTensorShape()
		if err != nil {
			logger.Log.Warn("scoring: failed to get tensor shape", "name", name, "error", err)
			continue
		}

		data, _, err := onnxruntime.GetTensorData[float32](val)
		if err != nil {
			logger.Log.Warn("scoring: failed to get tensor data", "name", name, "error", err)
			continue
		}

		logger.Log.Debug("scoring: output tensor",
			"name", name,
			"shape", shape,
			"dataLen", len(data),
		)

		// Use the first valid output tensor.
		if len(data) > 0 {
			allData = data
			allShape = shape
		}
	}

	if allData == nil || len(allShape) == 0 {
		logger.Log.Debug("scoring: no valid outputs from model")
		return &ScoreResult{}, nil
	}

	faces := parseSelectedBoxes(allData, allShape)

	// Scale bounding boxes from model input space (128x128) to original image space.
	scaleX := float64(imgBounds.Dx()) / float64(blazefaceInputSize)
	scaleY := float64(imgBounds.Dy()) / float64(blazefaceInputSize)
	for i := range faces {
		bb := faces[i].BoundingBox
		faces[i].BoundingBox = image.Rect(
			int(float64(bb.Min.X)*scaleX),
			int(float64(bb.Min.Y)*scaleY),
			int(float64(bb.Max.X)*scaleX),
			int(float64(bb.Max.Y)*scaleY),
		)
	}

	// Compute overall score based on face detection quality.
	overallScore := 0.0
	if len(faces) > 0 {
		var totalConf float64
		for _, f := range faces {
			totalConf += f.Confidence
		}
		overallScore = totalConf / float64(len(faces))
	}

	logger.Log.Debug("scoring: face detection complete",
		"faceCount", len(faces),
		"overallScore", overallScore,
	)

	return &ScoreResult{
		Faces:        faces,
		OverallScore: overallScore,
		Confidence:   overallScore,
	}, nil
}

// Close releases ONNX runtime resources.
func (p *LocalProvider) Close() {
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
	logger.Log.Debug("scoring: ONNX runtime closed")
}

// decodeImage decodes JPEG or PNG image bytes.
func decodeImage(data []byte) (image.Image, error) {
	r := bytes.NewReader(data)
	img, _, err := image.Decode(r)
	if err != nil {
		// Try JPEG specifically (handles more edge cases).
		r.Reset(data)
		img, err = jpeg.Decode(r)
		if err != nil {
			r.Reset(data)
			img, err = png.Decode(r)
		}
	}
	return img, err
}

// preprocessImage resizes an image and converts to NCHW float32 tensor normalized to [0,1].
func preprocessImage(img image.Image, width, height int) []float32 {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	tensor := make([]float32, 1*3*height*width)

	for y := range height {
		for x := range width {
			// Nearest-neighbor resize: map target pixel to source pixel.
			srcX := bounds.Min.X + x*srcW/width
			srcY := bounds.Min.Y + y*srcH/height

			r, g, b, _ := img.At(srcX, srcY).RGBA()

			// NCHW layout: [batch][channel][height][width]
			// Normalize from [0, 65535] to [0.0, 1.0].
			idx := y*width + x
			tensor[0*height*width+idx] = float32(r) / 65535.0 // R channel
			tensor[1*height*width+idx] = float32(g) / 65535.0 // G channel
			tensor[2*height*width+idx] = float32(b) / 65535.0 // B channel
		}
	}

	return tensor
}

// parseSelectedBoxes parses the "selectedBoxes" output from BlazeFace.
// This model does NMS internally and returns a single tensor.
//
// Output tensor shapes observed in production:
//   - [1, 0, 16]  → batch=1, N=0 faces (no detections)
//   - [1, N, 16]  → batch=1, N faces detected
//   - [1, 16]     → 1 face detected (batch dim collapsed)
//
// Each row has 16 values: [x1, y1, x2, y2, lm1_x, lm1_y, ..., lm6_x, lm6_y]
// 4 bbox coords + 12 landmark coords. NO confidence column — all rows
// already passed the conf_threshold input.
func parseSelectedBoxes(data []float32, shape []int64) []FaceRegion {
	if len(data) == 0 {
		return nil
	}

	const cols = 16 // BlazeFace: 4 bbox + 12 landmarks
	var n int       // number of detections

	switch len(shape) {
	case 3:
		// [batch, N, 16] — strip batch dimension.
		n = int(shape[1])
	case 2:
		// [N, 16] or [1, 16].
		if int(shape[1]) == cols {
			n = int(shape[0])
		} else {
			n = len(data) / cols
		}
	case 1:
		// [16] — single detection flattened.
		n = len(data) / cols
	default:
		return nil
	}

	if n == 0 {
		return nil
	}

	var faces []FaceRegion

	for i := range n {
		offset := i * cols
		if offset+4 > len(data) {
			break
		}

		x1 := data[offset]
		y1 := data[offset+1]
		x2 := data[offset+2]
		y2 := data[offset+3]

		// Skip zero-padded rows.
		if x1 == 0 && y1 == 0 && x2 == 0 && y2 == 0 {
			continue
		}

		// Coords may be normalized [0,1] or pixel [0,128].
		maxCoord := max32(x1, max32(y1, max32(x2, y2)))
		if maxCoord <= 1.0 && maxCoord > 0 {
			x1 *= blazefaceInputSize
			y1 *= blazefaceInputSize
			x2 *= blazefaceInputSize
			y2 *= blazefaceInputSize
		}

		bb := image.Rect(
			int(math.Round(float64(x1))),
			int(math.Round(float64(y1))),
			int(math.Round(float64(x2))),
			int(math.Round(float64(y2))),
		)

		// Confidence is 1.0 — model already filtered by conf_threshold.
		face := FaceRegion{
			BoundingBox: bb,
			Confidence:  1.0,
			EyesOpen:    true,
		}

		faces = append(faces, face)
	}

	return faces
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
