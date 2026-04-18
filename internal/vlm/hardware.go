package vlm

import (
	"cullsnap/internal/logger"
	"fmt"
	"runtime"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// ExecutionMode describes how the VLM pipeline should run relative to the caller.
type ExecutionMode int

const (
	ModeONNXOnly   ExecutionMode = iota // VLM disabled; ONNX-only scoring
	ModeSync                            // VLM runs inline, caller waits for results
	ModeBackground                      // VLM runs in background goroutine; results delivered async
)

// String returns a human-readable name for the execution mode.
func (m ExecutionMode) String() string {
	switch m {
	case ModeONNXOnly:
		return "onnx_only"
	case ModeSync:
		return "sync"
	case ModeBackground:
		return "background"
	default:
		return "unknown"
	}
}

// ExecutionPlan is the output of BuildExecutionPlan — it describes exactly what
// will run, how long it should take, and how the caller should schedule it.
type ExecutionPlan struct {
	PhotoCount   int
	HardwareTier HardwareTier
	ONNXEstimate int64 // ms — ONNX-only scoring estimate

	VLMEnabled bool
	VLMModel   string
	VLMBackend string

	// Stage 4 — per-photo VLM scoring
	Stage4Count    int
	Stage4Estimate int64 // ms total

	// Stage 5 — group ranking (batch)
	Stage5Count    int   // number of groups
	Stage5Estimate int64 // ms total

	TotalEstimate int64 // ms — all stages combined

	Mode        ExecutionMode
	UserMessage string // human-readable summary for UI
}

// ProbeHardware detects CPU, RAM, and GPU information from the host machine
// and returns a populated HardwareProfile with the classified tier.
func ProbeHardware() HardwareProfile {
	prof := HardwareProfile{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	// CPU info
	if infos, err := cpu.Info(); err != nil {
		if logger.Log != nil {
			logger.Log.Warn("vlm: failed to query CPU info", "err", err)
		}
	} else if len(infos) > 0 {
		prof.CPUModel = infos[0].ModelName
		prof.CPUCores = int(infos[0].Cores)
		if logger.Log != nil {
			logger.Log.Debug("vlm: CPU detected", "model", prof.CPUModel, "cores", prof.CPUCores)
		}
	}

	// RAM info
	if vm, err := mem.VirtualMemory(); err != nil {
		if logger.Log != nil {
			logger.Log.Warn("vlm: failed to query memory", "err", err)
		}
	} else {
		prof.TotalRAMMB = int64(vm.Total) / (1024 * 1024)
		prof.AvailRAMMB = int64(vm.Available) / (1024 * 1024)
		if logger.Log != nil {
			logger.Log.Debug("vlm: RAM detected", "total_mb", prof.TotalRAMMB, "avail_mb", prof.AvailRAMMB)
		}
	}

	// GPU detection — Apple Silicon identified by OS + arch
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		prof.GPUType = GPUAppleSilicon
		prof.MLXCapable = true
		prof.GPUName = "Apple Silicon"
		if logger.Log != nil {
			logger.Log.Debug("vlm: Apple Silicon GPU detected, MLX capable")
		}
	}

	prof.Tier = ClassifyTier(prof)
	if logger.Log != nil {
		logger.Log.Debug("vlm: hardware probe complete",
			"tier", prof.Tier,
			"gpu", prof.GPUType,
			"mlx", prof.MLXCapable,
			"ram_mb", prof.TotalRAMMB,
		)
	}
	return prof
}

// ClassifyTier maps a HardwareProfile to a HardwareTier based on GPU type,
// VRAM, and system RAM.
func ClassifyTier(prof HardwareProfile) HardwareTier {
	switch prof.GPUType {
	case GPUAppleSilicon:
		switch {
		case prof.TotalRAMMB >= 32*1024:
			return TierPower
		case prof.TotalRAMMB >= 16*1024:
			return TierCapable
		case prof.TotalRAMMB >= 8*1024:
			return TierBasic
		default:
			return TierLegacy
		}

	case GPUNVIDIA:
		switch {
		case prof.VRAMMB >= 8*1024 && prof.TotalRAMMB >= 16*1024:
			return TierCapable
		case prof.VRAMMB >= 4*1024 && prof.TotalRAMMB >= 8*1024:
			return TierBasic
		default:
			return TierLegacy
		}

	default: // GPUNone, GPUIntegrated, GPUAMD (no VRAM reporting)
		switch {
		case prof.TotalRAMMB >= 16*1024:
			return TierBasic
		default:
			return TierLegacy
		}
	}
}

// RecommendModel returns the VLM model identifier best suited for the given tier.
// Returns "" for TierLegacy (VLM disabled).
func RecommendModel(tier HardwareTier) string {
	switch tier {
	case TierPower, TierCapable:
		return "gemma-4-e4b-it"
	case TierBasic:
		return "gemma-4-e2b-it"
	default:
		return ""
	}
}

// RecommendBackend returns the preferred inference backend for the hardware profile.
func RecommendBackend(prof HardwareProfile) string {
	if prof.MLXCapable {
		return "mlx"
	}
	return "llamacpp"
}

// BuildExecutionPlan constructs a full execution plan for the given photo count
// and hardware tier, including stage estimates and scheduling mode.
func BuildExecutionPlan(photoCount int, tier HardwareTier) ExecutionPlan {
	plan := ExecutionPlan{
		PhotoCount:   photoCount,
		HardwareTier: tier,
		ONNXEstimate: int64(photoCount) * 50, // ~50ms per photo for ONNX stages (face detection + sharpness)
	}

	if tier == TierLegacy {
		plan.VLMEnabled = false
		plan.Mode = ModeONNXOnly
		plan.Stage4Count = photoCount
		plan.TotalEstimate = plan.ONNXEstimate
		plan.UserMessage = "ONNX scoring only (hardware tier too low for VLM)"
		if logger.Log != nil {
			logger.Log.Debug("vlm: execution plan — ONNX only", "reason", "TierLegacy", "photos", photoCount)
		}
		return plan
	}

	plan.VLMEnabled = true
	plan.Stage4Count = photoCount

	// Stage 5 groups: 1 group per 5 photos, skipped on TierBasic or TierCapable>500
	groupCount := photoCount / 5
	switch tier {
	case TierBasic:
		groupCount = 0
	case TierCapable:
		if photoCount > 500 {
			groupCount = 0
		}
	}
	plan.Stage5Count = groupCount

	// Time estimates per tier
	var msPerPhotoStage4, msPerGroupStage5 int64
	switch tier {
	case TierPower:
		msPerPhotoStage4 = 800
		msPerGroupStage5 = 3000
	case TierCapable:
		msPerPhotoStage4 = 1500
		msPerGroupStage5 = 5000
	case TierBasic:
		msPerPhotoStage4 = 4000
		msPerGroupStage5 = 0
	}

	plan.Stage4Estimate = int64(plan.Stage4Count) * msPerPhotoStage4
	plan.Stage5Estimate = int64(plan.Stage5Count) * msPerGroupStage5
	plan.TotalEstimate = plan.ONNXEstimate + plan.Stage4Estimate + plan.Stage5Estimate

	// Scheduling mode
	switch tier {
	case TierPower:
		if photoCount <= 200 {
			plan.Mode = ModeSync
		} else {
			plan.Mode = ModeBackground
		}
	case TierCapable:
		if photoCount <= 100 {
			plan.Mode = ModeSync
		} else {
			plan.Mode = ModeBackground
		}
	case TierBasic:
		if photoCount <= 50 {
			plan.Mode = ModeSync
		} else {
			plan.Mode = ModeBackground
		}
	}

	plan.UserMessage = fmt.Sprintf(
		"VLM enabled (%s tier): %d photos stage-4, %d groups stage-5, mode=%s, est=%dms",
		tier, plan.Stage4Count, plan.Stage5Count, plan.Mode, plan.TotalEstimate,
	)

	if logger.Log != nil {
		logger.Log.Debug("vlm: execution plan built",
			"tier", tier,
			"photos", photoCount,
			"stage4_count", plan.Stage4Count,
			"stage5_count", plan.Stage5Count,
			"mode", plan.Mode,
			"total_est_ms", plan.TotalEstimate,
		)
	}

	return plan
}

// CalibrationKey returns the config KV key for a calibrated timing.
func CalibrationKey(tier HardwareTier, backend, model string) string {
	return fmt.Sprintf("vlm_bench_%s_%s_%s", tier, backend, model)
}

// BuildExecutionPlanCalibrated creates a plan using calibrated timings if available.
// calibratedStage4Ms and calibratedStage5Ms are per-photo/per-group times from a previous run (0 = use defaults).
func BuildExecutionPlanCalibrated(photoCount int, tier HardwareTier, calibratedStage4Ms int, calibratedStage5Ms int) ExecutionPlan {
	plan := BuildExecutionPlan(photoCount, tier)
	if !plan.VLMEnabled {
		return plan
	}

	// Override estimates with calibrated values if available.
	if calibratedStage4Ms > 0 {
		plan.Stage4Estimate = int64(photoCount) * int64(calibratedStage4Ms)
	}
	if calibratedStage5Ms > 0 && plan.Stage5Count > 0 {
		plan.Stage5Estimate = int64(plan.Stage5Count) * int64(calibratedStage5Ms)
	}
	plan.TotalEstimate = plan.ONNXEstimate + plan.Stage4Estimate + plan.Stage5Estimate

	// Update user message with calibrated timing.
	totalSec := int(plan.TotalEstimate / 1000)
	if plan.Mode == ModeSync {
		plan.UserMessage = fmt.Sprintf("Analyzing %d photos — ~%d min", photoCount, max(totalSec/60, 1))
	} else {
		plan.UserMessage = fmt.Sprintf("~%d min. You can minimize and keep working.", max(totalSec/60, 1))
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: calibrated execution plan built",
			"tier", tier,
			"photos", photoCount,
			"calibrated_stage4_ms", calibratedStage4Ms,
			"calibrated_stage5_ms", calibratedStage5Ms,
			"stage4_est_ms", plan.Stage4Estimate,
			"stage5_est_ms", plan.Stage5Estimate,
			"total_est_ms", plan.TotalEstimate,
		)
	}

	return plan
}
