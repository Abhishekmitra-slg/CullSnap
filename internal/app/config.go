package app

import (
	"os"
	"path/filepath"
	"runtime"
)

// SystemProbe holds detected hardware and OS capabilities.
type SystemProbe struct {
	OS          string `json:"OS"`
	Arch        string `json:"Arch"`
	CPUs        int    `json:"CPUs"`
	RAMMB       int    `json:"RAMMB"`
	FDSoftLimit uint64 `json:"FDSoftLimit"`
	FFmpegReady bool   `json:"FFmpegReady"`
	StorageHint string `json:"StorageHint"`
}

// AppConfig holds all runtime-tunable performance parameters.
type AppConfig struct {
	MaxConnections       int         `json:"maxConnections"`
	ThumbnailWorkers     int         `json:"thumbnailWorkers"`
	ScannerWorkers       int         `json:"scannerWorkers"`
	ServerIdleTimeoutSec int         `json:"serverIdleTimeoutSec"`
	CacheDir             string      `json:"cacheDir"`
	Probe                SystemProbe `json:"probe"`
}

// DeriveDefaults calculates optimal AppConfig values from a SystemProbe.
func DeriveDefaults(probe SystemProbe) AppConfig {
	cfg := AppConfig{Probe: probe}

	// MaxConnections: CPUs×4, capped at 50 and fd_limit/4, floor 10
	maxConn := probe.CPUs * 4
	if probe.FDSoftLimit > 0 {
		fdCap := int(probe.FDSoftLimit / 4)
		if maxConn > fdCap {
			maxConn = fdCap
		}
	}
	if maxConn > 50 {
		maxConn = 50
	}
	if maxConn < 10 {
		maxConn = 10
	}
	cfg.MaxConnections = maxConn

	// ThumbnailWorkers: clamp(CPUs, 2, 8)
	tw := probe.CPUs
	if tw < 2 {
		tw = 2
	}
	if tw > 8 {
		tw = 8
	}
	cfg.ThumbnailWorkers = tw

	// ScannerWorkers: clamp(CPUs/2, 1, 4)
	sw := probe.CPUs / 2
	if sw < 1 {
		sw = 1
	}
	if sw > 4 {
		sw = 4
	}
	cfg.ScannerWorkers = sw

	// IdleTimeout: 30s Unix, 60s Windows
	if probe.OS == "windows" {
		cfg.ServerIdleTimeoutSec = 60
	} else {
		cfg.ServerIdleTimeoutSec = 30
	}

	// CacheDir: OS-appropriate location
	cacheBase, err := os.UserCacheDir()
	if err != nil {
		cacheBase, _ = os.UserHomeDir()
	}
	cfg.CacheDir = filepath.Join(cacheBase, "CullSnap", "thumbs")

	return cfg
}

// RunSystemProbe collects hardware and OS information.
func RunSystemProbe(ffmpegPath string) SystemProbe {
	probe := SystemProbe{
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		CPUs:        runtime.NumCPU(),
		StorageHint: "unknown",
	}
	probe.RAMMB = detectRAMMB()
	probe.FDSoftLimit = detectFDLimit()
	if _, err := os.Stat(ffmpegPath); err == nil {
		probe.FFmpegReady = true
	}
	return probe
}
