//go:build windows

package scoring

import (
	"context"
	"fmt"
	"image"
)

const (
	aestheticModelName   = "nima_aesthetic"
	aestheticModelURL    = "" // placeholder — will be populated in Task 21
	aestheticModelSHA256 = "" // placeholder — will be populated in Task 21
	aestheticModelFile   = "nima_aesthetic.onnx"
	aestheticInputSize   = 224
	aestheticModelSizeMB = 10 * 1024 * 1024
)

// AestheticPlugin is a stub on Windows where onnxruntime-purego is not supported.
// ONNX Runtime uses dlopen (Unix) which has no Windows equivalent in purego.
// Windows users can use the CloudProvider instead.
type AestheticPlugin struct {
	modelManager *ModelManager
}

// Name returns the plugin identifier.
func (p *AestheticPlugin) Name() string { return "aesthetic" }

// Category returns CategoryQuality.
func (p *AestheticPlugin) Category() PluginCategory { return CategoryQuality }

// Models returns the NIMA model spec (for UI display purposes).
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

// Available always returns false on Windows.
func (p *AestheticPlugin) Available() bool { return false }

// Init always returns an error on Windows.
func (p *AestheticPlugin) Init(_ string) error {
	return fmt.Errorf("aesthetic: ONNX runtime not supported on Windows (use cloud provider)")
}

// Close is a no-op on Windows.
func (p *AestheticPlugin) Close() error { return nil }

// Process always returns an error on Windows.
func (p *AestheticPlugin) Process(_ context.Context, _ image.Image) (PluginResult, error) {
	return PluginResult{}, fmt.Errorf("aesthetic: local ONNX not available on Windows")
}
