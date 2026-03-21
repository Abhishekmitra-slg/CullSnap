package updater

import (
	"context"
	"cullsnap/internal/logger"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	tmpDir, _ := os.MkdirTemp("", "cullsnap-updater-test-*")
	logPath := filepath.Join(tmpDir, "test.log")
	_ = logger.Init(logPath)
	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestIsHomebrewPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"Apple Silicon Homebrew", "/opt/homebrew/Caskroom/cullsnap/2.2.0/CullSnap.app/Contents/MacOS/CullSnap", true},
		{"Intel Homebrew", "/usr/local/Caskroom/cullsnap/2.2.0/CullSnap.app/Contents/MacOS/CullSnap", true},
		{"Direct install", "/Applications/CullSnap.app/Contents/MacOS/CullSnap", false},
		{"Linux binary", "/usr/local/bin/CullSnap", false},
		{"Windows", `C:\Users\test\CullSnap.exe`, false},
		{"Empty path", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHomebrewPath(tt.path)
			if got != tt.want {
				t.Errorf("isHomebrewPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsValidSemver(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"v1.0.0", true},
		{"v2.2.0", true},
		{"1.0.0", true},
		{"dev", false},
		{"", false},
		{"latest", false},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := isValidSemver(tt.version)
			if got != tt.want {
				t.Errorf("isValidSemver(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestNewUpdater_DevBuild(t *testing.T) {
	u := NewUpdater(nil, "dev", nil, "notify")
	if u.shouldRun {
		t.Error("dev build should not run update checks")
	}
}

func TestNewUpdater_OffMode(t *testing.T) {
	u := NewUpdater(nil, "v1.0.0", nil, "off")
	if u.shouldRun {
		t.Error("off mode should not run update checks")
	}
}

func TestNewUpdater_NotifyMode(t *testing.T) {
	u := NewUpdater(nil, "v1.0.0", nil, "notify")
	if !u.shouldRun {
		t.Error("notify mode should run update checks")
	}
	if u.mode != "notify" {
		t.Errorf("expected mode 'notify', got %q", u.mode)
	}
}

func TestCheckNow_Cooldown(t *testing.T) {
	u := NewUpdater(context.Background(), "v1.0.0", nil, "notify")
	u.lastCheck = time.Now()

	err := u.CheckNow()
	if err == nil {
		t.Error("expected cooldown error, got nil")
	}
	if !strings.Contains(err.Error(), "cooldown") {
		t.Errorf("expected cooldown error, got: %v", err)
	}
}

func TestStateTransitions(t *testing.T) {
	u := NewUpdater(nil, "v1.0.0", nil, "notify")

	if u.GetState() != StateIdle {
		t.Fatalf("expected initial state Idle, got %v", u.GetState())
	}

	// Idle can transition to Checking or Downloading
	if !u.canTransitionTo(StateChecking) {
		t.Error("should allow Idle -> Checking")
	}
	if !u.canTransitionTo(StateDownloading) {
		t.Error("should allow Idle -> Downloading")
	}

	// Set to downloading, can't download again
	u.mu.Lock()
	u.state = StateDownloading
	u.mu.Unlock()

	if u.canTransitionTo(StateDownloading) {
		t.Error("should not allow Downloading -> Downloading")
	}
	if !u.canTransitionTo(StateReady) {
		t.Error("should allow Downloading -> Ready")
	}
	if !u.canTransitionTo(StateError) {
		t.Error("should allow Downloading -> Error")
	}
}
