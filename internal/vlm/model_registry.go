package vlm

import (
	"cullsnap/internal/logger"
	"fmt"
	"runtime"
)

// ModelEntry describes a downloadable VLM model variant.
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
}

// builtinModels is the static registry of supported Gemma 4 VLM models.
var builtinModels = []ModelEntry{
	{
		Name:      "gemma-4-e4b-it",
		Variant:   "Q4_K_M",
		Format:    "gguf",
		Backend:   "llamacpp",
		URL:       "https://huggingface.co/unsloth/gemma-4-E4B-it-GGUF/resolve/main/gemma-4-E4B-it-Q4_K_M.gguf",
		SHA256:    "PLACEHOLDER_SHA256_E4B_GGUF",
		SizeBytes: 2_800_000_000,
		RAMUsage:  3_200_000_000,
		MinTier:   TierCapable,
		Filename:  "gemma-4-e4b-it-Q4_K_M.gguf",
	},
	{
		Name:      "gemma-4-e4b-it",
		Variant:   "mlx-4bit",
		Format:    "mlx",
		Backend:   "mlx",
		URL:       "https://huggingface.co/unsloth/gemma-4-E4B-it-mlx-4bit",
		SHA256:    "PLACEHOLDER_SHA256_E4B_MLX",
		SizeBytes: 2_600_000_000,
		RAMUsage:  3_000_000_000,
		MinTier:   TierCapable,
		Filename:  "gemma-4-e4b-it-mlx-4bit",
	},
	{
		Name:      "gemma-4-e2b-it",
		Variant:   "Q4_K_M",
		Format:    "gguf",
		Backend:   "llamacpp",
		URL:       "https://huggingface.co/unsloth/gemma-4-E2B-it-GGUF/resolve/main/gemma-4-E2B-it-Q4_K_M.gguf",
		SHA256:    "PLACEHOLDER_SHA256_E2B_GGUF",
		SizeBytes: 1_500_000_000,
		RAMUsage:  1_800_000_000,
		MinTier:   TierBasic,
		Filename:  "gemma-4-e2b-it-Q4_K_M.gguf",
	},
	{
		Name:      "gemma-4-e2b-it",
		Variant:   "mlx-4bit",
		Format:    "mlx",
		Backend:   "mlx",
		URL:       "https://huggingface.co/unsloth/gemma-4-E2B-it-mlx-4bit",
		SHA256:    "PLACEHOLDER_SHA256_E2B_MLX",
		SizeBytes: 1_400_000_000,
		RAMUsage:  1_600_000_000,
		MinTier:   TierBasic,
		Filename:  "gemma-4-e2b-it-mlx-4bit",
	},
}

// llamaServerURLs maps "GOOS/GOARCH" to the llama.cpp release download URL (b5280).
var llamaServerURLs = map[string]string{
	"darwin/arm64": "https://github.com/ggml-org/llama.cpp/releases/download/b5280/llama-b5280-bin-macos-arm64.zip",
	"darwin/amd64": "https://github.com/ggml-org/llama.cpp/releases/download/b5280/llama-b5280-bin-macos-x64.zip",
	"linux/amd64":  "https://github.com/ggml-org/llama.cpp/releases/download/b5280/llama-b5280-bin-ubuntu-x64.zip",
}

// LookupModel returns the ModelEntry matching name and backend.
// Returns false if no matching entry is found.
func LookupModel(name, backend string) (ModelEntry, bool) {
	if logger.Log != nil {
		logger.Log.Debug("vlm: looking up model", "name", name, "backend", backend)
	}

	for i := range builtinModels {
		m := &builtinModels[i]
		if m.Name == name && m.Backend == backend {
			if logger.Log != nil {
				logger.Log.Debug("vlm: model found", "name", m.Name, "variant", m.Variant)
			}
			return *m, true
		}
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: model not found", "name", name, "backend", backend)
	}
	return ModelEntry{}, false
}

// ModelsForTier returns all models whose MinTier is <= tier.
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
		if m.MinTier <= tier {
			result = append(result, *m)
		}
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: models available for tier", "count", len(result), "tier", tier.String())
	}
	return result
}

// AllModels returns a copy of the full builtin model registry.
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
