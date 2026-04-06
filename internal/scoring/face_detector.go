//go:build !windows

package scoring

import (
	"context"
	"cullsnap/internal/logger"
	"fmt"
	"image"
	"math"
	"sort"
	"sync"

	"github.com/shota3506/onnxruntime-purego/onnxruntime"
)

const (
	scrfdModelName = "scrfd_2_5g"
	// scrfdModelURL is the direct HuggingFace resolve URL for scrfd_2.5g.onnx
	// hosted in the JackCui/facefusion repository (3.29 MB, stable URL).
	// Input:  "input.1" — [1, 3, 640, 640] float32
	// Outputs: score_8/16/32 — confidence per anchor per stride
	//          bbox_8/16/32  — bbox deltas per anchor per stride
	scrfdModelURL    = "https://huggingface.co/JackCui/facefusion/resolve/main/scrfd_2.5g.onnx"
	scrfdModelSHA256 = "bc24bb349491481c3ca793cf89306723162c280cb284c5a5e49df3760bf5c2ce"
	scrfdModelFile   = "scrfd_2_5g.onnx"
	scrfdInputSize   = 640
	scrfdConfThresh  = 0.5
	scrfdIOUThresh   = 0.4

	// scrfdModelSize is the exact model file size in bytes (3.29 MB).
	scrfdModelSizeMB = 3_450_109
)

// scrfdTensor holds the extracted data from a single ONNX output tensor.
type scrfdTensor struct {
	name  string
	shape []int64
	data  []float32
}

// FaceDetectorPlugin is a ScoringPlugin that runs SCRFD face detection via ONNX.
// SCRFD (Sample and Computation Redistribution for Efficient Face Detection) uses
// a multi-stride feature pyramid to detect faces at multiple scales in a single pass.
type FaceDetectorPlugin struct {
	modelManager *ModelManager
	runtime      *onnxruntime.Runtime
	env          *onnxruntime.Env
	session      *onnxruntime.Session
	mu           sync.Mutex
	initialized  bool
}

// Name returns the plugin identifier.
func (p *FaceDetectorPlugin) Name() string { return "face-detector" }

// Category returns CategoryDetection.
func (p *FaceDetectorPlugin) Category() PluginCategory { return CategoryDetection }

// Models returns the single SCRFD model spec required by this plugin.
func (p *FaceDetectorPlugin) Models() []ModelSpec {
	return []ModelSpec{
		{
			Name:     scrfdModelName,
			Filename: scrfdModelFile,
			URL:      scrfdModelURL,
			SHA256:   scrfdModelSHA256,
			Size:     scrfdModelSizeMB,
		},
	}
}

// Available reports whether the plugin is initialized and the model is downloaded.
func (p *FaceDetectorPlugin) Available() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initialized && p.modelManager != nil && p.modelManager.IsDownloaded(scrfdModelName)
}

// Init loads the ONNX runtime from libPath and registers the SCRFD model.
// If the model is already downloaded, the session is loaded immediately.
func (p *FaceDetectorPlugin) Init(libPath string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		logger.Log.Debug("scoring: face-detector already initialized")
		return nil
	}

	logger.Log.Info("scoring: face-detector init", "libPath", libPath)

	rt, err := onnxruntime.NewRuntime(libPath, 23)
	if err != nil {
		return fmt.Errorf("face-detector: load ONNX runtime: %w", err)
	}

	env, err := rt.NewEnv("cullsnap-scrfd", onnxruntime.LoggingLevelWarning)
	if err != nil {
		rt.Close() //nolint:errcheck,gosec // best effort
		return fmt.Errorf("face-detector: create ONNX environment: %w", err)
	}

	p.runtime = rt
	p.env = env
	p.initialized = true

	logger.Log.Info("scoring: face-detector ONNX runtime initialized")

	// Load session immediately if the model is already on disk.
	if p.modelManager != nil && p.modelManager.IsDownloaded(scrfdModelName) {
		return p.loadSessionLocked()
	}

	return nil
}

// loadSessionLocked creates an ONNX inference session. Must be called with p.mu held.
func (p *FaceDetectorPlugin) loadSessionLocked() error {
	modelPath := p.modelManager.ModelPath(scrfdModelName)
	logger.Log.Info("scoring: face-detector loading SCRFD model", "path", modelPath)

	sess, err := p.runtime.NewSession(p.env, modelPath, &onnxruntime.SessionOptions{
		IntraOpNumThreads: 2,
	})
	if err != nil {
		return fmt.Errorf("face-detector: create ONNX session: %w", err)
	}

	p.session = sess

	logger.Log.Info("scoring: face-detector SCRFD model loaded",
		"inputs", sess.InputNames(),
		"outputs", sess.OutputNames(),
	)

	return nil
}

// Close releases ONNX runtime resources.
func (p *FaceDetectorPlugin) Close() error {
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

	logger.Log.Debug("scoring: face-detector ONNX runtime closed")
	return nil
}

// Process runs SCRFD face detection on img and returns detected FaceRegions.
func (p *FaceDetectorPlugin) Process(ctx context.Context, img image.Image) (PluginResult, error) {
	p.mu.Lock()
	if !p.initialized || p.session == nil {
		p.mu.Unlock()
		return PluginResult{}, fmt.Errorf("face-detector: not initialized or model not loaded")
	}
	session := p.session
	rt := p.runtime
	p.mu.Unlock()

	imgW := img.Bounds().Dx()
	imgH := img.Bounds().Dy()

	logger.Log.Debug("scoring: face-detector processing image",
		"width", imgW,
		"height", imgH,
	)

	// Preprocess: resize to 640x640, NCHW, normalize to [0,1].
	inputData := preprocessForSCRFD(img, scrfdInputSize)

	inputTensor, err := onnxruntime.NewTensorValue(rt, inputData, []int64{1, 3, scrfdInputSize, scrfdInputSize})
	if err != nil {
		return PluginResult{}, fmt.Errorf("face-detector: create input tensor: %w", err)
	}
	defer inputTensor.Close()

	// SCRFD typically uses a single input named "input" or "images".
	inputName := "input"
	if names := session.InputNames(); len(names) > 0 {
		inputName = names[0]
	}

	outputs, err := session.Run(ctx, map[string]*onnxruntime.Value{
		inputName: inputTensor,
	})
	if err != nil {
		return PluginResult{}, fmt.Errorf("face-detector: ONNX inference: %w", err)
	}
	defer func() {
		for _, v := range outputs {
			v.Close()
		}
	}()

	faces := parseSCRFDOutputs(outputs, imgW, imgH, scrfdConfThresh)
	faces = nms(faces, scrfdIOUThresh)

	logger.Log.Debug("scoring: face-detector detection complete",
		"faceCount", len(faces),
	)

	return PluginResult{Faces: faces}, nil
}

// preprocessForSCRFD resizes img to targetSize×targetSize using nearest-neighbor
// and returns a flat NCHW float32 tensor with mean=[127.5,127.5,127.5]
// std=[128,128,128] normalization, mapping [0,255] to [-1,1].
func preprocessForSCRFD(img image.Image, targetSize int) []float32 {
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
			// Convert from [0, 65535] to [0, 255], then apply (x - 127.5) / 128.0.
			tensor[0*targetSize*targetSize+idx] = (float32(r)/257.0 - 127.5) / 128.0
			tensor[1*targetSize*targetSize+idx] = (float32(g)/257.0 - 127.5) / 128.0
			tensor[2*targetSize*targetSize+idx] = (float32(b)/257.0 - 127.5) / 128.0
		}
	}

	return tensor
}

// parseSCRFDOutputs parses multi-stride SCRFD output tensors into FaceRegions.
//
// SCRFD uses a feature pyramid with strides [8, 16, 32]. For each stride the
// network produces two output tensors:
//   - scores:  [1, H*W*num_anchors, 1] — objectness confidence per anchor
//   - boxes:   [1, H*W*num_anchors, 4] — bbox deltas (cx_offset, cy_offset, w, h)
//
// Optionally a landmark tensor may also be present:
//   - kps:     [1, H*W*num_anchors, 10] — 5 landmarks (x, y) per face
//
// When the output format is unknown the function falls back to a generic parse
// that treats any large enough tensor as a flat list of [x1,y1,x2,y2,conf] rows.
func parseSCRFDOutputs(outputs map[string]*onnxruntime.Value, imgW, imgH int, confThresh float32) []FaceRegion {
	if len(outputs) == 0 {
		return nil
	}

	tensors := make([]scrfdTensor, 0, len(outputs))
	for name, val := range outputs {
		shape, err := val.GetTensorShape()
		if err != nil {
			logger.Log.Warn("scoring: face-detector failed to get tensor shape", "name", name, "error", err)
			continue
		}
		data, _, err := onnxruntime.GetTensorData[float32](val)
		if err != nil {
			logger.Log.Warn("scoring: face-detector failed to get tensor data", "name", name, "error", err)
			continue
		}
		logger.Log.Debug("scoring: face-detector output tensor",
			"name", name,
			"shape", shape,
			"dataLen", len(data),
		)
		tensors = append(tensors, scrfdTensor{name: name, shape: shape, data: data})
	}

	if len(tensors) == 0 {
		return nil
	}

	// Try named multi-stride parsing first (score/box pairs per stride).
	faces := parseSCRFDStridedOutputs(tensors, imgW, imgH, confThresh)
	if faces != nil {
		return faces
	}

	// Generic fallback: look for the largest tensor and parse as [N, 5+] rows
	// with layout [x1, y1, x2, y2, conf, ...].
	return parseSCRFDGeneric(tensors, imgW, imgH, confThresh)
}

// parseSCRFDStridedOutputs handles the canonical SCRFD output format where
// separate score and box tensors are emitted for each of the 3 strides.
func parseSCRFDStridedOutputs(tensors []scrfdTensor, imgW, imgH int, confThresh float32) []FaceRegion {
	strides := []int{8, 16, 32}
	numAnchors := 2 // SCRFD 2.5G uses 2 anchors per location

	var faces []FaceRegion
	found := false

	for _, stride := range strides {
		featH := scrfdInputSize / stride
		featW := scrfdInputSize / stride
		numLocs := featH * featW * numAnchors

		// Find matching score tensor (shape [1, numLocs, 1] or [1, numLocs]).
		var scoreData []float32
		var boxData []float32
		var kpsData []float32

		for _, t := range tensors {
			n := tensorNumElements(t.shape)
			if n == 0 {
				continue
			}
			switch {
			case n == numLocs && matchesScoreShape(t.shape, numLocs):
				scoreData = t.data
			case n == numLocs*4 && matchesBoxShape(t.shape, numLocs):
				boxData = t.data
			case n == numLocs*10 && matchesKpsShape(t.shape, numLocs):
				kpsData = t.data
			}
		}

		if scoreData == nil || boxData == nil {
			continue
		}

		found = true

		// Generate anchor centers for this stride.
		anchors := generateSCRFDAnchors(featH, featW, stride, numAnchors)

		scaleX := float64(imgW) / float64(scrfdInputSize)
		scaleY := float64(imgH) / float64(scrfdInputSize)

		for i, anchor := range anchors {
			if i >= len(scoreData) {
				break
			}
			conf := scoreData[i]
			if conf < confThresh {
				continue
			}

			// SCRFD distance-based bbox: [left, top, right, bottom] distances
			// from anchor center to each edge, each multiplied by stride.
			boxOff := i * 4
			if boxOff+4 > len(boxData) {
				break
			}
			x1 := anchor[0] - float64(boxData[boxOff+0])*float64(stride)
			y1 := anchor[1] - float64(boxData[boxOff+1])*float64(stride)
			x2 := anchor[0] + float64(boxData[boxOff+2])*float64(stride)
			y2 := anchor[1] + float64(boxData[boxOff+3])*float64(stride)

			// Scale back to original image coordinates.
			x1 *= scaleX
			y1 *= scaleY
			x2 *= scaleX
			y2 *= scaleY

			face := FaceRegion{
				BoundingBox: image.Rect(
					int(math.Round(x1)),
					int(math.Round(y1)),
					int(math.Round(x2)),
					int(math.Round(y2)),
				),
				Confidence: float64(conf),
			}

			// Parse 5 landmarks if available.
			if kpsData != nil {
				kpsOff := i * 10
				if kpsOff+10 <= len(kpsData) {
					for k := range 5 {
						lx := float64(kpsData[kpsOff+k*2]) * scaleX
						ly := float64(kpsData[kpsOff+k*2+1]) * scaleY
						// Clamp to [0,1] normalised.
						face.Landmarks[k][0] = float32(lx / float64(imgW))
						face.Landmarks[k][1] = float32(ly / float64(imgH))
					}
				}
			}

			faces = append(faces, face)
		}
	}

	if !found {
		return nil
	}
	return faces
}

// parseSCRFDGeneric is a fallback parser for unknown SCRFD output layouts.
// It searches for the largest float32 tensor and treats it as [N, cols] where
// cols >= 5 and layout is [x1, y1, x2, y2, conf, ...].
func parseSCRFDGeneric(tensors []scrfdTensor, imgW, imgH int, confThresh float32) []FaceRegion {
	// Find the tensor with the most elements.
	best := -1
	bestN := 0
	for i, t := range tensors {
		if len(t.data) > bestN {
			bestN = len(t.data)
			best = i
		}
	}
	if best < 0 {
		return nil
	}

	t := tensors[best]
	const minCols = 5
	cols := minCols

	// Infer columns from shape.
	if len(t.shape) >= 2 {
		cols = int(t.shape[len(t.shape)-1])
		if cols < minCols {
			cols = minCols
		}
	}

	n := len(t.data) / cols
	if n == 0 {
		return nil
	}

	scaleX := float64(imgW) / float64(scrfdInputSize)
	scaleY := float64(imgH) / float64(scrfdInputSize)

	var faces []FaceRegion

	for i := range n {
		off := i * cols
		if off+5 > len(t.data) {
			break
		}
		x1 := float64(t.data[off])
		y1 := float64(t.data[off+1])
		x2 := float64(t.data[off+2])
		y2 := float64(t.data[off+3])
		conf := t.data[off+4]

		if conf < confThresh {
			continue
		}
		if x1 == 0 && y1 == 0 && x2 == 0 && y2 == 0 {
			continue
		}

		// If coords look normalized ([0,1]), scale to input size first.
		maxCoord := math.Max(math.Max(x1, y1), math.Max(x2, y2))
		if maxCoord <= 1.0 && maxCoord > 0 {
			x1 *= float64(scrfdInputSize)
			y1 *= float64(scrfdInputSize)
			x2 *= float64(scrfdInputSize)
			y2 *= float64(scrfdInputSize)
		}

		faces = append(faces, FaceRegion{
			BoundingBox: image.Rect(
				int(math.Round(x1*scaleX)),
				int(math.Round(y1*scaleY)),
				int(math.Round(x2*scaleX)),
				int(math.Round(y2*scaleY)),
			),
			Confidence: float64(conf),
		})
	}

	return faces
}

// generateSCRFDAnchors returns anchor center points (cx, cy) in input-image pixel
// coordinates for a feature map of size featH×featW with the given stride and
// number of anchors per location.
func generateSCRFDAnchors(featH, featW, stride, numAnchors int) [][2]float64 {
	anchors := make([][2]float64, 0, featH*featW*numAnchors)
	for y := range featH {
		for x := range featW {
			cx := (float64(x) + 0.5) * float64(stride)
			cy := (float64(y) + 0.5) * float64(stride)
			for range numAnchors {
				anchors = append(anchors, [2]float64{cx, cy})
			}
		}
	}
	return anchors
}

// nms applies non-maximum suppression to faces, keeping the highest-confidence
// detection and removing overlapping detections above iouThresh.
func nms(faces []FaceRegion, iouThresh float64) []FaceRegion {
	if len(faces) == 0 {
		return faces
	}

	// Sort descending by confidence.
	sort.Slice(faces, func(i, j int) bool {
		return faces[i].Confidence > faces[j].Confidence
	})

	suppressed := make([]bool, len(faces))
	result := make([]FaceRegion, 0, len(faces))

	for i := range faces {
		if suppressed[i] {
			continue
		}
		result = append(result, faces[i])
		// Suppress all lower-confidence detections that overlap significantly.
		for j := i + 1; j < len(faces); j++ {
			if suppressed[j] {
				continue
			}
			if iou(faces[i].BoundingBox, faces[j].BoundingBox) > iouThresh {
				suppressed[j] = true
			}
		}
	}

	return result
}

// iou computes the intersection-over-union of two rectangles.
func iou(a, b image.Rectangle) float64 {
	inter := a.Intersect(b)
	interArea := float64(inter.Dx() * inter.Dy())
	if interArea <= 0 {
		return 0
	}

	aArea := float64(a.Dx() * a.Dy())
	bArea := float64(b.Dx() * b.Dy())
	union := aArea + bArea - interArea
	if union <= 0 {
		return 0
	}

	return interArea / union
}

// tensorNumElements returns the total number of elements described by shape.
func tensorNumElements(shape []int64) int {
	if len(shape) == 0 {
		return 0
	}
	n := 1
	for _, d := range shape {
		n *= int(d)
	}
	return n
}

func matchesScoreShape(shape []int64, numLocs int) bool {
	n := tensorNumElements(shape)
	return n == numLocs
}

func matchesBoxShape(shape []int64, numLocs int) bool {
	n := tensorNumElements(shape)
	return n == numLocs*4
}

func matchesKpsShape(shape []int64, numLocs int) bool {
	n := tensorNumElements(shape)
	return n == numLocs*10
}
