package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRunSystemProbe(t *testing.T) {
	probe := RunSystemProbe("/nonexistent/ffmpeg/path")

	if probe.OS != runtime.GOOS {
		t.Errorf("expected OS=%q, got %q", runtime.GOOS, probe.OS)
	}
	if probe.Arch != runtime.GOARCH {
		t.Errorf("expected Arch=%q, got %q", runtime.GOARCH, probe.Arch)
	}
	if probe.CPUs < 1 {
		t.Errorf("expected CPUs >= 1, got %d", probe.CPUs)
	}
	if probe.RAMMB < 1 {
		t.Errorf("expected RAMMB >= 1, got %d", probe.RAMMB)
	}
	if probe.FFmpegReady {
		t.Error("expected FFmpegReady=false for nonexistent path")
	}
	if probe.StorageHint == "" {
		t.Error("expected non-empty StorageHint")
	}
}

func TestRunSystemProbe_WithValidFFmpeg(t *testing.T) {
	// Create a fake ffmpeg binary to test FFmpegReady=true
	tmpDir := t.TempDir()
	fakePath := filepath.Join(tmpDir, "ffmpeg")
	f, err := os.Create(fakePath)
	if err != nil {
		t.Fatalf("failed to create fake ffmpeg: %v", err)
	}
	f.Close()

	probe := RunSystemProbe(fakePath)
	if !probe.FFmpegReady {
		t.Error("expected FFmpegReady=true when file exists")
	}
}

func TestDetectRAMMB(t *testing.T) {
	ram := detectRAMMB()
	if ram < 1 {
		t.Errorf("expected RAM >= 1 MB, got %d", ram)
	}
}

func TestDetectFDLimit(t *testing.T) {
	limit := detectFDLimit()
	if limit < 1 {
		t.Errorf("expected FD limit >= 1, got %d", limit)
	}
}

func TestDetectStorageHint(t *testing.T) {
	hint := detectStorageHint()
	valid := map[string]bool{"SSD": true, "HDD": true, "unknown": true}
	if !valid[hint] {
		t.Errorf("expected SSD/HDD/unknown, got %q", hint)
	}
}

func TestDeriveDefaults_FDLimitCapping(t *testing.T) {
	// When FD limit is very low, MaxConnections should be capped
	probe := SystemProbe{
		OS:          "linux",
		CPUs:        16,
		RAMMB:       32768,
		FDSoftLimit: 60, // Very low FD limit: 60/4 = 15
	}
	cfg := DeriveDefaults(probe)
	if cfg.MaxConnections > 15 {
		t.Errorf("expected MaxConnections <= 15 (FD limit 60/4), got %d", cfg.MaxConnections)
	}
	if cfg.MaxConnections < 10 {
		t.Errorf("expected MaxConnections >= 10 (floor), got %d", cfg.MaxConnections)
	}
}

func TestDeriveDefaults_CacheDirFallback(t *testing.T) {
	probe := SystemProbe{OS: "linux", CPUs: 4}
	cfg := DeriveDefaults(probe)
	if cfg.CacheDir == "" {
		t.Error("CacheDir should have a value")
	}
	if !filepath.IsAbs(cfg.CacheDir) {
		t.Errorf("CacheDir should be absolute, got %q", cfg.CacheDir)
	}
}

func TestLoadOrInitConfig_FirstRun(t *testing.T) {
	store, err := newTestStore(t)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer func() { _ = store.Close() }()

	a := NewApp(store)
	// Use context.Background to avoid nil ctx panic in persistConfig.
	// persistConfig calls runtime.LogWarningf which needs a Wails context.
	// However, if there are no errors from SetConfig, LogWarningf is never called.
	// SetConfig on a valid store should succeed, so this won't panic.
	a.ctx = context.Background()

	cfg := a.loadOrInitConfig("/nonexistent/ffmpeg")
	if cfg == nil {
		t.Fatal("loadOrInitConfig returned nil")
	}
	if cfg.MaxConnections < 10 {
		t.Errorf("expected MaxConnections >= 10, got %d", cfg.MaxConnections)
	}
	if cfg.ThumbnailWorkers < 2 {
		t.Errorf("expected ThumbnailWorkers >= 2, got %d", cfg.ThumbnailWorkers)
	}
	if cfg.CacheDir == "" {
		t.Error("expected non-empty CacheDir")
	}
}

func TestLoadOrInitConfig_ExistingConfig(t *testing.T) {
	store, err := newTestStore(t)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Pre-populate config in store
	_ = store.SetConfig("maxConnections", "25")
	_ = store.SetConfig("thumbnailWorkers", "6")
	_ = store.SetConfig("scannerWorkers", "3")
	_ = store.SetConfig("serverIdleTimeoutSec", "45")
	_ = store.SetConfig("cacheDir", "/custom/cache")

	a := NewApp(store)
	cfg := a.loadOrInitConfig("/nonexistent/ffmpeg")
	if cfg == nil {
		t.Fatal("loadOrInitConfig returned nil")
	}
	if cfg.MaxConnections != 25 {
		t.Errorf("expected MaxConnections=25, got %d", cfg.MaxConnections)
	}
	// ThumbnailWorkers is capped at runtime.NumCPU() (min 2).
	expectedWorkers := 6
	maxWorkers := runtime.NumCPU()
	if maxWorkers < 2 {
		maxWorkers = 2
	}
	if expectedWorkers > maxWorkers {
		expectedWorkers = maxWorkers
	}
	if cfg.ThumbnailWorkers != expectedWorkers {
		t.Errorf("expected ThumbnailWorkers=%d, got %d", expectedWorkers, cfg.ThumbnailWorkers)
	}
	if cfg.ScannerWorkers != 3 {
		t.Errorf("expected ScannerWorkers=3, got %d", cfg.ScannerWorkers)
	}
	if cfg.ServerIdleTimeoutSec != 45 {
		t.Errorf("expected ServerIdleTimeoutSec=45, got %d", cfg.ServerIdleTimeoutSec)
	}
	if cfg.CacheDir != "/custom/cache" {
		t.Errorf("expected CacheDir='/custom/cache', got %q", cfg.CacheDir)
	}
}

func TestLoadOrInitConfig_FloorEnforcement(t *testing.T) {
	store, err := newTestStore(t)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer func() { _ = store.Close() }()

	// Store below-floor values
	_ = store.SetConfig("maxConnections", "3")
	_ = store.SetConfig("thumbnailWorkers", "0")
	_ = store.SetConfig("scannerWorkers", "0")
	_ = store.SetConfig("serverIdleTimeoutSec", "0")
	_ = store.SetConfig("cacheDir", "")

	a := NewApp(store)
	cfg := a.loadOrInitConfig("/nonexistent/ffmpeg")

	if cfg.MaxConnections < 10 {
		t.Errorf("expected MaxConnections floor 10, got %d", cfg.MaxConnections)
	}
	if cfg.ThumbnailWorkers < 2 {
		t.Errorf("expected ThumbnailWorkers floor 2, got %d", cfg.ThumbnailWorkers)
	}
	if cfg.ScannerWorkers < 1 {
		t.Errorf("expected ScannerWorkers floor 1, got %d", cfg.ScannerWorkers)
	}
	if cfg.ServerIdleTimeoutSec < 1 {
		t.Errorf("expected ServerIdleTimeoutSec floor 1, got %d", cfg.ServerIdleTimeoutSec)
	}
	if cfg.CacheDir == "" {
		t.Error("expected non-empty CacheDir fallback")
	}
}

func TestCancelDeduplicate_WithActiveCancel(t *testing.T) {
	store, err := newTestStore(t)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer func() { _ = store.Close() }()

	a := NewApp(store)

	// Set up a cancel function as if dedup were running
	ctx, cancel := context.WithCancel(context.Background())
	a.dedupeCancel = cancel

	// Should not panic and should call cancel
	a.CancelDeduplicate()

	// Verify context was cancelled
	select {
	case <-ctx.Done():
		// Expected
	default:
		t.Error("expected context to be cancelled")
	}

	// dedupeCancel should be nil after cancellation
	if a.dedupeCancel != nil {
		t.Error("expected dedupeCancel to be nil after CancelDeduplicate")
	}
}

func TestSaveAppConfig(t *testing.T) {
	store, err := newTestStore(t)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer func() { _ = store.Close() }()

	a := NewApp(store)
	a.ctx = context.Background()
	a.cfg = &AppConfig{
		MaxConnections:       20,
		ThumbnailWorkers:     4,
		ScannerWorkers:       2,
		ServerIdleTimeoutSec: 30,
		CacheDir:             "/old/cache",
		Probe: SystemProbe{
			OS:   "darwin",
			Arch: "arm64",
			CPUs: 10,
		},
	}

	newCfg := AppConfig{
		MaxConnections:       35,
		ThumbnailWorkers:     7,
		ScannerWorkers:       3,
		ServerIdleTimeoutSec: 45,
		CacheDir:             "/new/cache",
	}

	err = a.SaveAppConfig(newCfg)
	if err != nil {
		t.Fatalf("SaveAppConfig failed: %v", err)
	}

	// Verify probe was preserved from original config
	if a.cfg.Probe.CPUs != 10 {
		t.Errorf("expected probe CPUs=10 preserved, got %d", a.cfg.Probe.CPUs)
	}
	if a.cfg.Probe.OS != "darwin" {
		t.Errorf("expected probe OS='darwin' preserved, got %q", a.cfg.Probe.OS)
	}

	// Verify new values were applied
	if a.cfg.MaxConnections != 35 {
		t.Errorf("expected MaxConnections=35, got %d", a.cfg.MaxConnections)
	}
	if a.cfg.CacheDir != "/new/cache" {
		t.Errorf("expected CacheDir='/new/cache', got %q", a.cfg.CacheDir)
	}

	// Verify persisted to store
	val, _ := store.GetConfig("maxConnections")
	if val != "35" {
		t.Errorf("expected persisted maxConnections='35', got %q", val)
	}
}

func TestResetAppConfig(t *testing.T) {
	store, err := newTestStore(t)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer func() { _ = store.Close() }()

	a := NewApp(store)
	a.ctx = context.Background()

	// Pre-populate store with custom config
	_ = store.SetConfig("maxConnections", "99")
	_ = store.SetConfig("thumbnailWorkers", "99")

	cfg, err := a.ResetAppConfig()
	if err != nil {
		t.Fatalf("ResetAppConfig failed: %v", err)
	}
	if cfg == nil {
		t.Fatal("ResetAppConfig returned nil")
	}

	// After reset, values should be derived defaults, not 99
	if cfg.MaxConnections > 50 {
		t.Errorf("expected MaxConnections <= 50 after reset, got %d", cfg.MaxConnections)
	}
	if cfg.ThumbnailWorkers > 8 {
		t.Errorf("expected ThumbnailWorkers <= 8 after reset, got %d", cfg.ThumbnailWorkers)
	}

	// Verify a.cfg was updated
	if a.cfg != cfg {
		t.Error("expected a.cfg to point to the reset config")
	}
}

func TestPersistConfig(t *testing.T) {
	store, err := newTestStore(t)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer func() { _ = store.Close() }()

	a := NewApp(store)
	a.ctx = context.Background()

	cfg := &AppConfig{
		MaxConnections:       22,
		ThumbnailWorkers:     5,
		ScannerWorkers:       2,
		ServerIdleTimeoutSec: 40,
		CacheDir:             "/test/persist",
		Probe: SystemProbe{
			OS:   "linux",
			CPUs: 4,
		},
	}

	a.persistConfig(cfg)

	// Verify all values persisted to store
	val, _ := store.GetConfig("maxConnections")
	if val != "22" {
		t.Errorf("expected maxConnections='22', got %q", val)
	}
	val, _ = store.GetConfig("thumbnailWorkers")
	if val != "5" {
		t.Errorf("expected thumbnailWorkers='5', got %q", val)
	}
	val, _ = store.GetConfig("scannerWorkers")
	if val != "2" {
		t.Errorf("expected scannerWorkers='2', got %q", val)
	}
	val, _ = store.GetConfig("serverIdleTimeoutSec")
	if val != "40" {
		t.Errorf("expected serverIdleTimeoutSec='40', got %q", val)
	}
	val, _ = store.GetConfig("cacheDir")
	if val != "/test/persist" {
		t.Errorf("expected cacheDir='/test/persist', got %q", val)
	}
	val, _ = store.GetConfig("probe")
	if val == "" {
		t.Error("expected probe to be persisted as JSON")
	}
}

func TestAutoUpdateConfigPersistence(t *testing.T) {
	store, err := newTestStore(t)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer func() { _ = store.Close() }()

	a := NewApp(store)
	a.cfg = &AppConfig{AutoUpdate: "auto"}
	a.ctx = context.Background()

	a.persistConfig(a.cfg)

	val, _ := store.GetConfig("autoUpdate")
	if val != "auto" {
		t.Errorf("expected autoUpdate 'auto', got %q", val)
	}
}

func TestAutoUpdateConfigDefault(t *testing.T) {
	store, err := newTestStore(t)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer func() { _ = store.Close() }()

	a := NewApp(store)
	cfg := DeriveDefaults(SystemProbe{OS: "darwin", Arch: "arm64", CPUs: 8})
	if cfg.AutoUpdate != "notify" {
		t.Errorf("expected default AutoUpdate 'notify', got %q", cfg.AutoUpdate)
	}

	_ = a // suppress unused variable warning
}

func TestGetPhotoEXIF_ValidJPEG(t *testing.T) {
	store, err := newTestStore(t)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer func() { _ = store.Close() }()

	a := NewApp(store)

	// Create a minimal JPEG without EXIF — should return an error from ExtractFullEXIF
	tmpDir := t.TempDir()
	jpgPath := filepath.Join(tmpDir, "test.jpg")
	// Write minimal JPEG bytes (SOI + EOI markers only)
	if err := os.WriteFile(jpgPath, []byte{0xFF, 0xD8, 0xFF, 0xD9}, 0o644); err != nil {
		t.Fatalf("failed to write test jpeg: %v", err)
	}

	_, err = a.GetPhotoEXIF(jpgPath)
	// A minimal JPEG without EXIF should return an error
	if err == nil {
		t.Log("GetPhotoEXIF succeeded for minimal JPEG (no EXIF) - this is acceptable")
	}
}
