package updater

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cullsnap/internal/logger"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	repoSlug      = "Abhishekmitra-slg/CullSnap"
	checkInterval = 6 * time.Hour
	cooldownSec   = 60
)

// Ensure unused imports are referenced to satisfy the compiler during
// incremental development; they are consumed in later tasks.
var (
	_ = exec.Command
	_ = runtime.EventsEmit
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
		Filters: []string{"CullSnap-"},
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
