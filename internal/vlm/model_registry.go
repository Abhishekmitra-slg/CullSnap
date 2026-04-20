package vlm

import (
	"cullsnap/internal/hfclient"
	"cullsnap/internal/logger"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"runtime"
)

// ServerEngine identifies the MLX inference server implementation.
type ServerEngine string

const (
	EngineMLXVLM  ServerEngine = "mlx_vlm"
	EngineVLLMMLX ServerEngine = "vllm_mlx"
)

// ModelManifest is the unified registry shape for all VLM model variants.
type ModelManifest struct {
	Name    string       `json:"name"`
	Variant string       `json:"variant"`
	Format  string       `json:"format"`
	Engine  ServerEngine `json:"engine,omitempty"`
	Backend string       `json:"backend"`

	Repo          string               `json:"repo,omitempty"`
	CommitSHA     string               `json:"commit_sha,omitempty"`
	AllowPatterns []string             `json:"allow_patterns,omitempty"`
	PerFile       []hfclient.FileEntry `json:"per_file,omitempty"`

	SizeBytes int64        `json:"size_bytes"`
	RAMUsage  int64        `json:"ram_usage"`
	MinTier   HardwareTier `json:"min_tier"`

	URL      string `json:"url,omitempty"`
	SHA256   string `json:"sha256,omitempty"`
	Filename string `json:"filename,omitempty"`

	Available bool `json:"available"`
}

//go:embed manifests/*.json
var manifestFS embed.FS

var builtinManifests []ModelManifest

func init() {
	entries, err := fs.ReadDir(manifestFS, "manifests")
	if err != nil {
		panic(fmt.Sprintf("vlm: read manifests: %v", err))
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := manifestFS.ReadFile("manifests/" + e.Name())
		if err != nil {
			panic(fmt.Sprintf("vlm: read manifest %s: %v", e.Name(), err))
		}
		var m ModelManifest
		if err := json.Unmarshal(b, &m); err != nil {
			panic(fmt.Sprintf("vlm: parse manifest %s: %v", e.Name(), err))
		}
		builtinManifests = append(builtinManifests, m)
	}
	if logger.Log != nil {
		logger.Log.Debug("vlm: loaded manifests", "count", len(builtinManifests))
	}
}

// LookupModel returns the manifest matching name, backend, and engine ("" for gguf).
// Returns entries regardless of their Available flag so callers that need to
// inspect unavailable backends (tests, diagnostics) can still reach them.
// Callers that intend to provision a model must check ModelManifest.Available.
func LookupModel(name, backend string, engine ServerEngine) (ModelManifest, bool) {
	if logger.Log != nil {
		logger.Log.Debug("vlm: looking up model", "name", name, "backend", backend, "engine", engine)
	}

	for i := range builtinManifests {
		m := builtinManifests[i]
		if m.Name != name || m.Backend != backend {
			continue
		}
		if m.Backend == "mlx" && m.Engine != engine {
			continue
		}
		if logger.Log != nil {
			logger.Log.Debug("vlm: model found",
				"name", m.Name, "variant", m.Variant, "available", m.Available)
		}
		return m, true
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: model not found", "name", name, "backend", backend, "engine", engine)
	}
	return ModelManifest{}, false
}

// ModelsForTier returns all Available manifests whose MinTier ≤ tier.
// TierLegacy always returns an empty slice since VLM is disabled at that tier.
func ModelsForTier(tier HardwareTier) []ModelManifest {
	if logger.Log != nil {
		logger.Log.Debug("vlm: filtering models for tier", "tier", tier.String())
	}

	if tier == TierLegacy {
		if logger.Log != nil {
			logger.Log.Debug("vlm: TierLegacy — returning no models")
		}
		return nil
	}

	out := make([]ModelManifest, 0, len(builtinManifests))
	for i := range builtinManifests {
		m := &builtinManifests[i]
		if m.Available && m.MinTier <= tier {
			out = append(out, *m)
		}
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: models available for tier", "count", len(out), "tier", tier.String())
	}
	return out
}

// AllModels returns every manifest including unavailable entries.
// Intended for diagnostic tools and registry tests.
func AllModels() []ModelManifest {
	if logger.Log != nil {
		logger.Log.Debug("vlm: returning all models", "count", len(builtinManifests))
	}
	out := make([]ModelManifest, len(builtinManifests))
	copy(out, builtinManifests)
	return out
}

// ----- llama-server URLs (unchanged from before) -----

var llamaServerURLs = map[string]string{
	"darwin/arm64": "https://github.com/ggml-org/llama.cpp/releases/download/b5280/llama-b5280-bin-macos-arm64.zip",
	"darwin/amd64": "https://github.com/ggml-org/llama.cpp/releases/download/b5280/llama-b5280-bin-macos-x64.zip",
	"linux/amd64":  "https://github.com/ggml-org/llama.cpp/releases/download/b5280/llama-b5280-bin-ubuntu-x64.zip",
}

var llamaServerSHA256s = map[string]string{
	"darwin/arm64": "75a2b54875248fc7407f7a29da3ec00f428b08a57f5a7cf5e9abc7eab3f55096",
	"darwin/amd64": "b6f7b5aa44ea7121d1171df2426823e063445f73692a650baff5a8f5deb80f6b",
	"linux/amd64":  "349d1835950f077d67b3ffec1c4b81a9c6d2b5b074c0e15f2b7fc6b6d87e5feb",
}

// LlamaServerDownloadURL returns the llama-server binary download URL for the
// current runtime OS and architecture. Returns an empty string if the platform is unsupported.
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
// release zip for the current runtime OS and architecture.
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

// LlamaServerPlatforms returns the list of "GOOS/GOARCH" keys for which
// llama-server binaries are registered.
func LlamaServerPlatforms() []string {
	keys := make([]string, 0, len(llamaServerURLs))
	for k := range llamaServerURLs {
		keys = append(keys, k)
	}
	return keys
}

// LlamaServerSHA256For returns the SHA256 digest registered for the given
// "GOOS/GOARCH" key. Returns false when the platform is not in the registry.
func LlamaServerSHA256For(platform string) (string, bool) {
	sha, ok := llamaServerSHA256s[platform]
	return sha, ok
}
