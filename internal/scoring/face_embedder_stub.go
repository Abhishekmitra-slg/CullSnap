//go:build windows

package scoring

import (
	"context"
	"fmt"
	"image"
)

const (
	arcfaceModelName   = "arcface_mobilefacenet"
	arcfaceModelURL    = "" // placeholder — will be populated in Task 21
	arcfaceModelSHA256 = "" // placeholder — will be populated in Task 21
	arcfaceModelFile   = "arcface_mobilefacenet.onnx"
	arcfaceInputSize   = 112
	arcfaceEmbedDim    = 512
	arcfaceModelSizeMB = 4 * 1024 * 1024
)

// FaceEmbedderPlugin is a stub on Windows where onnxruntime-purego is not supported.
// ONNX Runtime uses dlopen (Unix) which has no Windows equivalent in purego.
// Windows users can use the CloudProvider instead.
type FaceEmbedderPlugin struct {
	modelManager *ModelManager
}

// Name returns the plugin identifier.
func (p *FaceEmbedderPlugin) Name() string { return "face-embedder" }

// Category returns CategoryRecognition.
func (p *FaceEmbedderPlugin) Category() PluginCategory { return CategoryRecognition }

// Models returns the ArcFace model spec (for UI display purposes).
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

// Available always returns false on Windows.
func (p *FaceEmbedderPlugin) Available() bool { return false }

// Init always returns an error on Windows.
func (p *FaceEmbedderPlugin) Init(_ string) error {
	return fmt.Errorf("face-embedder: ONNX runtime not supported on Windows (use cloud provider)")
}

// Close is a no-op on Windows.
func (p *FaceEmbedderPlugin) Close() error { return nil }

// Process always returns an error on Windows.
func (p *FaceEmbedderPlugin) Process(_ context.Context, _ image.Image) (PluginResult, error) {
	return PluginResult{}, fmt.Errorf("face-embedder: local ONNX not available on Windows")
}

// ProcessRegions always returns an error on Windows.
func (p *FaceEmbedderPlugin) ProcessRegions(_ context.Context, _ image.Image, _ []FaceRegion) (PluginResult, error) {
	return PluginResult{}, fmt.Errorf("face-embedder: local ONNX not available on Windows")
}
