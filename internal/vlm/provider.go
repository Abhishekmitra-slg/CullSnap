package vlm

import (
	"context"
	"fmt"
)

// HardwareTier classifies the host machine's capability for VLM inference.
type HardwareTier int

const (
	TierLegacy  HardwareTier = iota // CPU-only, <8GB RAM — VLM disabled
	TierBasic                       // 8-12GB RAM — E2B only
	TierCapable                     // 16GB+ RAM, Apple M-series or discrete GPU — E4B
	TierPower                       // 32GB+, M-series Pro/Max/Ultra — E4B full precision
)

// String returns a human-readable name for the hardware tier.
func (t HardwareTier) String() string {
	switch t {
	case TierLegacy:
		return "legacy"
	case TierBasic:
		return "basic"
	case TierCapable:
		return "capable"
	case TierPower:
		return "power"
	default:
		return "unknown"
	}
}

// VLMProvider is the interface every VLM backend must implement.
type VLMProvider interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Health(ctx context.Context) error
	ScorePhoto(ctx context.Context, req ScoreRequest) (*VLMScore, error)
	RankPhotos(ctx context.Context, req RankRequest) (*RankingResult, error)
	ModelInfo() ModelInfo
}

// ScoreRequest describes a single photo to be scored by the VLM.
type ScoreRequest struct {
	PhotoPath   string            // Absolute path to thumbnail (300px cached)
	Context     string            // Optional: "portrait session", "landscape", "event"
	TokenBudget int               // Tokens per image (70-1120), set by hardware tier
	Metadata    map[string]string // EXIF hints: focal length, ISO, etc.
	FaceCount   int               // From ONNX Stage 1 — injected as context
	Sharpness   float64           // From ONNX Stage 1 — injected as context
}

// VLMScore is the structured output from a VLM individual scoring call.
type VLMScore struct {
	Aesthetic     float64  `json:"aesthetic"`
	Composition   float64  `json:"composition"`
	Expression    float64  `json:"expression"`
	TechnicalQual float64  `json:"technical_quality"`
	SceneType     string   `json:"scene_type"`
	Issues        []string `json:"issues"`
	Explanation   string   `json:"explanation"`
	TokensUsed    int      `json:"tokens_used"`
}

// Validate checks that all score fields are in [0,1] and explanation is present.
func (s VLMScore) Validate() error {
	for _, pair := range []struct {
		name string
		val  float64
	}{
		{"aesthetic", s.Aesthetic},
		{"composition", s.Composition},
		{"expression", s.Expression},
		{"technical_quality", s.TechnicalQual},
	} {
		if pair.val < 0 || pair.val > 1 {
			return fmt.Errorf("%s score %.2f out of range [0, 1]", pair.name, pair.val)
		}
	}
	if s.Explanation == "" {
		return fmt.Errorf("explanation is required")
	}
	if len(s.Issues) > 3 {
		return fmt.Errorf("max 3 issues, got %d", len(s.Issues))
	}
	return nil
}

// RankRequest describes a batch of photos to compare.
type RankRequest struct {
	PhotoPaths  []string // 3-5 photos to compare (thumbnails)
	UseCase     string   // "linkedin profile", "wedding album", "portfolio"
	TokenBudget int      // Higher budget for comparison (560-1120 tokens/image)
	// Per-photo ONNX context injected by pipeline.
	PhotoScores []PhotoContext
}

// PhotoContext carries ONNX-derived scores for a single photo in a ranking batch.
type PhotoContext struct {
	Aesthetic float64
	Sharpness float64
	FaceCount int
	Issues    []string
}

// RankingResult is the output of a VLM pairwise ranking call.
type RankingResult struct {
	Ranked      []RankedPhoto `json:"ranked"`
	Explanation string        `json:"explanation"`
	TokensUsed  int           `json:"tokens_used"`
}

// RankedPhoto is a single photo's rank within a comparison batch.
type RankedPhoto struct {
	PhotoPath string  `json:"photo_path"`
	Rank      int     `json:"rank"`
	Score     float64 `json:"score"`
	Notes     string  `json:"notes"`
}

// ModelInfo describes the currently loaded VLM model.
type ModelInfo struct {
	Name         string
	Variant      string // "Q4_K_M", "mlx-4bit"
	SizeBytes    int64
	RAMUsage     int64
	Backend      string // "mlx", "llamacpp"
	MaxImages    int
	TokenBudgets []int // [70, 140, 280, 560, 1120]
}

// HardwareProfile describes the host machine's hardware.
type HardwareProfile struct {
	CPUCores   int
	CPUModel   string
	TotalRAMMB int64
	AvailRAMMB int64
	GPUName    string
	GPUType    GPUType
	VRAMMB     int64
	OS         string
	Arch       string
	Tier       HardwareTier
	MLXCapable bool
}

// GPUType classifies the GPU type.
type GPUType int

const (
	GPUNone GPUType = iota
	GPUAppleSilicon
	GPUNVIDIA
	GPUAMD
	GPUIntegrated
)

// TokenBudgetPolicy controls token spending per session.
type TokenBudgetPolicy struct {
	Mode          string // "local" (unlimited) or "cloud" (budgeted)
	MaxPerPhoto   int
	MaxPerBatch   int
	MaxPerSession int // 0 = unlimited
	SessionUsed   int
}
