package vlm

import (
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// hexSHA256Pattern matches a 64-character lowercase hex SHA256 digest.
var hexSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

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
	// E2B should be strictly smaller than E4B but Q4_K_M of the 2B variant is
	// still several GB — just assert the ordering invariant, not an absolute bound.
	e4b, _ := LookupModel("gemma-4-e4b-it", "llamacpp")
	if m.SizeBytes <= 0 {
		t.Errorf("expected SizeBytes > 0, got %d", m.SizeBytes)
	}
	if m.SizeBytes >= e4b.SizeBytes {
		t.Errorf("expected E2B (%d) to be smaller than E4B (%d)", m.SizeBytes, e4b.SizeBytes)
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

// TestModelsForTierExcludesUnavailable asserts that ModelsForTier — which
// drives the user-facing model picker — filters out entries marked
// Available=false. Without this guard, users could select MLX models whose
// download path is not yet wired up.
func TestModelsForTierExcludesUnavailable(t *testing.T) {
	models := ModelsForTier(TierPower)
	for _, m := range models {
		if !m.Available {
			t.Errorf("ModelsForTier returned unavailable entry %s/%s — user would see a broken option",
				m.Name, m.Backend)
		}
	}
}

// TestAllModelsIncludesUnavailable asserts that diagnostic tools still see
// every entry, so that issues with unavailable backends stay discoverable.
func TestAllModelsIncludesUnavailable(t *testing.T) {
	var sawUnavailable bool
	for _, m := range AllModels() {
		if !m.Available {
			sawUnavailable = true
			break
		}
	}
	if !sawUnavailable {
		t.Error("AllModels is expected to expose at least one unavailable entry " +
			"(MLX) while its download path is under redesign")
	}
}

// TestMLXEntriesMarkedUnavailable is a regression guard: until the MLX download
// redesign lands, every MLX entry must be Available=false and must carry the
// documented sentinel hash so the release CI test recognises it as a tracked stub.
func TestMLXEntriesMarkedUnavailable(t *testing.T) {
	for _, m := range AllModels() {
		if m.Backend != "mlx" {
			continue
		}
		if m.Available {
			t.Errorf("MLX entry %s/%s is marked Available=true but the MLX download "+
				"pipeline is not yet implemented — re-enable only when shippable",
				m.Name, m.Backend)
		}
		if m.SHA256 != UnreleasedMLXSentinel {
			t.Errorf("MLX entry %s/%s has SHA256=%q, want sentinel %q",
				m.Name, m.Backend, m.SHA256, UnreleasedMLXSentinel)
		}
	}
}

// TestNoPlaceholderHashes is the release-blocker CI gate from issue #113:
// refuse to ship if any registry entry still carries a "PLACEHOLDER_" prefix.
// MLX's UNRELEASED_ sentinel is explicitly distinct from PLACEHOLDER_ so that
// tracked, Available=false stubs stay permissible while forgotten placeholder
// literals never do.
func TestNoPlaceholderHashes(t *testing.T) {
	for _, m := range AllModels() {
		if strings.HasPrefix(m.SHA256, "PLACEHOLDER_") {
			t.Errorf("model %s/%s still has PLACEHOLDER_ SHA256 hash %q — "+
				"compute and commit the real digest before release",
				m.Name, m.Backend, m.SHA256)
		}
	}
	for _, plat := range LlamaServerPlatforms() {
		sha, _ := LlamaServerSHA256For(plat)
		if strings.HasPrefix(sha, "PLACEHOLDER_") {
			t.Errorf("llama-server/%s still has PLACEHOLDER_ SHA256 hash %q — "+
				"compute and commit the real digest before release",
				plat, sha)
		}
	}
}

// TestAvailableModelsHaveRealSHA256 asserts that every entry a user can
// actually download carries a 64-character lowercase hex SHA256. Unavailable
// entries may hold a documented sentinel value instead.
func TestAvailableModelsHaveRealSHA256(t *testing.T) {
	for _, m := range AllModels() {
		if !m.Available {
			continue
		}
		if !hexSHA256Pattern.MatchString(m.SHA256) {
			t.Errorf("Available model %s/%s has SHA256 %q — want 64 lowercase hex chars",
				m.Name, m.Backend, m.SHA256)
		}
	}
}

// TestLlamaServerHashesRegistered asserts that every platform with a
// llama-server download URL has a corresponding real SHA256 digest — empty
// strings must never reach DownloadFileResumable for a supported platform.
func TestLlamaServerHashesRegistered(t *testing.T) {
	platforms := LlamaServerPlatforms()
	if len(platforms) == 0 {
		t.Fatal("llamaServerURLs is empty — at least one platform must be supported")
	}
	for _, plat := range platforms {
		sha, ok := LlamaServerSHA256For(plat)
		if !ok {
			t.Errorf("llama-server/%s has a URL but no SHA256 in llamaServerSHA256s", plat)
			continue
		}
		if !hexSHA256Pattern.MatchString(sha) {
			t.Errorf("llama-server/%s SHA256 %q is not 64 lowercase hex chars", plat, sha)
		}
	}
}

// TestLlamaServerSHA256 verifies LlamaServerSHA256() returns a real digest on
// supported host platforms (the ones CI runs on) and empty elsewhere.
func TestLlamaServerSHA256(t *testing.T) {
	got := LlamaServerSHA256()
	_, supported := LlamaServerSHA256For(runtime.GOOS + "/" + runtime.GOARCH)
	if supported {
		if !hexSHA256Pattern.MatchString(got) {
			t.Errorf("LlamaServerSHA256 on supported %s/%s = %q, want 64-hex digest",
				runtime.GOOS, runtime.GOARCH, got)
		}
	} else if got != "" {
		t.Errorf("LlamaServerSHA256 on unsupported %s/%s = %q, want empty",
			runtime.GOOS, runtime.GOARCH, got)
	}
}
