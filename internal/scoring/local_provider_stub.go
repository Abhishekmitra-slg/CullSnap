//go:build windows

package scoring

import (
	"context"
	"fmt"
	"sync"
)

// LocalProvider is a stub on Windows where onnxruntime-purego is not supported.
// ONNX Runtime uses dlopen (Unix) which has no Windows equivalent in purego.
// Windows users can use the CloudProvider instead.
type LocalProvider struct {
	modelManager *ModelManager
	mu           sync.Mutex
}

// NewLocalProvider creates a stub local provider on Windows.
func NewLocalProvider(cacheDir string) (*LocalProvider, error) {
	mm, err := NewModelManager(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("create model manager: %w", err)
	}
	return &LocalProvider{modelManager: mm}, nil
}

func (p *LocalProvider) Name() string           { return "Local (ONNX)" }
func (p *LocalProvider) RequiresAPIKey() bool   { return false }
func (p *LocalProvider) RequiresDownload() bool { return true }
func (p *LocalProvider) Available() bool        { return false }

// InitRuntime is a no-op on Windows.
func (p *LocalProvider) InitRuntime(_ string) error {
	return fmt.Errorf("ONNX runtime not supported on Windows (use cloud provider)")
}

// DownloadModel is a no-op on Windows.
func (p *LocalProvider) DownloadModel(_ context.Context) error {
	return fmt.Errorf("ONNX runtime not supported on Windows")
}

// Score always returns an error on Windows.
func (p *LocalProvider) Score(_ context.Context, _ []byte) (*ScoreResult, error) {
	return nil, fmt.Errorf("local ONNX scoring not available on Windows")
}

// Close is a no-op on Windows.
func (p *LocalProvider) Close() {}
