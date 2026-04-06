package scoring

import (
	"context"
	"crypto/sha256"
	"cullsnap/internal/logger"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	modelDownloadTimeout = 5 * time.Minute
	modelDirPerm         = 0o700
	modelFilePerm        = 0o600
)

// ModelManager handles downloading and caching ONNX models in ~/.cullsnap/models/.
type ModelManager struct {
	modelsDir string
	specs     map[string]ModelSpec
}

// NewModelManager creates a model manager that stores models in cacheDir/models/.
func NewModelManager(cacheDir string) (*ModelManager, error) {
	modelsDir := filepath.Join(cacheDir, "models")
	if err := os.MkdirAll(modelsDir, modelDirPerm); err != nil {
		return nil, fmt.Errorf("create models directory: %w", err)
	}

	logger.Log.Debug("scoring: model manager initialized", "dir", modelsDir)

	return &ModelManager{
		modelsDir: modelsDir,
		specs:     make(map[string]ModelSpec),
	}, nil
}

// Register adds a model specification to the manager.
func (m *ModelManager) Register(spec ModelSpec) {
	m.specs[spec.Name] = spec
	logger.Log.Debug("scoring: registered model spec",
		"name", spec.Name,
		"filename", spec.Filename,
	)
}

// IsDownloaded reports whether a model file exists on disk.
func (m *ModelManager) IsDownloaded(name string) bool {
	spec, ok := m.specs[name]
	if !ok {
		return false
	}
	_, err := os.Stat(filepath.Join(m.modelsDir, spec.Filename))
	return err == nil
}

// ModelPath returns the full path to a downloaded model, or empty string if unknown.
func (m *ModelManager) ModelPath(name string) string {
	spec, ok := m.specs[name]
	if !ok {
		return ""
	}
	return filepath.Join(m.modelsDir, spec.Filename)
}

// Download fetches a model from its URL and verifies the SHA256 hash.
// The model is written to a temp file first, then renamed on success.
func (m *ModelManager) Download(ctx context.Context, name string) error {
	spec, ok := m.specs[name]
	if !ok {
		return fmt.Errorf("unknown model: %s", name)
	}

	destPath := filepath.Join(m.modelsDir, spec.Filename)

	logger.Log.Info("scoring: downloading model",
		"name", name,
		"url", spec.URL,
		"dest", destPath,
	)

	dlCtx, cancel := context.WithTimeout(ctx, modelDownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, spec.URL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download model %s: %w", name, err)
	}
	defer resp.Body.Close() //nolint:errcheck // HTTP response body close

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download model %s: HTTP %d", name, resp.StatusCode)
	}

	// Write to temp file in the same directory (atomic rename).
	tmpFile, err := os.CreateTemp(m.modelsDir, spec.Filename+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()    //nolint:errcheck,gosec // cleanup best-effort
		os.Remove(tmpPath) //nolint:errcheck // cleanup best-effort
	}()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return fmt.Errorf("write model %s: %w", name, err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Verify hash.
	if err := m.verifyHash(tmpPath, spec.SHA256); err != nil {
		return fmt.Errorf("model %s hash mismatch: %w", name, err)
	}

	// Atomic rename.
	if err := os.Chmod(tmpPath, modelFilePerm); err != nil {
		return fmt.Errorf("chmod model file: %w", err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("rename model file: %w", err)
	}

	logger.Log.Info("scoring: model downloaded and verified",
		"name", name,
		"path", destPath,
	)

	return nil
}

// RegisterAll registers multiple model specs in one call.
func (m *ModelManager) RegisterAll(specs []ModelSpec) {
	for _, spec := range specs {
		m.Register(spec)
	}
	logger.Log.Debug("scoring: registered all model specs", "count", len(specs))
}

// AllDownloaded returns true if every registered model exists on disk.
// Returns false when no models are registered.
func (m *ModelManager) AllDownloaded() bool {
	if len(m.specs) == 0 {
		return false
	}
	for name := range m.specs {
		if !m.IsDownloaded(name) {
			return false
		}
	}
	return true
}

// DownloadAll downloads every registered model that is not already present on
// disk. progressFn is called for each chunk of data written; it may be nil.
func (m *ModelManager) DownloadAll(ctx context.Context, progressFn func(name string, downloaded, total int64)) error {
	for name, spec := range m.specs {
		if m.IsDownloaded(name) {
			logger.Log.Debug("scoring: model already present, skipping", "name", name)
			continue
		}

		logger.Log.Info("scoring: downloading model via DownloadAll", "name", name)

		if progressFn != nil {
			progressFn(name, 0, spec.Size)
		}

		if err := m.Download(ctx, name); err != nil {
			return fmt.Errorf("download model %s: %w", name, err)
		}

		if progressFn != nil {
			progressFn(name, spec.Size, spec.Size)
		}
	}
	return nil
}

// RegisteredModels returns a snapshot of all registered model specs.
func (m *ModelManager) RegisteredModels() []ModelSpec {
	out := make([]ModelSpec, 0, len(m.specs))
	for _, spec := range m.specs {
		out = append(out, spec)
	}
	return out
}

// verifyHash checks that a file's SHA256 matches the expected hex string.
func (m *ModelManager) verifyHash(path, expectedHash string) error {
	f, err := os.Open(path) //nolint:gosec // trusted internal path
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // read-only file close

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expectedHash {
		return fmt.Errorf("expected %s, got %s", expectedHash, actual)
	}

	return nil
}
