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
// libraryPath is the path to libonnxruntime.dylib/so/dll (empty = system search).
func (p *LocalProvider) InitRuntime(libraryPath string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		return nil
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
func (p *LocalProvider) parseOutputs(outputs map[string]*onnxruntime.Value, imgBounds image.Rectangle) (*ScoreResult, error) {
	// Find the output tensors by iterating available keys.
	var boxesData []float32
	var boxesShape []int64
	var scoresData []float32
	var scoresShape []int64

	for name, val := range outputs {
		shape, err := val.GetTensorShape()
		if err != nil {
			continue
		}

		data, _, err := onnxruntime.GetTensorData[float32](val)
		if err != nil {
			continue
		}

		// Heuristic: boxes output has shape [N, 17] (4 bbox + 12 landmarks + 1),
		// scores output has shape [N] (1D confidence scores).
		if len(shape) == 2 {
			boxesData = data
			boxesShape = shape
			logger.Log.Debug("scoring: boxes output", "name", name, "shape", shape)
		} else if len(shape) == 1 {
			scoresData = data
			scoresShape = shape
			logger.Log.Debug("scoring: scores output", "name", name, "shape", shape)
		}
	}

	if boxesData == nil || scoresData == nil {
		logger.Log.Debug("scoring: no valid outputs from model")
		return &ScoreResult{}, nil
	}

	faces := parseFaceDetections(boxesData, boxesShape, scoresData, scoresShape, defaultConfThreshold)

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

// parseFaceDetections converts raw model outputs to FaceRegion slices.
// boxes shape: [N, 17] — 4 bbox coords + 12 landmark coords + 1 padding.
// scores shape: [N] — confidence per detection.
func parseFaceDetections(boxes []float32, boxesShape []int64, scores []float32, scoresShape []int64, confThreshold float32) []FaceRegion {
	if len(scoresShape) == 0 || scoresShape[0] == 0 {
		return nil
	}

	n := int(scoresShape[0])
	stride := 17 // Default stride for BlazeFace output.
	if len(boxesShape) == 2 && boxesShape[1] > 0 {
		stride = int(boxesShape[1])
	}

	var faces []FaceRegion

	for i := range n {
		if i >= len(scores) {
			break
		}

		conf := scores[i]
		if conf < confThreshold {
			continue
		}

		offset := i * stride
		if offset+4 > len(boxes) {
			break
		}

		// Bounding box in normalized coordinates [0,1] relative to model input.
		x1 := boxes[offset]
		y1 := boxes[offset+1]
		x2 := boxes[offset+2]
		y2 := boxes[offset+3]

		// Scale to model input pixel space.
		bb := image.Rect(
			int(math.Round(float64(x1)*blazefaceInputSize)),
			int(math.Round(float64(y1)*blazefaceInputSize)),
			int(math.Round(float64(x2)*blazefaceInputSize)),
			int(math.Round(float64(y2)*blazefaceInputSize)),
		)

		face := FaceRegion{
			BoundingBox: bb,
			Confidence:  float64(conf),
		}

		// Extract eye landmarks if available (used for eye sharpness region).
		if offset+8 <= len(boxes) {
			// Landmarks: left eye (x,y), right eye (x,y).
			face.EyesOpen = true // Assume open; will be refined by sharpness scoring.
		}

		faces = append(faces, face)
	}

	return faces
}
