package vlm

import (
	"testing"
)

func TestRegistryLookup(t *testing.T) {
	m, ok := LookupModel("gemma-4-e4b-it", "llamacpp")
	if !ok {
		t.Fatal("expected to find gemma-4-e4b-it/llamacpp but got false")
	}
	if m.Format != "gguf" {
		t.Errorf("expected format=gguf, got %q", m.Format)
	}
	if m.SizeBytes <= 0 {
		t.Errorf("expected SizeBytes>0, got %d", m.SizeBytes)
	}
	if m.SHA256 == "" {
		t.Error("expected non-empty SHA256")
	}
}

func TestRegistryLookupMLX(t *testing.T) {
	m, ok := LookupModel("gemma-4-e4b-it", "mlx")
	if !ok {
		t.Fatal("expected to find gemma-4-e4b-it/mlx but got false")
	}
	if m.Format != "mlx" {
		t.Errorf("expected format=mlx, got %q", m.Format)
	}
}

func TestRegistryLookupE2B(t *testing.T) {
	m, ok := LookupModel("gemma-4-e2b-it", "llamacpp")
	if !ok {
		t.Fatal("expected to find gemma-4-e2b-it/llamacpp but got false")
	}
	const twoGB = 2 * 1024 * 1024 * 1024
	if m.SizeBytes >= twoGB {
		t.Errorf("expected SizeBytes < 2GB, got %d", m.SizeBytes)
	}
}

func TestRegistryLookupUnknown(t *testing.T) {
	_, ok := LookupModel("nonexistent", "llamacpp")
	if ok {
		t.Error("expected false for unknown model but got true")
	}
}

func TestModelsForTier(t *testing.T) {
	// TierLegacy must return zero models.
	legacy := ModelsForTier(TierLegacy)
	if len(legacy) != 0 {
		t.Errorf("expected 0 models for TierLegacy, got %d", len(legacy))
	}

	// TierBasic must return at least one model, and every returned model must have MinTier <= TierBasic.
	basic := ModelsForTier(TierBasic)
	if len(basic) == 0 {
		t.Error("expected at least 1 model for TierBasic, got 0")
	}
	for _, m := range basic {
		if m.MinTier > TierBasic {
			t.Errorf("model %s/%s has MinTier %v > TierBasic", m.Name, m.Backend, m.MinTier)
		}
	}
}

func TestAllModels(t *testing.T) {
	models := AllModels()
	if len(models) == 0 {
		t.Fatal("AllModels() returned no models, expected at least one")
	}
	// Verify it returns a copy: mutating the result must not affect subsequent calls.
	models[0].Name = "MUTATED"
	fresh := AllModels()
	if fresh[0].Name == "MUTATED" {
		t.Error("AllModels() did not return a copy — mutation leaked")
	}
}
