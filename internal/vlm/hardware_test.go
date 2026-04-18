package vlm

import (
	"runtime"
	"testing"
)

func TestClassifyTier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		prof HardwareProfile
		want HardwareTier
	}{
		// Apple Silicon tiers
		{
			name: "apple silicon 32GB → TierPower",
			prof: HardwareProfile{GPUType: GPUAppleSilicon, TotalRAMMB: 32 * 1024},
			want: TierPower,
		},
		{
			name: "apple silicon 16GB → TierCapable",
			prof: HardwareProfile{GPUType: GPUAppleSilicon, TotalRAMMB: 16 * 1024},
			want: TierCapable,
		},
		{
			name: "apple silicon 8GB → TierBasic",
			prof: HardwareProfile{GPUType: GPUAppleSilicon, TotalRAMMB: 8 * 1024},
			want: TierBasic,
		},
		{
			name: "apple silicon 4GB → TierLegacy",
			prof: HardwareProfile{GPUType: GPUAppleSilicon, TotalRAMMB: 4 * 1024},
			want: TierLegacy,
		},
		// NVIDIA tiers
		{
			name: "nvidia 8GB VRAM + 16GB RAM → TierCapable",
			prof: HardwareProfile{GPUType: GPUNVIDIA, VRAMMB: 8 * 1024, TotalRAMMB: 16 * 1024},
			want: TierCapable,
		},
		{
			name: "nvidia 4GB VRAM + 8GB RAM → TierBasic",
			prof: HardwareProfile{GPUType: GPUNVIDIA, VRAMMB: 4 * 1024, TotalRAMMB: 8 * 1024},
			want: TierBasic,
		},
		{
			name: "nvidia 2GB VRAM → TierLegacy",
			prof: HardwareProfile{GPUType: GPUNVIDIA, VRAMMB: 2 * 1024, TotalRAMMB: 16 * 1024},
			want: TierLegacy,
		},
		// CPU-only / integrated
		{
			name: "cpu-only 16GB → TierBasic",
			prof: HardwareProfile{GPUType: GPUNone, TotalRAMMB: 16 * 1024},
			want: TierBasic,
		},
		{
			name: "cpu-only 4GB → TierLegacy",
			prof: HardwareProfile{GPUType: GPUNone, TotalRAMMB: 4 * 1024},
			want: TierLegacy,
		},
		{
			name: "integrated GPU 16GB → TierBasic",
			prof: HardwareProfile{GPUType: GPUIntegrated, TotalRAMMB: 16 * 1024},
			want: TierBasic,
		},
		{
			name: "integrated GPU 8GB → TierLegacy",
			prof: HardwareProfile{GPUType: GPUIntegrated, TotalRAMMB: 8 * 1024},
			want: TierLegacy,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ClassifyTier(tc.prof)
			if got != tc.want {
				t.Errorf("ClassifyTier(%+v) = %s, want %s", tc.prof, got, tc.want)
			}
		})
	}
}

// TestClassifyTierMLXCapable verifies that ProbeHardware sets MLXCapable on darwin/arm64.
func TestClassifyTierMLXCapable(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("MLXCapable only set on darwin/arm64")
	}

	prof := ProbeHardware()
	if !prof.MLXCapable {
		t.Error("expected MLXCapable=true on darwin/arm64")
	}
	if prof.GPUType != GPUAppleSilicon {
		t.Errorf("expected GPUAppleSilicon on darwin/arm64, got %d", prof.GPUType)
	}
}

func TestExecutionPlanTierLegacy(t *testing.T) {
	t.Parallel()

	plan := BuildExecutionPlan(100, TierLegacy)

	if plan.VLMEnabled {
		t.Error("expected VLMEnabled=false for TierLegacy")
	}
	if plan.Mode != ModeONNXOnly {
		t.Errorf("expected ModeONNXOnly, got %s", plan.Mode)
	}
}

func TestExecutionPlanTierPowerSmall(t *testing.T) {
	t.Parallel()

	// 150 photos on TierPower: should be sync with stage5 groups
	plan := BuildExecutionPlan(150, TierPower)

	if !plan.VLMEnabled {
		t.Error("expected VLMEnabled=true for TierPower")
	}
	if plan.Mode != ModeSync {
		t.Errorf("expected ModeSync for 150 photos on TierPower, got %s", plan.Mode)
	}
	if plan.Stage5Count <= 0 {
		t.Errorf("expected Stage5Count>0 for 150 photos on TierPower, got %d", plan.Stage5Count)
	}
	if plan.Stage4Count != 150 {
		t.Errorf("expected Stage4Count=150, got %d", plan.Stage4Count)
	}
}

func TestExecutionPlanTierPowerLarge(t *testing.T) {
	t.Parallel()

	// 500 photos on TierPower: exceeds 200 threshold → background
	plan := BuildExecutionPlan(500, TierPower)

	if plan.Mode != ModeBackground {
		t.Errorf("expected ModeBackground for 500 photos on TierPower, got %s", plan.Mode)
	}
	if !plan.VLMEnabled {
		t.Error("expected VLMEnabled=true for TierPower")
	}
}

func TestExecutionPlanTierBasicSmall(t *testing.T) {
	t.Parallel()

	// 40 photos on TierBasic: <=50 → sync, stage5 disabled
	plan := BuildExecutionPlan(40, TierBasic)

	if !plan.VLMEnabled {
		t.Error("expected VLMEnabled=true for TierBasic")
	}
	if plan.Mode != ModeSync {
		t.Errorf("expected ModeSync for 40 photos on TierBasic, got %s", plan.Mode)
	}
	if plan.Stage5Count != 0 {
		t.Errorf("expected Stage5Count=0 for TierBasic, got %d", plan.Stage5Count)
	}
}

func TestBuildExecutionPlanCalibrated(t *testing.T) {
	t.Parallel()

	// With calibrated stage4 = 500ms/photo (faster than default 1500ms for TierCapable).
	plan := BuildExecutionPlanCalibrated(100, TierCapable, 500, 3000)
	// Stage4 should use calibrated: 100 * 500ms = 50000ms (vs default 100 * 1500ms = 150000ms).
	if plan.Stage4Estimate > 60000 {
		t.Errorf("Stage4 estimate %d ms too high for calibrated 500ms/photo", plan.Stage4Estimate)
	}
}

func TestBuildExecutionPlanCalibratedZeroFallback(t *testing.T) {
	t.Parallel()

	// Zero calibration should use defaults.
	calibrated := BuildExecutionPlanCalibrated(100, TierCapable, 0, 0)
	defaultPlan := BuildExecutionPlan(100, TierCapable)
	if calibrated.Stage4Estimate != defaultPlan.Stage4Estimate {
		t.Errorf("zero calibration should use default: got %d, want %d", calibrated.Stage4Estimate, defaultPlan.Stage4Estimate)
	}
}

func TestCalibrationKey(t *testing.T) {
	t.Parallel()

	key := CalibrationKey(TierCapable, "mlx", "gemma-4-e4b-it")
	if key != "vlm_bench_capable_mlx_gemma-4-e4b-it" {
		t.Errorf("key = %q, want vlm_bench_capable_mlx_gemma-4-e4b-it", key)
	}
}

func TestRecommendModel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tier HardwareTier
		want string
	}{
		{TierPower, "gemma-4-e4b-it"},
		{TierCapable, "gemma-4-e4b-it"},
		{TierBasic, "gemma-4-e2b-it"},
		{TierLegacy, ""},
	}
	for _, c := range cases {
		if got := RecommendModel(c.tier); got != c.want {
			t.Errorf("RecommendModel(%v) = %q, want %q", c.tier, got, c.want)
		}
	}
}

func TestRecommendBackend(t *testing.T) {
	t.Parallel()
	if got := RecommendBackend(HardwareProfile{MLXCapable: true}); got != "mlx" {
		t.Errorf("RecommendBackend(MLXCapable) = %q, want mlx", got)
	}
	if got := RecommendBackend(HardwareProfile{MLXCapable: false}); got != "llamacpp" {
		t.Errorf("RecommendBackend(!MLXCapable) = %q, want llamacpp", got)
	}
}
