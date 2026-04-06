//go:build windows

package scoring

import (
	"context"
	"fmt"
	"image"
)

const (
	scrfdModelName   = "scrfd_2_5g"
	scrfdModelURL    = "" // placeholder — will be populated in Task 21
	scrfdModelSHA256 = "" // placeholder — will be populated in Task 21
	scrfdModelFile   = "scrfd_2_5g.onnx"
	scrfdModelSizeMB = 3 * 1024 * 1024
)

// FaceDetectorPlugin is a stub on Windows where onnxruntime-purego is not supported.
// ONNX Runtime uses dlopen (Unix) which has no Windows equivalent in purego.
// Windows users can use the CloudProvider instead.
type FaceDetectorPlugin struct {
	modelManager *ModelManager
}

// Name returns the plugin identifier.
func (p *FaceDetectorPlugin) Name() string { return "face-detector" }

// Category returns CategoryDetection.
func (p *FaceDetectorPlugin) Category() PluginCategory { return CategoryDetection }

// Models returns the SCRFD model spec (for UI display purposes).
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

// Available always returns false on Windows.
func (p *FaceDetectorPlugin) Available() bool { return false }

// Init always returns an error on Windows.
func (p *FaceDetectorPlugin) Init(_ string) error {
	return fmt.Errorf("face-detector: ONNX runtime not supported on Windows (use cloud provider)")
}

// Close is a no-op on Windows.
func (p *FaceDetectorPlugin) Close() error { return nil }

// Process always returns an error on Windows.
func (p *FaceDetectorPlugin) Process(_ context.Context, _ image.Image) (PluginResult, error) {
	return PluginResult{}, fmt.Errorf("face-detector: local ONNX not available on Windows")
}
