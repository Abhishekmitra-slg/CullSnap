package scoring

import (
	"cullsnap/internal/logger"
	"fmt"
)

// NewFaceDetectorPlugin creates a FaceDetectorPlugin backed by a ModelManager
// rooted at cacheDir. Returns an error only if the model directory cannot be
// created; the plugin may still be unavailable until Init is called.
func NewFaceDetectorPlugin(cacheDir string) (*FaceDetectorPlugin, error) {
	mm, err := NewModelManager(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("face-detector: model manager: %w", err)
	}
	p := &FaceDetectorPlugin{modelManager: mm}
	mm.RegisterAll(p.Models())
	logger.Log.Debug("scoring: created face detector plugin", "cacheDir", cacheDir)
	return p, nil
}

// NewFaceEmbedderPlugin creates a FaceEmbedderPlugin backed by a ModelManager
// rooted at cacheDir.
func NewFaceEmbedderPlugin(cacheDir string) (*FaceEmbedderPlugin, error) {
	mm, err := NewModelManager(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("face-embedder: model manager: %w", err)
	}
	p := &FaceEmbedderPlugin{modelManager: mm}
	mm.RegisterAll(p.Models())
	logger.Log.Debug("scoring: created face embedder plugin", "cacheDir", cacheDir)
	return p, nil
}

// NewAestheticPlugin creates an AestheticPlugin backed by a ModelManager
// rooted at cacheDir.
func NewAestheticPlugin(cacheDir string) (*AestheticPlugin, error) {
	mm, err := NewModelManager(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("aesthetic: model manager: %w", err)
	}
	p := &AestheticPlugin{modelManager: mm}
	mm.RegisterAll(p.Models())
	logger.Log.Debug("scoring: created aesthetic plugin", "cacheDir", cacheDir)
	return p, nil
}
