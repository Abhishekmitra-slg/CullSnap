package vlm

import (
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// hexSHA256Pattern matches a 64-character lowercase hex SHA256 digest.
var hexSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// manifestCommitSHARE matches a 40-character lowercase hex git commit SHA.
var manifestCommitSHARE = regexp.MustCompile(`^[0-9a-f]{40}$`)

// manifestSHA1RE matches a 40-character lowercase hex git blob SHA-1.
var manifestSHA1RE = regexp.MustCompile(`^[0-9a-f]{40}$`)

func TestRegistryLookup(t *testing.T) {
	m, ok := LookupModel("gemma-4-e4b-it", "llamacpp", "")
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
	m, ok := LookupModel("gemma-4-e4b-it", "mlx", EngineMLXVLM)
	if !ok {
		t.Fatal("expected to find gemma-4-e4b-it/mlx/mlx_vlm but got false")
	}
	if m.Format != "mlx" {
		t.Errorf("expected format=mlx, got %q", m.Format)
	}
}

func TestRegistryLookupVLLMMLX(t *testing.T) {
	m, ok := LookupModel("gemma-4-e4b-it", "mlx", EngineVLLMMLX)
	if !ok {
		t.Fatal("expected to find gemma-4-e4b-it/mlx/vllm_mlx but got false")
	}
	if m.Engine != EngineVLLMMLX {
		t.Errorf("expected engine=vllm_mlx, got %q", m.Engine)
	}
}

func TestRegistryLookupE2B(t *testing.T) {
	m, ok := LookupModel("gemma-4-e2b-it", "llamacpp", "")
	if !ok {
		t.Fatal("expected to find gemma-4-e2b-it/llamacpp but got false")
	}
	// E2B should be strictly smaller than E4B but Q4_K_M of the 2B variant is
	// still several GB — just assert the ordering invariant, not an absolute bound.
	e4b, _ := LookupModel("gemma-4-e4b-it", "llamacpp", "")
	if m.SizeBytes <= 0 {
		t.Errorf("expected SizeBytes > 0, got %d", m.SizeBytes)
	}
	if m.SizeBytes >= e4b.SizeBytes {
		t.Errorf("expected E2B (%d) to be smaller than E4B (%d)", m.SizeBytes, e4b.SizeBytes)
	}
}

func TestRegistryLookupUnknown(t *testing.T) {
	_, ok := LookupModel("nonexistent", "llamacpp", "")
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
// Available=false. Without this guard, users could select models whose
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

// TestNoPlaceholderHashes is the release-blocker CI gate from issue #113:
// refuse to ship if any registry entry still carries a "PLACEHOLDER_" prefix.
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

// TestAvailableGGUFModelsHaveRealSHA256 asserts that every GGUF entry a user
// can actually download carries a 64-character lowercase hex SHA256.
// MLX manifests use per-file integrity (PerFile[].SHA256) instead of a
// top-level SHA256, so they are excluded from this check.
func TestAvailableGGUFModelsHaveRealSHA256(t *testing.T) {
	for _, m := range AllModels() {
		if !m.Available || m.Format != "gguf" {
			continue
		}
		if !hexSHA256Pattern.MatchString(m.SHA256) {
			t.Errorf("Available GGUF model %s/%s has SHA256 %q — want 64 lowercase hex chars",
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

// TestAllManifestsHaveRequiredFields checks every manifest has a non-empty name,
// variant, format, and backend.
func TestAllManifestsHaveRequiredFields(t *testing.T) {
	for _, m := range AllModels() {
		if m.Name == "" {
			t.Errorf("manifest missing name: %+v", m)
		}
		if m.Variant == "" {
			t.Errorf("manifest %s missing variant", m.Name)
		}
		if m.Format == "" {
			t.Errorf("manifest %s/%s missing format", m.Name, m.Variant)
		}
		if m.Backend == "" {
			t.Errorf("manifest %s/%s missing backend", m.Name, m.Variant)
		}
		if m.SizeBytes <= 0 {
			t.Errorf("manifest %s/%s has SizeBytes=%d, want >0", m.Name, m.Variant, m.SizeBytes)
		}
		if m.RAMUsage <= 0 {
			t.Errorf("manifest %s/%s has RAMUsage=%d, want >0", m.Name, m.Variant, m.RAMUsage)
		}
	}
}

// TestEveryManifestValid iterates builtinManifests and asserts per-manifest
// integrity: MLX manifests must have a valid 40-hex commit_sha and at least one
// per_file entry with valid hashes; llamacpp manifests must have a real SHA-256
// and a non-empty download URL.
func TestEveryManifestValid(t *testing.T) {
	for _, m := range builtinManifests {
		m := m
		t.Run(m.Name+"."+m.Variant, func(t *testing.T) {
			switch m.Backend {
			case "mlx":
				if !manifestCommitSHARE.MatchString(m.CommitSHA) {
					t.Fatalf("bad commit_sha: %q", m.CommitSHA)
				}
				if len(m.PerFile) == 0 {
					t.Fatal("empty per_file")
				}
				for _, fe := range m.PerFile {
					if fe.Path == "" {
						t.Fatal("per_file entry has empty path")
					}
					if fe.IsLFS && !hexSHA256Pattern.MatchString(fe.SHA256) {
						t.Fatalf("file %q: bad SHA-256: %q", fe.Path, fe.SHA256)
					}
					if !fe.IsLFS && !manifestSHA1RE.MatchString(fe.SHA1) {
						t.Fatalf("file %q: bad git SHA-1: %q", fe.Path, fe.SHA1)
					}
				}
			case "llamacpp":
				if !hexSHA256Pattern.MatchString(m.SHA256) {
					t.Fatalf("bad sha256: %q", m.SHA256)
				}
				if m.URL == "" {
					t.Fatal("empty url")
				}
			}
		})
	}
}

// TestNoSentinelHashes asserts that no manifest carries the old
// UNRELEASED_MLX_PENDING_DOWNLOAD_REDESIGN sentinel hash which was used as a
// placeholder before real digest data was computed.
func TestNoSentinelHashes(t *testing.T) {
	for _, m := range builtinManifests {
		if m.SHA256 == "UNRELEASED_MLX_PENDING_DOWNLOAD_REDESIGN" {
			t.Fatalf("manifest %s.%s carries unreleased sentinel", m.Name, m.Variant)
		}
	}
}
