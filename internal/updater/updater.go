package updater

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"cullsnap/internal/logger"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	repoSlug      = "Abhishekmitra-slg/CullSnap"
	checkInterval = 6 * time.Hour
	cooldownSec   = 60
)

// State represents the update lifecycle state machine.
type State int

const (
	StateIdle State = iota
	StateChecking
	StateDownloading
	StateReady
	StateError
)

// Updater manages self-update checks and downloads for CullSnap.
type Updater struct {
	ctx            context.Context
	currentVersion string
	publicKey      []byte
	mode           string // "off", "notify", "auto"
	mu             sync.Mutex
	state          State
	latestRelease  *selfupdate.Release
	isHomebrew     bool
	shouldRun      bool
	lastCheck      time.Time
	config         selfupdate.Config // configured with validator + filters
}

// NewUpdater constructs an Updater. ctx may be nil during testing.
func NewUpdater(ctx context.Context, version string, publicKey []byte, mode string) *Updater {
	u := &Updater{
		ctx:            ctx,
		currentVersion: version,
		publicKey:      publicKey,
		mode:           mode,
		state:          StateIdle,
	}

	if !isValidSemver(version) {
		logger.Log.Info("Skipping update checks: dev build", "version", version)
		u.shouldRun = false
		return u
	}

	if mode == "off" {
		u.shouldRun = false
		return u
	}

	// Configure ECDSA validator + asset filters.
	u.config = selfupdate.Config{
		Filters:       []string{"CullSnap-"},
		UniversalArch: "universal", // fallback to CullSnap-darwin-universal on macOS
	}
	if len(publicKey) > 0 {
		ecdsaKey, err := parseECDSAPublicKey(publicKey)
		if err != nil {
			logger.Log.Error("Failed to parse ECDSA public key", "error", err)
		} else {
			u.config.Validator = &selfupdate.ECDSAValidator{
				PublicKey: ecdsaKey,
			}
		}
	}

	// Detect Homebrew installation by resolving the executable path.
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			u.isHomebrew = isHomebrewPath(resolved)
		}
	}

	u.shouldRun = true
	logger.Log.Info("Updater initialized", "version", version, "mode", mode, "isHomebrew", u.isHomebrew, "hasValidator", u.config.Validator != nil)
	return u
}

// GetState returns the current update state, safe for concurrent use.
func (u *Updater) GetState() State {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state
}

// IsHomebrew reports whether the running binary was installed via Homebrew.
func (u *Updater) IsHomebrew() bool {
	return u.isHomebrew
}

// canTransitionTo reports whether the state machine permits a transition from
// the current state to target. Caller must hold u.mu.
func (u *Updater) canTransitionTo(target State) bool {
	switch u.state {
	case StateIdle:
		return target == StateChecking || target == StateDownloading
	case StateChecking:
		return target == StateIdle || target == StateDownloading || target == StateError
	case StateDownloading:
		return target == StateReady || target == StateError
	case StateReady:
		return target == StateIdle
	case StateError:
		return target == StateIdle || target == StateChecking
	}
	return false
}

// isHomebrewPath reports whether the given executable path is inside a
// Homebrew Caskroom installation.
func isHomebrewPath(path string) bool {
	if path == "" {
		return false
	}
	return strings.Contains(path, "/Caskroom/cullsnap/") ||
		strings.Contains(path, "/homebrew/")
}

// isValidSemver returns true for versions that look like semantic version
// strings (with or without a leading "v"), e.g. "v1.2.3" or "1.2.3".
// "dev", empty strings, and non-numeric tokens are rejected.
func isValidSemver(version string) bool {
	if version == "" || version == "dev" {
		return false
	}
	v := strings.TrimPrefix(version, "v")
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				if c == '-' {
					break
				}
				return false
			}
		}
	}
	return true
}

// parseECDSAPublicKey decodes a PEM-encoded PKIX public key and returns the
// ECDSA key it contains.
func parseECDSAPublicKey(pemData []byte) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in public key")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}
	ecdsaKey, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not ECDSA")
	}
	return ecdsaKey, nil
}

// Start begins the background update check loop if update checks are enabled.
func (u *Updater) Start() {
	if !u.shouldRun {
		return
	}
	go u.backgroundLoop()
}

// backgroundLoop runs an immediate check then rechecks on each interval tick
// until the context is cancelled.
func (u *Updater) backgroundLoop() {
	u.check()

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			u.check()
		case <-u.ctx.Done():
			return
		}
	}
}

// CheckNow triggers an immediate update check. It returns an error if update
// checks are disabled or if the cooldown period has not elapsed since the last
// check.
func (u *Updater) CheckNow() error {
	if !u.shouldRun {
		return fmt.Errorf("update checks disabled")
	}

	u.mu.Lock()
	elapsed := time.Since(u.lastCheck)
	u.mu.Unlock()

	if elapsed < time.Duration(cooldownSec)*time.Second {
		return fmt.Errorf("cooldown: please wait %d seconds", cooldownSec-int(elapsed.Seconds()))
	}

	go u.check()
	return nil
}

// check performs a single update check: queries GitHub for the latest release,
// compares it to the running version, and emits Wails events as appropriate.
func (u *Updater) check() {
	u.mu.Lock()
	if !u.canTransitionTo(StateChecking) {
		u.mu.Unlock()
		return
	}
	u.state = StateChecking
	u.lastCheck = time.Now()
	u.mu.Unlock()

	logger.Log.Info("Update check starting", "currentVersion", u.currentVersion)

	suUpdater, err := selfupdate.NewUpdater(u.config)
	if err != nil {
		u.handleError(fmt.Sprintf("Failed to create updater: %v", err))
		return
	}

	latest, found, err := suUpdater.DetectLatest(u.ctx, selfupdate.ParseSlug(repoSlug))
	logger.Log.Info("DetectLatest result", "found", found, "err", err)
	if err != nil {
		u.handleError(fmt.Sprintf("Update check failed: %v", err))
		return
	}

	if !found {
		u.mu.Lock()
		u.state = StateIdle
		u.mu.Unlock()
		return
	}

	v := strings.TrimPrefix(u.currentVersion, "v")
	if !latest.GreaterThan(v) {
		logger.Log.Info("Already up to date", "current", u.currentVersion, "latest", latest.Version())
		u.mu.Lock()
		u.state = StateIdle
		u.mu.Unlock()
		return
	}

	logger.Log.Info("Update available", "current", u.currentVersion, "latest", latest.Version())
	u.mu.Lock()
	u.latestRelease = latest
	u.mu.Unlock()

	releaseURL := fmt.Sprintf("https://github.com/%s/releases/tag/v%s", repoSlug, latest.Version())
	if u.ctx != nil {
		runtime.EventsEmit(u.ctx, "update:available", map[string]interface{}{
			"version":    latest.Version(),
			"releaseURL": releaseURL,
			"homebrew":   u.isHomebrew,
		})
	}

	// In auto mode (non-Homebrew), proceed to download immediately.
	if u.mode == "auto" && !u.isHomebrew {
		u.download()
	} else {
		u.mu.Lock()
		u.state = StateIdle
		u.mu.Unlock()
	}
}

// handleError logs an error, transitions the state machine to StateError, and
// emits a Wails event if a context is available.
func (u *Updater) handleError(msg string) {
	logger.Log.Error("Update error", "message", msg)
	u.mu.Lock()
	u.state = StateError
	u.mu.Unlock()
	if u.ctx != nil {
		runtime.EventsEmit(u.ctx, "update:error", map[string]string{"message": msg})
	}
}

// DownloadUpdate initiates a background download and application of the
// latest release. It returns an error immediately if the install is managed
// by Homebrew or if no update has been cached from a previous check.
func (u *Updater) DownloadUpdate() error {
	if u.isHomebrew {
		return fmt.Errorf("homebrew-managed install: use 'brew upgrade cullsnap' instead")
	}

	u.mu.Lock()
	release := u.latestRelease
	u.mu.Unlock()

	if release == nil {
		return fmt.Errorf("no update available to download")
	}

	go u.download()
	return nil
}

// download performs the actual binary replacement using go-selfupdate. It
// must be called from a goroutine; state transitions and event emission are
// handled internally.
func (u *Updater) download() {
	u.mu.Lock()
	if !u.canTransitionTo(StateDownloading) {
		u.mu.Unlock()
		return
	}
	u.state = StateDownloading
	release := u.latestRelease
	u.mu.Unlock()

	if u.ctx != nil {
		runtime.EventsEmit(u.ctx, "update:downloading", map[string]string{
			"version": release.Version(),
		})
	}

	exe, err := os.Executable()
	if err != nil {
		u.handleError(fmt.Sprintf("Failed to locate executable: %v", err))
		return
	}

	suUpdater, err := selfupdate.NewUpdater(u.config)
	if err != nil {
		u.handleError(fmt.Sprintf("Failed to create updater: %v", err))
		return
	}

	err = suUpdater.UpdateTo(u.ctx, release, exe)
	if err != nil {
		u.handleError(fmt.Sprintf("Update failed: %v", err))
		return
	}

	logger.Log.Info("Update applied", "version", release.Version())

	u.mu.Lock()
	u.state = StateReady
	u.mu.Unlock()

	if u.ctx != nil {
		runtime.EventsEmit(u.ctx, "update:ready", map[string]string{
			"version": release.Version(),
		})
	}
}

// RestartForUpdate launches a new instance of the running binary and then
// quits the current process via the Wails runtime.
//
// The executable path is obtained from os.Executable (the running process
// itself), resolved through any symlinks, and validated to be absolute before
// use — it is never derived from user input.
func (u *Updater) RestartForUpdate() error {
	raw, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate executable: %w", err)
	}

	// Resolve symlinks so we always exec the real binary, not a wrapper.
	exe, err := filepath.EvalSymlinks(raw)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Require an absolute path as a safety invariant — os.Executable should
	// always return one, but reject anything unexpected before passing it to
	// exec.Command.
	if !filepath.IsAbs(exe) {
		return fmt.Errorf("executable path is not absolute: %q", exe)
	}

	// exe is the resolved, absolute path of the running process binary
	// obtained from os.Executable — it is never user-supplied input.
	cmd := exec.Command(exe) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to restart: %w", err)
	}

	if u.ctx != nil {
		runtime.Quit(u.ctx)
	}
	return nil
}
