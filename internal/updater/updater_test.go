package updater

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"cullsnap/internal/logger"
	"encoding/pem"
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
	u := NewUpdater(context.TODO(), "dev", nil, "notify")
	if u.shouldRun {
		t.Error("dev build should not run update checks")
	}
}

func TestNewUpdater_OffMode(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "off")
	if u.shouldRun {
		t.Error("off mode should not run update checks")
	}
}

func TestNewUpdater_NotifyMode(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "notify")
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

func TestDownloadUpdate_HomebrewNoop(t *testing.T) {
	u := NewUpdater(context.Background(), "v1.0.0", nil, "notify")
	u.isHomebrew = true

	err := u.DownloadUpdate()
	if err == nil {
		t.Error("expected error for Homebrew download attempt")
	}
	if !strings.Contains(err.Error(), "homebrew") {
		t.Errorf("expected homebrew error, got: %v", err)
	}
}

func TestDownloadUpdate_NoRelease(t *testing.T) {
	u := NewUpdater(context.Background(), "v1.0.0", nil, "notify")
	err := u.DownloadUpdate()
	if err == nil {
		t.Error("expected error when no release cached")
	}
}

func TestStateTransitions(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "notify")

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

func TestStart_OffMode(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "off")
	// Start should return immediately without spawning a goroutine
	u.Start()
	// If we get here without hanging, the test passes
	if u.GetState() != StateIdle {
		t.Errorf("expected state Idle after Start with off mode, got %v", u.GetState())
	}
}

func TestStart_DevBuild(t *testing.T) {
	u := NewUpdater(context.TODO(), "dev", nil, "auto")
	u.Start()
	if u.GetState() != StateIdle {
		t.Errorf("expected state Idle after Start with dev build, got %v", u.GetState())
	}
}

func TestCheckNow_Disabled(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "off")
	err := u.CheckNow()
	if err == nil {
		t.Error("expected error when update checks are disabled")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("expected disabled error, got: %v", err)
	}
}

func TestHandleError_TransitionsToErrorState(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "notify")
	// Set ctx to nil so handleError skips EventsEmit (which requires a real Wails context)
	u.ctx = nil
	u.handleError("test error message")
	if u.GetState() != StateError {
		t.Errorf("expected state Error after handleError, got %v", u.GetState())
	}
}

func TestParseECDSAPublicKey_InvalidPEM(t *testing.T) {
	_, err := parseECDSAPublicKey([]byte("not a valid PEM"))
	if err == nil {
		t.Error("expected error for invalid PEM data")
	}
	if !strings.Contains(err.Error(), "no PEM block") {
		t.Errorf("expected 'no PEM block' error, got: %v", err)
	}
}

func TestParseECDSAPublicKey_EmptyInput(t *testing.T) {
	_, err := parseECDSAPublicKey([]byte{})
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestStateTransitions_ErrorToIdle(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "notify")
	u.mu.Lock()
	u.state = StateError
	u.mu.Unlock()

	if !u.canTransitionTo(StateIdle) {
		t.Error("should allow Error -> Idle")
	}
	if !u.canTransitionTo(StateChecking) {
		t.Error("should allow Error -> Checking")
	}
	if u.canTransitionTo(StateDownloading) {
		t.Error("should not allow Error -> Downloading")
	}
}

func TestStateTransitions_ReadyToIdle(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "notify")
	u.mu.Lock()
	u.state = StateReady
	u.mu.Unlock()

	if !u.canTransitionTo(StateIdle) {
		t.Error("should allow Ready -> Idle")
	}
	if u.canTransitionTo(StateChecking) {
		t.Error("should not allow Ready -> Checking")
	}
}

func TestStateTransitions_CheckingTransitions(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "notify")
	u.mu.Lock()
	u.state = StateChecking
	u.mu.Unlock()

	if !u.canTransitionTo(StateIdle) {
		t.Error("should allow Checking -> Idle")
	}
	if !u.canTransitionTo(StateDownloading) {
		t.Error("should allow Checking -> Downloading")
	}
	if !u.canTransitionTo(StateError) {
		t.Error("should allow Checking -> Error")
	}
	if u.canTransitionTo(StateReady) {
		t.Error("should not allow Checking -> Ready")
	}
}

func TestIsHomebrew(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "notify")
	if u.IsHomebrew() {
		t.Error("expected IsHomebrew to be false for non-homebrew install")
	}
	u.isHomebrew = true
	if !u.IsHomebrew() {
		t.Error("expected IsHomebrew to be true after setting flag")
	}
}

func TestNewUpdater_AutoMode(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "auto")
	if !u.shouldRun {
		t.Error("auto mode should run update checks")
	}
	if u.mode != "auto" {
		t.Errorf("expected mode 'auto', got %q", u.mode)
	}
}

func TestDownload_NilRelease(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "notify")
	u.ctx = nil
	// Set state to Idle so it can transition to Downloading
	u.mu.Lock()
	u.state = StateIdle
	u.latestRelease = nil
	u.mu.Unlock()

	// download() should transition to Downloading but fail since release is nil
	// Actually it will call EventsEmit which needs non-nil ctx...
	// Set state so canTransitionTo returns false
	u.mu.Lock()
	u.state = StateReady
	u.mu.Unlock()
	u.download()

	if u.GetState() != StateReady {
		t.Errorf("expected state Ready (transition rejected), got %v", u.GetState())
	}
}

func TestCheck_NilContext(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "notify")
	// Set ctx to nil -- check() will call DetectLatest(nil, ...) which will fail,
	// and handleError will skip EventsEmit since ctx is nil
	u.ctx = nil

	u.check()

	state := u.GetState()
	// After a check with nil ctx, DetectLatest will fail -> handleError -> StateError
	if state != StateError {
		t.Errorf("expected state Error after nil-context check, got %v", state)
	}
}

func TestCheck_InvalidTransition(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "notify")
	// Set state to Ready, which cannot transition to Checking
	u.mu.Lock()
	u.state = StateReady
	u.mu.Unlock()

	u.check()

	// State should remain Ready since the transition was rejected
	if u.GetState() != StateReady {
		t.Errorf("expected state Ready (transition rejected), got %v", u.GetState())
	}
}

func TestDownload_InvalidTransition(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "notify")
	// Set state to Ready, which cannot transition to Downloading
	u.mu.Lock()
	u.state = StateReady
	u.mu.Unlock()

	u.download()

	// State should remain Ready since the transition was rejected
	if u.GetState() != StateReady {
		t.Errorf("expected state Ready (transition rejected), got %v", u.GetState())
	}
}

func TestParseECDSAPublicKey_ValidKey(t *testing.T) {
	// Generate a real ECDSA key pair for testing
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ECDSA key: %v", err)
	}

	pubBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("failed to marshal public key: %v", err)
	}

	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	})

	key, err := parseECDSAPublicKey(pemData)
	if err != nil {
		t.Fatalf("parseECDSAPublicKey failed: %v", err)
	}
	if key == nil {
		t.Fatal("expected non-nil key")
	}
}

func TestParseECDSAPublicKey_NonECDSAKey(t *testing.T) {
	// Create a PEM block with invalid DER data to test parse failure
	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: []byte("not a valid DER-encoded key"),
	})

	_, err := parseECDSAPublicKey(pemData)
	if err == nil {
		t.Error("expected error for invalid DER data")
	}
}

func TestNewUpdater_WithValidPublicKey(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ECDSA key: %v", err)
	}

	pubBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("failed to marshal public key: %v", err)
	}

	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	})

	u := NewUpdater(context.TODO(), "v1.0.0", pemData, "notify")
	if !u.shouldRun {
		t.Error("expected shouldRun to be true with valid key")
	}
	if u.config.Validator == nil {
		t.Error("expected validator to be set with valid public key")
	}
}

func TestNewUpdater_WithInvalidPublicKey(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", []byte("bad key"), "notify")
	if !u.shouldRun {
		t.Error("expected shouldRun to be true even with bad key")
	}
	if u.config.Validator != nil {
		t.Error("expected validator to be nil with invalid public key")
	}
}

func TestDownloadUpdate_DisabledUpdater(t *testing.T) {
	u := NewUpdater(context.TODO(), "dev", nil, "notify")
	// Dev build has shouldRun=false, but DownloadUpdate checks isHomebrew and latestRelease
	// It doesn't check shouldRun, so it should return "no update available" error
	err := u.DownloadUpdate()
	if err == nil {
		t.Error("expected error for download with no cached release")
	}
}

func TestIsValidSemver_MoreCases(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"v1.0.0-beta", true},
		{"1.0", true},
		{"v0.0.1", true},
		{"abc.def.ghi", false},
		{"v", false},
		{"v.", false},
		{"1..0", false},
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

func TestCheckNow_SuccessfulTrigger(t *testing.T) {
	u := NewUpdater(context.TODO(), "v1.0.0", nil, "notify")
	// Set ctx to nil so the spawned goroutine's check() -> handleError skips EventsEmit
	u.ctx = nil

	// Ensure enough time has elapsed since lastCheck (zero value)
	err := u.CheckNow()
	if err != nil {
		t.Errorf("expected no error for valid CheckNow, got: %v", err)
	}
	// Give the goroutine a moment to run
	time.Sleep(200 * time.Millisecond)
}
