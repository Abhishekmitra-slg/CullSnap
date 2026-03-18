package app

import (
	"runtime"
	"testing"
)

func TestDeriveDefaults_MinimumValues(t *testing.T) {
	probe := SystemProbe{
		OS:          "linux",
		Arch:        "amd64",
		CPUs:        1,
		RAMMB:       512,
		FDSoftLimit: 64,
		FFmpegReady: false,
		StorageHint: "unknown",
	}
	cfg := DeriveDefaults(probe)
	if cfg.MaxConnections < 10 {
		t.Errorf("MaxConnections floor should be 10, got %d", cfg.MaxConnections)
	}
	if cfg.ThumbnailWorkers < 2 {
		t.Errorf("ThumbnailWorkers floor should be 2, got %d", cfg.ThumbnailWorkers)
	}
	if cfg.ScannerWorkers < 1 {
		t.Errorf("ScannerWorkers floor should be 1, got %d", cfg.ScannerWorkers)
	}
}

func TestDeriveDefaults_IdleTimeoutByOS(t *testing.T) {
	probe := SystemProbe{OS: runtime.GOOS, CPUs: 4, RAMMB: 8192, FDSoftLimit: 1024}
	cfg := DeriveDefaults(probe)
	if runtime.GOOS == "windows" {
		if cfg.ServerIdleTimeoutSec != 60 {
			t.Errorf("Windows IdleTimeout should be 60, got %d", cfg.ServerIdleTimeoutSec)
		}
	} else {
		if cfg.ServerIdleTimeoutSec != 30 {
			t.Errorf("Unix IdleTimeout should be 30, got %d", cfg.ServerIdleTimeoutSec)
		}
	}
}

func TestDeriveDefaults_HighCPU(t *testing.T) {
	probe := SystemProbe{
		OS:          "darwin",
		CPUs:        16,
		RAMMB:       32768,
		FDSoftLimit: 10240,
	}
	cfg := DeriveDefaults(probe)
	if cfg.ThumbnailWorkers > 8 {
		t.Errorf("ThumbnailWorkers should be capped at 8, got %d", cfg.ThumbnailWorkers)
	}
	if cfg.ScannerWorkers > 4 {
		t.Errorf("ScannerWorkers should be capped at 4, got %d", cfg.ScannerWorkers)
	}
	if cfg.MaxConnections > 50 {
		t.Errorf("MaxConnections should be capped at 50, got %d", cfg.MaxConnections)
	}
}
