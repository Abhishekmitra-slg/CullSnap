package scoring

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestModelManager_ModelsDir(t *testing.T) {
	tmp := t.TempDir()
	mm, err := NewModelManager(tmp)
	if err != nil {
		t.Fatalf("NewModelManager: %v", err)
	}

	info, err := os.Stat(mm.modelsDir)
	if err != nil {
		t.Fatalf("models dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("models dir is not a directory")
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("models dir perm = %o, want 700", info.Mode().Perm())
	}
}

func TestModelManager_IsDownloaded(t *testing.T) {
	tmp := t.TempDir()
	mm, err := NewModelManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	mm.Register(ModelSpec{
		Name:     "test-model",
		URL:      "https://example.com/model.onnx",
		SHA256:   "abc123",
		Filename: "model.onnx",
	})

	if mm.IsDownloaded("test-model") {
		t.Error("should not be downloaded before file exists")
	}

	// Create the model file.
	modelPath := filepath.Join(mm.modelsDir, "model.onnx")
	if err := os.WriteFile(modelPath, []byte("fake model data"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !mm.IsDownloaded("test-model") {
		t.Error("should be downloaded after file exists")
	}
}

func TestModelManager_IsDownloaded_UnknownModel(t *testing.T) {
	tmp := t.TempDir()
	mm, err := NewModelManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	if mm.IsDownloaded("nonexistent") {
		t.Error("unknown model should not be downloaded")
	}
}

func TestModelManager_ModelPath(t *testing.T) {
	tmp := t.TempDir()
	mm, err := NewModelManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	mm.Register(ModelSpec{
		Name:     "blazeface",
		URL:      "https://example.com/blaze.onnx",
		SHA256:   "abc",
		Filename: "blaze.onnx",
	})

	got := mm.ModelPath("blazeface")
	want := filepath.Join(mm.modelsDir, "blaze.onnx")
	if got != want {
		t.Errorf("ModelPath = %q, want %q", got, want)
	}

	if mm.ModelPath("nonexistent") != "" {
		t.Error("unknown model should return empty path")
	}
}

func TestModelManager_VerifyHash(t *testing.T) {
	tmp := t.TempDir()
	mm, err := NewModelManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("test model content")
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	path := filepath.Join(tmp, "test.onnx")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Correct hash should pass.
	if err := mm.verifyHash(path, hash); err != nil {
		t.Errorf("correct hash failed: %v", err)
	}

	// Wrong hash should fail.
	if err := mm.verifyHash(path, "wrong-hash"); err == nil {
		t.Error("wrong hash should fail")
	}
}

func TestModelManager_Download_BadURL(t *testing.T) {
	tmp := t.TempDir()
	mm, err := NewModelManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	mm.Register(ModelSpec{
		Name:     "bad-model",
		URL:      "http://localhost:1/nonexistent.onnx",
		SHA256:   "abc",
		Filename: "bad.onnx",
	})

	err = mm.Download(context.Background(), "bad-model")
	if err == nil {
		t.Error("download from bad URL should fail")
	}
}

func TestModelManager_Download_UnknownModel(t *testing.T) {
	tmp := t.TempDir()
	mm, err := NewModelManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	err = mm.Download(context.Background(), "nonexistent")
	if err == nil {
		t.Error("download of unknown model should fail")
	}
}

func TestModelManager_RegisterAll(t *testing.T) {
	tmp := t.TempDir()
	mm, err := NewModelManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	specs := []ModelSpec{
		{Name: "model-a", Filename: "a.onnx", URL: "https://example.com/a.onnx", SHA256: "aaa"},
		{Name: "model-b", Filename: "b.onnx", URL: "https://example.com/b.onnx", SHA256: "bbb"},
	}
	mm.RegisterAll(specs)

	if mm.ModelPath("model-a") == "" {
		t.Error("model-a should be registered")
	}
	if mm.ModelPath("model-b") == "" {
		t.Error("model-b should be registered")
	}
}

func TestModelManager_AllDownloaded_Empty(t *testing.T) {
	tmp := t.TempDir()
	mm, err := NewModelManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	if mm.AllDownloaded() {
		t.Error("AllDownloaded should be false when no models are registered")
	}
}

func TestModelManager_AllDownloaded_SomePresent(t *testing.T) {
	tmp := t.TempDir()
	mm, err := NewModelManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	specs := []ModelSpec{
		{Name: "present", Filename: "present.onnx", URL: "https://example.com/present.onnx", SHA256: "p"},
		{Name: "missing", Filename: "missing.onnx", URL: "https://example.com/missing.onnx", SHA256: "m"},
	}
	mm.RegisterAll(specs)

	// Create only the first model file.
	presentPath := filepath.Join(mm.modelsDir, "present.onnx")
	if err := os.WriteFile(presentPath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	if mm.AllDownloaded() {
		t.Error("AllDownloaded should be false when some models are missing")
	}
}

func TestModelManager_RegisteredModels(t *testing.T) {
	tmp := t.TempDir()
	mm, err := NewModelManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	specs := []ModelSpec{
		{Name: "x", Filename: "x.onnx", URL: "https://example.com/x.onnx", SHA256: "x"},
		{Name: "y", Filename: "y.onnx", URL: "https://example.com/y.onnx", SHA256: "y"},
	}
	mm.RegisterAll(specs)

	registered := mm.RegisteredModels()
	if len(registered) != 2 {
		t.Errorf("RegisteredModels count = %d, want 2", len(registered))
	}
}
