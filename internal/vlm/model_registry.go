package vlm

import (
	"cullsnap/internal/logger"
	"fmt"
	"runtime"
)

// UnreleasedMLXSentinel marks MLX entries whose download pipeline is not yet
// implemented. ModelEntry.Available is false for these entries, and the
// placeholder-hash CI test accepts this sentinel as a documented stub.
// Tracked by the MLX download redesign follow-up issue.
const UnreleasedMLXSentinel = "UNRELEASED_MLX_PENDING_DOWNLOAD_REDESIGN"

// ModelEntry describes a downloadable VLM model variant.
//
// Available indicates whether the entry is ready for end-user provisioning.
// MLX entries are currently Available=false because their download pipeline
// requires multi-file HF repo fetches that DownloadFileResumable cannot express.
// User-facing selection (ModelsForTier) excludes unavailable entries.
type ModelEntry struct {
	Name      string       // e.g. "gemma-4-e4b-it"
	Variant   string       // e.g. "Q4_K_M", "mlx-4bit"
	Format    string       // "gguf" or "mlx"
	Backend   string       // "llamacpp" or "mlx"
	URL       string       // Download URL (HuggingFace)
	SHA256    string       // Expected hex SHA256 of the downloaded artifact
	SizeBytes int64        // Approximate download size in bytes
	RAMUsage  int64        // Approximate runtime RAM usage in bytes
	MinTier   HardwareTier // Minimum hardware tier required
	Filename  string       // Local filename under ~/.cullsnap/models/
	Available bool         // False when the backend's download path is not yet wired up
}

// builtinModels is the static registry of supported Gemma 4 VLM models.
//
// GGUF SHA256 values come from HuggingFace Git LFS OIDs (x-linked-etag header)
// and match the sha256 computed from the downloaded artifact.
// MLX entries remain in the registry for backend coverage but are marked
// Available=false until the MLX download pipeline redesign lands.
var builtinModels = []ModelEntry{
	{
		Name:      "gemma-4-e4b-it",
		Variant:   "Q4_K_M",
		Format:    "gguf",
		Backend:   "llamacpp",
		URL:       "https://huggingface.co/unsloth/gemma-4-E4B-it-GGUF/resolve/main/gemma-4-E4B-it-Q4_K_M.gguf",
		SHA256:    "dff0ffba4c90b4082d70214d53ce9504a28d4d8d998276dcb3b8881a656c742a",
		SizeBytes: 4_977_169_088,
		RAMUsage:  5_500_000_000,
		MinTier:   TierCapable,
		Filename:  "gemma-4-e4b-it-Q4_K_M.gguf",
		Available: true,
	},
	{
		Name:      "gemma-4-e4b-it",
		Variant:   "mlx-4bit",
		Format:    "mlx",
		Backend:   "mlx",
		URL:       "https://huggingface.co/unsloth/gemma-4-E4B-it-mlx-4bit",
		SHA256:    UnreleasedMLXSentinel,
		SizeBytes: 2_600_000_000,
		RAMUsage:  3_000_000_000,
		MinTier:   TierCapable,
		Filename:  "gemma-4-e4b-it-mlx-4bit",
		Available: false,
	},
	{
		Name:      "gemma-4-e2b-it",
		Variant:   "Q4_K_M",
		Format:    "gguf",
		Backend:   "llamacpp",
		URL:       "https://huggingface.co/unsloth/gemma-4-E2B-it-GGUF/resolve/main/gemma-4-E2B-it-Q4_K_M.gguf",
		SHA256:    "ac0069ebccd39925d836f24a88c0f0c858d20578c29b21ab7cedce66ee576845",
		SizeBytes: 3_106_735_776,
		RAMUsage:  3_500_000_000,
		MinTier:   TierBasic,
		Filename:  "gemma-4-e2b-it-Q4_K_M.gguf",
		Available: true,
	},
	{
		Name:      "gemma-4-e2b-it",
		Variant:   "mlx-4bit",
		Format:    "mlx",
		Backend:   "mlx",
		URL:       "https://huggingface.co/unsloth/gemma-4-E2B-it-mlx-4bit",
		SHA256:    UnreleasedMLXSentinel,
		SizeBytes: 1_400_000_000,
		RAMUsage:  1_600_000_000,
		MinTier:   TierBasic,
		Filename:  "gemma-4-e2b-it-mlx-4bit",
		Available: false,
	},
}

// llamaServerURLs maps "GOOS/GOARCH" to the llama.cpp release download URL (b5280).
var llamaServerURLs = map[string]string{
	"darwin/arm64": "https://github.com/ggml-org/llama.cpp/releases/download/b5280/llama-b5280-bin-macos-arm64.zip",
	"darwin/amd64": "https://github.com/ggml-org/llama.cpp/releases/download/b5280/llama-b5280-bin-macos-x64.zip",
	"linux/amd64":  "https://github.com/ggml-org/llama.cpp/releases/download/b5280/llama-b5280-bin-ubuntu-x64.zip",
}

// llamaServerSHA256s holds the SHA256 digest of each llama-server release zip,
// keyed by "GOOS/GOARCH". Values are the lowercase hex digest of the downloaded
// zip bytes (verified against the GitHub release asset at build b5280).
var llamaServerSHA256s = map[string]string{
	"darwin/arm64": "75a2b54875248fc7407f7a29da3ec00f428b08a57f5a7cf5e9abc7eab3f55096",
	"darwin/amd64": "b6f7b5aa44ea7121d1171df2426823e063445f73692a650baff5a8f5deb80f6b",
	"linux/amd64":  "349d1835950f077d67b3ffec1c4b81a9c6d2b5b074c0e15f2b7fc6b6d87e5feb",
}

// LookupModel returns the ModelEntry matching name and backend.
// Returns entries regardless of their Available flag so callers that need to
// inspect unavailable backends (tests, diagnostics) can still reach them.
// Callers that intend to provision a model must check ModelEntry.Available.
func LookupModel(name, backend string) (ModelEntry, bool) {
	if logger.Log != nil {
		logger.Log.Debug("vlm: looking up model", "name", name, "backend", backend)
	}

	for i := range builtinModels {
		m := &builtinModels[i]
		if m.Name == name && m.Backend == backend {
			if logger.Log != nil {
				logger.Log.Debug("vlm: model found",
					"name", m.Name, "variant", m.Variant, "available", m.Available)
			}
			return *m, true
		}
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: model not found", "name", name, "backend", backend)
	}
	return ModelEntry{}, false
}

// ModelsForTier returns all Available models whose MinTier is <= tier.
// Unavailable entries (backends whose download pipeline is not yet wired up)
// are excluded because this feeds user-facing selection.
// TierLegacy always returns an empty slice since VLM is disabled at that tier.
func ModelsForTier(tier HardwareTier) []ModelEntry {
	if logger.Log != nil {
		logger.Log.Debug("vlm: filtering models for tier", "tier", tier.String())
	}

	if tier == TierLegacy {
		if logger.Log != nil {
			logger.Log.Debug("vlm: TierLegacy — returning no models")
		}
		return nil
	}

	result := make([]ModelEntry, 0, len(builtinModels))
	for i := range builtinModels {
		m := &builtinModels[i]
		if !m.Available {
			continue
		}
		if m.MinTier <= tier {
			result = append(result, *m)
		}
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: models available for tier", "count", len(result), "tier", tier.String())
	}
	return result
}

// AllModels returns a copy of the full builtin model registry, including
// unavailable entries. Intended for diagnostic tools and registry tests.
func AllModels() []ModelEntry {
	if logger.Log != nil {
		logger.Log.Debug("vlm: returning all models", "count", len(builtinModels))
	}
	out := make([]ModelEntry, len(builtinModels))
	copy(out, builtinModels)
	return out
}

// LlamaServerDownloadURL returns the llama-server binary download URL for the
// current runtime OS and architecture. Returns an empty string with a log warning
// if the platform is not supported.
func LlamaServerDownloadURL() string {
	key := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	url, ok := llamaServerURLs[key]
	if !ok {
		if logger.Log != nil {
			logger.Log.Warn("vlm: no llama-server download URL for platform", "platform", key)
		}
		return ""
	}
	if logger.Log != nil {
		logger.Log.Debug("vlm: llama-server download URL resolved", "platform", key, "url", url)
	}
	return url
}

// LlamaServerSHA256 returns the expected SHA256 digest of the llama-server
// release zip for the current runtime OS and architecture. Returns an empty
// string and logs a warning when the platform is not in the registry — callers
// must treat that as a fatal provisioning error, not a verification skip.
func LlamaServerSHA256() string {
	key := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	sha, ok := llamaServerSHA256s[key]
	if !ok {
		if logger.Log != nil {
			logger.Log.Warn("vlm: no llama-server SHA256 for platform", "platform", key)
		}
		return ""
	}
	if logger.Log != nil {
		logger.Log.Debug("vlm: llama-server SHA256 resolved", "platform", key)
	}
	return sha
}

// LlamaServerPlatforms returns the sorted list of "GOOS/GOARCH" keys for which
// llama-server binaries are registered. Used by the placeholder-hash CI test
// to scan every platform hash, independent of the host we happen to test on.
func LlamaServerPlatforms() []string {
	keys := make([]string, 0, len(llamaServerURLs))
	for k := range llamaServerURLs {
		keys = append(keys, k)
	}
	return keys
}

// LlamaServerSHA256For returns the SHA256 digest registered for the given
// "GOOS/GOARCH" key, regardless of the runtime platform. Returns false when
// the platform is not in the registry.
func LlamaServerSHA256For(platform string) (string, bool) {
	sha, ok := llamaServerSHA256s[platform]
	return sha, ok
}
