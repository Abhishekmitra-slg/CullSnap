# Auto-Update Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add seamless in-app auto-update with SHA256+ECDSA verification, Homebrew coexistence, and three user control modes.

**Architecture:** New `internal/updater/` package wraps `creativeprojects/go-selfupdate` to check GitHub Releases, verify checksums+signatures, and replace the binary. Frontend gets toast notifications via Wails events. Homebrew installs on macOS are detected and deferred to `brew upgrade`.

**Tech Stack:** Go 1.25, creativeprojects/go-selfupdate, Wails v2 events, React/TypeScript, ECDSA P-256

**Spec:** `docs/superpowers/specs/2026-03-20-auto-update-design.md`

---

## File Structure

### New Files
| File | Responsibility |
|------|---------------|
| `internal/updater/updater.go` | Updater struct, state machine, background check loop, Homebrew detection |
| `internal/updater/updater_test.go` | Unit tests for all updater logic |
| `keys/update_signing.pub` | ECDSA P-256 public key (generated, committed) |
| `frontend/src/components/UpdateToast.tsx` | Toast notification component (5 states) |

### Modified Files
| File | Changes |
|------|---------|
| `go.mod` | Add `creativeprojects/go-selfupdate` dependency |
| `internal/app/config.go` | Add `AutoUpdate` field to `AppConfig` struct |
| `internal/app/app.go` | Add `UpdatePublicKey` field, create Updater in Startup, expose 3 Wails-bound methods |
| `main.go` | Embed public key via `go:embed`, pass to App |
| `frontend/src/components/SettingsModal.tsx` | Add "Updates" section with dropdown |
| `frontend/src/components/AboutModal.tsx` | Add "Check for Updates" button |
| `frontend/src/App.tsx` | Render UpdateToast, listen for 4 Wails events |
| `.github/workflows/release.yml` | Add standalone macOS binary, checksums, signing |

---

## Task 1: Generate ECDSA Key Pair and Add Dependency

**Files:**
- Create: `keys/update_signing.pub`
- Modify: `go.mod`

- [ ] **Step 1: Generate ECDSA P-256 key pair**

```bash
openssl ecparam -genkey -name prime256v1 -noout -out /tmp/cullsnap_update.pem
openssl ec -in /tmp/cullsnap_update.pem -pubout -out keys/update_signing.pub
```

Save the private key content for the GitHub secret (do NOT commit it):
```bash
cat /tmp/cullsnap_update.pem
# Copy this output — you'll add it as CULLSNAP_UPDATE_SIGNING_KEY secret later
```

- [ ] **Step 2: Verify the public key file**

```bash
cat keys/update_signing.pub
```
Expected: PEM-encoded public key starting with `-----BEGIN PUBLIC KEY-----`

- [ ] **Step 3: Add go-selfupdate dependency**

```bash
go get github.com/creativeprojects/go-selfupdate@latest
```

- [ ] **Step 4: Verify dependency was added**

```bash
grep go-selfupdate go.mod
```
Expected: `github.com/creativeprojects/go-selfupdate vX.Y.Z`

- [ ] **Step 5: Commit**

```bash
git add keys/update_signing.pub go.mod go.sum
git commit -m "chore: add ECDSA public key and go-selfupdate dependency"
```

---

## Task 2: Add AutoUpdate Field to AppConfig

**Files:**
- Modify: `internal/app/config.go:23-30` (AppConfig struct)
- Modify: `internal/app/app.go:94-133` (loadOrInitConfig)
- Modify: `internal/app/app.go:135-156` (persistConfig)
- Test: `internal/app/config_extra_test.go`

- [ ] **Step 1: Write failing test for AutoUpdate config persistence**

Add to `internal/app/config_extra_test.go`:

```go
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
	// Simulate first run — no config in store
	cfg := DeriveDefaults(SystemProbe{OS: "darwin", Arch: "arm64", CPUs: 8})
	if cfg.AutoUpdate != "notify" {
		t.Errorf("expected default AutoUpdate 'notify', got %q", cfg.AutoUpdate)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/app/ -run TestAutoUpdate -v
```
Expected: FAIL — `AutoUpdate` field doesn't exist yet

- [ ] **Step 3: Add AutoUpdate field to AppConfig struct**

In `internal/app/config.go`, add to `AppConfig` struct (after `CacheDir`):

```go
AutoUpdate string `json:"autoUpdate"` // "off", "notify", "auto"
```

- [ ] **Step 4: Set default in DeriveDefaults**

In `internal/app/config.go`, inside `DeriveDefaults()`, add before `return cfg`:

```go
cfg.AutoUpdate = "notify"
```

- [ ] **Step 5: Add to loadOrInitConfig**

In `internal/app/app.go`, inside `loadOrInitConfig()`, add after the `CacheDir` loading block (around line 111):

```go
cfg.AutoUpdate, _ = a.store.GetConfig("autoUpdate")
if cfg.AutoUpdate == "" {
	cfg.AutoUpdate = "notify"
}
```

- [ ] **Step 6: Add to persistConfig**

In `internal/app/app.go`, inside `persistConfig()`, add after the `cacheDir` persist (around line 149):

```go
if err := a.store.SetConfig("autoUpdate", cfg.AutoUpdate); err != nil {
	runtime.LogWarningf(a.ctx, "persistConfig: failed to save autoUpdate: %v", err)
}
```

- [ ] **Step 7: Run tests to verify they pass**

```bash
go test ./internal/app/ -run TestAutoUpdate -v
```
Expected: PASS

- [ ] **Step 8: Run all existing tests to check for regressions**

```bash
go test ./internal/...
```
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add internal/app/config.go internal/app/app.go internal/app/config_extra_test.go
git commit -m "feat: add AutoUpdate field to AppConfig with notify default"
```

---

## Task 3: Create Updater Package — Core Struct and Homebrew Detection

**Files:**
- Create: `internal/updater/updater.go`
- Create: `internal/updater/updater_test.go`

- [ ] **Step 1: Write failing tests for Homebrew detection and state management**

Create `internal/updater/updater_test.go`:

```go
package updater

import (
	"testing"
)

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

func TestStateTransitions(t *testing.T) {
	u := NewUpdater(nil, "v1.0.0", nil, "notify")

	if u.GetState() != StateIdle {
		t.Fatalf("expected initial state Idle, got %v", u.GetState())
	}

	// Can't download from idle (no release found yet)
	u.mu.Lock()
	u.state = StateDownloading
	u.mu.Unlock()

	// Can't start another download while downloading
	if u.canTransitionTo(StateDownloading) {
		t.Error("should not allow duplicate download transition")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/updater/ -v
```
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Create updater.go with core struct, Homebrew detection, semver check**

Create `internal/updater/updater.go`:

```go
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

type State int

const (
	StateIdle State = iota
	StateChecking
	StateDownloading
	StateReady
	StateError
)

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

// NewUpdater creates an updater. Note: spec says NewUpdater(ctx, store, version, publicKey)
// but we pass mode directly from AppConfig.AutoUpdate — the Updater does not read from
// the store, keeping AppConfig as the single source of truth.
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

	// Configure ECDSA + SHA256 validator if public key is provided
	u.config = selfupdate.Config{
		Filters: []string{"CullSnap-"}, // match non-standard asset names
	}
	if len(publicKey) > 0 {
		ecdsaKey, err := parseECDSAPublicKey(publicKey)
		if err != nil {
			logger.Log.Error("Failed to parse ECDSA public key", "error", err)
		} else {
			u.config.Validator = &selfupdate.ChecksumWithECDSAValidator{
				PublicKey: ecdsaKey,
			}
		}
	}

	// Detect Homebrew installation
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			u.isHomebrew = isHomebrewPath(resolved)
		}
	}

	u.shouldRun = true
	return u
}

// parseECDSAPublicKey parses a PEM-encoded ECDSA public key.
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

func (u *Updater) GetState() State {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state
}

func (u *Updater) IsHomebrew() bool {
	return u.isHomebrew
}

func (u *Updater) canTransitionTo(target State) bool {
	// Only allowed transitions
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

func isHomebrewPath(path string) bool {
	if path == "" {
		return false
	}
	return strings.Contains(path, "/Caskroom/cullsnap/") ||
		strings.Contains(path, "/homebrew/")
}

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
				// Allow pre-release suffixes like -beta
				if c == '-' {
					break
				}
				return false
			}
		}
	}
	return true
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/updater/ -v
```
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/updater/updater.go internal/updater/updater_test.go
git commit -m "feat: create updater package with state machine and Homebrew detection"
```

---

## Task 4: Add Background Check Loop and CheckNow

**Files:**
- Modify: `internal/updater/updater.go`
- Modify: `internal/updater/updater_test.go`

- [ ] **Step 1: Write failing test for check cooldown**

Add to `internal/updater/updater_test.go`:

```go
func TestCheckNow_Cooldown(t *testing.T) {
	u := NewUpdater(context.Background(), "v1.0.0", nil, "notify")
	u.lastCheck = time.Now() // just checked

	err := u.CheckNow()
	if err == nil {
		t.Error("expected cooldown error, got nil")
	}
	if !strings.Contains(err.Error(), "cooldown") {
		t.Errorf("expected cooldown error, got: %v", err)
	}
}
```

Add `"strings"` to test imports.

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/updater/ -run TestCheckNow -v
```
Expected: FAIL — `CheckNow` doesn't exist

- [ ] **Step 3: Implement Start(), CheckNow(), and background loop**

Add to `internal/updater/updater.go`:

```go
func (u *Updater) Start() {
	if !u.shouldRun {
		return
	}
	go u.backgroundLoop()
}

func (u *Updater) backgroundLoop() {
	// Check immediately on startup
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

func (u *Updater) check() {
	u.mu.Lock()
	if !u.canTransitionTo(StateChecking) {
		u.mu.Unlock()
		return
	}
	u.state = StateChecking
	u.lastCheck = time.Now()
	u.mu.Unlock()

	// Use configured updater with ECDSA validator and asset filters
	source, err := selfupdate.NewGitHubSource(selfupdate.GitHubConfig{})
	if err != nil {
		u.handleError(fmt.Sprintf("Failed to create GitHub source: %v", err))
		return
	}
	updater, err := selfupdate.NewUpdater(u.config)
	if err != nil {
		u.handleError(fmt.Sprintf("Failed to create updater: %v", err))
		return
	}

	latest, found, err := updater.DetectLatest(u.ctx, source, selfupdate.ParseSlug(repoSlug))
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

	// In auto mode (non-Homebrew), proceed to download immediately
	if u.mode == "auto" && !u.isHomebrew {
		u.download()
	} else {
		u.mu.Lock()
		u.state = StateIdle
		u.mu.Unlock()
	}
}
```

All imports are already in the initial file from Task 3.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/updater/ -v
```
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/updater/updater.go internal/updater/updater_test.go
git commit -m "feat: add background check loop and CheckNow with cooldown"
```

---

## Task 5: Add Download and Restart Methods

**Files:**
- Modify: `internal/updater/updater.go`
- Modify: `internal/updater/updater_test.go`

- [ ] **Step 1: Write failing test for DownloadUpdate when Homebrew**

Add to `internal/updater/updater_test.go`:

```go
func TestDownloadUpdate_HomebrewNoop(t *testing.T) {
	u := NewUpdater(context.Background(), "v1.0.0", nil, "notify")
	u.isHomebrew = true

	err := u.DownloadUpdate()
	if err == nil {
		t.Error("expected error for Homebrew download attempt")
	}
	if !strings.Contains(err.Error(), "Homebrew") {
		t.Errorf("expected Homebrew error, got: %v", err)
	}
}

func TestDownloadUpdate_NoRelease(t *testing.T) {
	u := NewUpdater(context.Background(), "v1.0.0", nil, "notify")
	err := u.DownloadUpdate()
	if err == nil {
		t.Error("expected error when no release cached")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/updater/ -run TestDownloadUpdate -v
```
Expected: FAIL — `DownloadUpdate` doesn't exist

- [ ] **Step 3: Implement DownloadUpdate and RestartForUpdate**

Add to `internal/updater/updater.go`:

```go
func (u *Updater) DownloadUpdate() error {
	if u.isHomebrew {
		return fmt.Errorf("Homebrew-managed install: use 'brew upgrade cullsnap' instead")
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

	// Use configured updater (with ECDSA+SHA256 validator) for the actual update
	updater, err := selfupdate.NewUpdater(u.config)
	if err != nil {
		u.handleError(fmt.Sprintf("Failed to create updater: %v", err))
		return
	}

	err = updater.UpdateTo(u.ctx, release, exe)
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

func (u *Updater) RestartForUpdate() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate executable: %w", err)
	}

	cmd := exec.Command(exe)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to restart: %w", err)
	}

	// Quit via Wails runtime
	if u.ctx != nil {
		runtime.Quit(u.ctx)
	}
	return nil
}

func (u *Updater) handleError(msg string) {
	logger.Log.Error("Update error", "message", msg)
	u.mu.Lock()
	u.state = StateError
	u.mu.Unlock()
	if u.ctx != nil {
		runtime.EventsEmit(u.ctx, "update:error", map[string]string{"message": msg})
	}
}
```

All imports are already in the initial file from Task 3.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/updater/ -v
```
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/updater/updater.go internal/updater/updater_test.go
git commit -m "feat: add DownloadUpdate and RestartForUpdate methods"
```

---

## Task 6: Integrate Updater into App and Main

**Files:**
- Modify: `internal/app/app.go:52-64` (App struct)
- Modify: `internal/app/app.go:73-92` (Startup)
- Modify: `main.go:28-36` (embeds and vars)
- Modify: `main.go:274-279` (app setup)

- [ ] **Step 1: Add UpdatePublicKey field to App struct**

In `internal/app/app.go`, add to the `App` struct (after `ContributorsRaw`):

```go
UpdatePublicKey []byte // ECDSA public key for update signature verification
```

- [ ] **Step 2: Add updater field and import**

In `internal/app/app.go`, add to the `App` struct:

```go
updater *updater.Updater
```

Add `"cullsnap/internal/updater"` to imports.

- [ ] **Step 3: Create Updater in Startup**

In `internal/app/app.go`, inside `Startup()`, add after the `go a.emitSystemMetrics()` line:

```go
// Start auto-update checker
a.updater = updater.NewUpdater(ctx, a.Version, a.UpdatePublicKey, a.cfg.AutoUpdate)
a.updater.Start()
```

- [ ] **Step 4: Add Wails-bound methods**

Add to `internal/app/app.go`:

```go
func (a *App) CheckForUpdate() error {
	if a.updater == nil {
		return fmt.Errorf("updater not initialized")
	}
	return a.updater.CheckNow()
}

func (a *App) DownloadUpdate() error {
	if a.updater == nil {
		return fmt.Errorf("updater not initialized")
	}
	return a.updater.DownloadUpdate()
}

func (a *App) RestartForUpdate() error {
	if a.updater == nil {
		return fmt.Errorf("updater not initialized")
	}
	return a.updater.RestartForUpdate()
}
```

- [ ] **Step 5: Embed public key in main.go**

In `main.go`, add after the `contributorsYML` embed:

```go
//go:embed keys/update_signing.pub
var updatePublicKey []byte
```

- [ ] **Step 6: Pass public key to App in main.go**

In `main.go`, add after `application.ContributorsRaw = contributorsYML`:

```go
application.UpdatePublicKey = updatePublicKey
```

- [ ] **Step 7: Verify compilation**

```bash
go build ./...
```
Expected: Build succeeds

- [ ] **Step 8: Run all tests**

```bash
go test ./internal/...
```
Expected: All PASS

- [ ] **Step 9: Commit**

```bash
git add internal/app/app.go main.go
git commit -m "feat: integrate updater into App startup and expose Wails-bound methods"
```

---

## Task 7: Frontend — UpdateToast Component

**Files:**
- Create: `frontend/src/components/UpdateToast.tsx`

- [ ] **Step 1: Create UpdateToast component**

Create `frontend/src/components/UpdateToast.tsx`:

```tsx
import { useState, useEffect } from 'react';
import { X, Download, RotateCcw, AlertTriangle, Loader } from 'lucide-react';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
import { DownloadUpdate, RestartForUpdate } from '../../wailsjs/go/app/App';

type UpdateState =
    | { type: 'hidden' }
    | { type: 'available'; version: string; releaseURL: string; homebrew: boolean }
    | { type: 'downloading'; version: string }
    | { type: 'ready'; version: string }
    | { type: 'error'; message: string };

export function UpdateToast() {
    const [state, setState] = useState<UpdateState>({ type: 'hidden' });
    const [dismissed, setDismissed] = useState(false);

    useEffect(() => {
        EventsOn('update:available', (data: any) => {
            setState({ type: 'available', version: data.version, releaseURL: data.releaseURL, homebrew: data.homebrew });
            setDismissed(false);
        });
        EventsOn('update:downloading', (data: any) => {
            setState({ type: 'downloading', version: data.version });
            setDismissed(false);
        });
        EventsOn('update:ready', (data: any) => {
            setState({ type: 'ready', version: data.version });
            setDismissed(false);
        });
        EventsOn('update:error', (data: any) => {
            setState({ type: 'error', message: data.message });
            setDismissed(false);
        });

        return () => {
            EventsOff('update:available');
            EventsOff('update:downloading');
            EventsOff('update:ready');
            EventsOff('update:error');
        };
    }, []);

    if (state.type === 'hidden' || dismissed) return null;

    const handleDownload = async () => {
        try {
            await DownloadUpdate();
        } catch (e) {
            console.error('Download failed:', e);
        }
    };

    const handleRestart = async () => {
        try {
            await RestartForUpdate();
        } catch (e) {
            console.error('Restart failed:', e);
        }
    };

    const borderColor = state.type === 'error'
        ? 'rgba(239,68,68,0.3)'
        : state.type === 'downloading'
            ? 'rgba(59,130,246,0.3)'
            : 'rgba(34,197,94,0.3)';

    const iconColor = state.type === 'error'
        ? '#ef4444'
        : state.type === 'downloading'
            ? '#3b82f6'
            : '#22c55e';

    return (
        <div style={{
            position: 'fixed',
            bottom: 40,
            right: 16,
            zIndex: 1000,
            background: 'linear-gradient(135deg, rgba(24,24,27,0.97), rgba(39,39,42,0.97))',
            border: `1px solid ${borderColor}`,
            borderRadius: 10,
            padding: '14px 16px',
            boxShadow: '0 4px 12px rgba(0,0,0,0.4)',
            maxWidth: 340,
            minWidth: 280,
        }}>
            <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12 }}>
                <div style={{ color: iconColor, marginTop: 2 }}>
                    {state.type === 'error' && <AlertTriangle size={18} />}
                    {state.type === 'downloading' && <Loader size={18} className="spin" />}
                    {(state.type === 'available' || state.type === 'ready') && <Download size={18} />}
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                    {state.type === 'available' && !state.homebrew && (
                        <>
                            <div style={{ fontSize: '0.85rem', fontWeight: 600, color: 'white' }}>
                                Update Available: {state.version}
                            </div>
                            <div style={{ fontSize: '0.75rem', color: '#94a3b8', marginTop: 4 }}>
                                A new version is ready to download
                            </div>
                            <div style={{ display: 'flex', gap: 8, marginTop: 10 }}>
                                <button className="btn btn-primary" style={{ fontSize: '0.75rem', padding: '4px 12px' }} onClick={handleDownload}>
                                    Download & Install
                                </button>
                                <button className="btn outline" style={{ fontSize: '0.75rem', padding: '4px 12px' }} onClick={() => setDismissed(true)}>
                                    Later
                                </button>
                            </div>
                        </>
                    )}
                    {state.type === 'available' && state.homebrew && (
                        <>
                            <div style={{ fontSize: '0.85rem', fontWeight: 600, color: 'white' }}>
                                Update Available: {state.version}
                            </div>
                            <div style={{ fontSize: '0.75rem', color: '#94a3b8', marginTop: 4 }}>
                                Run <code style={{ background: 'rgba(255,255,255,0.1)', padding: '1px 4px', borderRadius: 3 }}>brew upgrade cullsnap</code> to update
                            </div>
                        </>
                    )}
                    {state.type === 'downloading' && (
                        <>
                            <div style={{ fontSize: '0.85rem', fontWeight: 600, color: 'white' }}>
                                Downloading {state.version}...
                            </div>
                            <div style={{
                                background: 'rgba(255,255,255,0.1)',
                                borderRadius: 4,
                                height: 4,
                                marginTop: 8,
                                overflow: 'hidden',
                            }}>
                                <div className="progress-bar-indeterminate" style={{ height: '100%' }} />
                            </div>
                        </>
                    )}
                    {state.type === 'ready' && (
                        <>
                            <div style={{ fontSize: '0.85rem', fontWeight: 600, color: 'white' }}>
                                Update Ready: {state.version}
                            </div>
                            <div style={{ fontSize: '0.75rem', color: '#94a3b8', marginTop: 4 }}>
                                Applied to disk. Restart to use the new version.
                            </div>
                            <div style={{ display: 'flex', gap: 8, marginTop: 10 }}>
                                <button className="btn btn-primary" style={{ fontSize: '0.75rem', padding: '4px 12px' }} onClick={handleRestart}>
                                    Restart Now
                                </button>
                                <button className="btn outline" style={{ fontSize: '0.75rem', padding: '4px 12px' }} onClick={() => setDismissed(true)}>
                                    Later
                                </button>
                            </div>
                        </>
                    )}
                    {state.type === 'error' && (
                        <>
                            <div style={{ fontSize: '0.85rem', fontWeight: 600, color: 'white' }}>
                                Update Failed
                            </div>
                            <div style={{ fontSize: '0.75rem', color: '#94a3b8', marginTop: 4 }}>
                                {state.message}
                            </div>
                        </>
                    )}
                </div>
                <button
                    onClick={() => setDismissed(true)}
                    style={{ background: 'none', border: 'none', color: '#475569', cursor: 'pointer', padding: 0, lineHeight: 1 }}
                >
                    <X size={16} />
                </button>
            </div>
        </div>
    );
}
```

- [ ] **Step 2: Add CSS spin animation**

Check if `.spin` keyframe already exists in the app's CSS. If not, add to the main stylesheet (e.g., `frontend/src/style.css`):

```css
.spin {
    animation: spin 1s linear infinite;
}
@keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
}
```

The `progress-bar-indeterminate` class is already used by the existing loading bar in `App.tsx` (line 411), so it should already be defined.

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd frontend && npm run build
```
Expected: Build succeeds (Wails bindings will be generated after a `wails dev` or `wails generate module`)

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/UpdateToast.tsx frontend/src/style.css
git commit -m "feat: create UpdateToast component with 5 visual states"
```

---

## Task 8: Frontend — Settings Modal Updates Section

**Files:**
- Modify: `frontend/src/components/SettingsModal.tsx:91-103`

- [ ] **Step 1: Add Updates section to SettingsModal**

In `frontend/src/components/SettingsModal.tsx`, add a new section after the "Performance Tuning" section (before `<div className="settings-footer">`):

```tsx
<section className="settings-section">
    <h3>Updates</h3>
    <label className="settings-label">
        Auto-Update
        <span className="settings-hint">(how CullSnap handles new versions)</span>
        <select
            value={config.autoUpdate}
            onChange={e => setConfig(app.AppConfig.createFrom({ ...config, autoUpdate: e.target.value }))}
            style={{
                background: 'rgba(255,255,255,0.1)',
                border: '1px solid rgba(255,255,255,0.2)',
                borderRadius: 6,
                padding: '6px 12px',
                color: 'white',
                fontSize: '0.85rem',
                width: '100%',
                marginTop: 4,
            }}
        >
            <option value="off">Off</option>
            <option value="notify">Notify Only</option>
            <option value="auto">Auto-Update</option>
        </select>
    </label>
    <div style={{ fontSize: '0.7rem', color: 'var(--text-secondary)', lineHeight: 1.6, marginTop: 8, background: 'rgba(0,0,0,0.2)', borderRadius: 6, padding: '8px 10px' }}>
        <div><strong>Off</strong> — No update checks, no network calls</div>
        <div><strong>Notify Only</strong> — Checks for updates, notifies when available</div>
        <div><strong>Auto-Update</strong> — Downloads and applies updates automatically</div>
    </div>
    <div style={{ fontSize: '0.7rem', color: 'var(--text-secondary)', marginTop: 6, fontStyle: 'italic' }}>
        Changes take effect after restart.
    </div>
</section>
```

- [ ] **Step 2: Verify frontend builds**

```bash
cd frontend && npm run build
```
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/SettingsModal.tsx
git commit -m "feat: add Updates section to Settings modal with auto-update dropdown"
```

---

## Task 9: Frontend — About Modal Check for Updates Button

**Files:**
- Modify: `frontend/src/components/AboutModal.tsx`

- [ ] **Step 1: Add Check for Updates button and state**

In `frontend/src/components/AboutModal.tsx`, add import:

```tsx
import { CheckForUpdate } from '../../wailsjs/go/app/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
import { Loader } from 'lucide-react';
```

Add state inside the component:

```tsx
const [checking, setChecking] = useState(false);
const [checkResult, setCheckResult] = useState<string | null>(null);
```

Add after the version `<span>` (inside the centered `<div>`), below `{about.version}`:

```tsx
<div style={{ marginTop: 8 }}>
    <button
        className="btn outline"
        style={{ fontSize: '0.75rem', padding: '4px 12px', display: 'inline-flex', alignItems: 'center', gap: 4 }}
        disabled={checking}
        onClick={async () => {
            setChecking(true);
            setCheckResult(null);
            // Listen for update events to determine result
            const cleanup = () => {
                EventsOff('update:available');
                EventsOff('update:error');
            };
            EventsOn('update:available', () => {
                cleanup();
                setChecking(false);
                // Toast will handle showing the update — no need to show result here
            });
            EventsOn('update:error', (data: any) => {
                cleanup();
                setChecking(false);
                setCheckResult(data?.message || 'Check failed');
                setTimeout(() => setCheckResult(null), 3000);
            });
            try {
                await CheckForUpdate();
                // If no event fires within 8s, assume we're up to date
                setTimeout(() => {
                    cleanup();
                    if (checking) {
                        setChecking(false);
                        setCheckResult('You\'re up to date!');
                        setTimeout(() => setCheckResult(null), 3000);
                    }
                }, 8000);
            } catch (e: any) {
                cleanup();
                setChecking(false);
                setCheckResult(e?.message || 'Check failed');
                setTimeout(() => setCheckResult(null), 3000);
            }
        }}
    >
        {checking ? <Loader size={12} className="spin" /> : null}
        {checking ? 'Checking...' : 'Check for Updates'}
    </button>
    {checkResult && (
        <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: 4 }}>
            {checkResult}
        </div>
    )}
</div>
```

- [ ] **Step 2: Verify frontend builds**

```bash
cd frontend && npm run build
```
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/AboutModal.tsx
git commit -m "feat: add Check for Updates button to About modal"
```

---

## Task 10: Frontend — Wire UpdateToast into App.tsx

**Files:**
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Import and render UpdateToast**

In `frontend/src/App.tsx`, add import:

```tsx
import { UpdateToast } from './components/UpdateToast';
```

Add `<UpdateToast />` right before the closing `</div>` of the root element (after the `{helpOpen && <HelpModal .../>}` line, around line 436):

```tsx
<UpdateToast />
```

- [ ] **Step 2: Verify frontend builds**

```bash
cd frontend && npm run build
```
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add frontend/src/App.tsx
git commit -m "feat: wire UpdateToast into App root"
```

---

## Task 11: CI — Release Workflow Changes

**Files:**
- Modify: `.github/workflows/release.yml`

- [ ] **Step 1: Add standalone macOS binary extraction to build job**

In `.github/workflows/release.yml`, in the macOS compress step (around line 69), add after the `ditto` command:

```yaml
      - name: Extract standalone macOS binary
        if: matrix.os == 'macos-latest'
        run: |
          cp build/bin/CullSnap.app/Contents/MacOS/CullSnap build/bin/CullSnap-darwin-universal
```

Update the artifact upload paths (around line 91) to include the new binary:

```yaml
          path: |
            build/bin/CullSnap-macos-universal.zip
            build/bin/CullSnap-darwin-universal
            build/bin/CullSnap-windows-amd64.exe
            build/bin/CullSnap-linux-amd64
```

- [ ] **Step 2: Add checksum generation and signing to release job**

In the `release` job, add these steps after "Download Artifacts" and before "Create GitHub Release":

```yaml
      - name: Flatten and generate checksums
        run: |
          mkdir -p release_assets
          find artifacts -name 'CullSnap-*' -type f ! -name '*.zip' -exec cp {} release_assets/ \;
          cd release_assets
          sha256sum CullSnap-* | sort > checksums.txt
          cat checksums.txt

      - name: Sign checksums
        env:
          CULLSNAP_UPDATE_SIGNING_KEY: ${{ secrets.CULLSNAP_UPDATE_SIGNING_KEY }}
        run: |
          echo "$CULLSNAP_UPDATE_SIGNING_KEY" > /tmp/signing.pem
          openssl dgst -sha256 -sign /tmp/signing.pem -out release_assets/checksums.txt.sig release_assets/checksums.txt
          rm /tmp/signing.pem
```

Update the "Create GitHub Release" step files list to include checksums:

```yaml
          files: |
            artifacts/**/*CullSnap-*
            release_assets/checksums.txt
            release_assets/checksums.txt.sig
```

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add standalone macOS binary, checksum generation, and ECDSA signing"
```

---

## Task 12: Add GitHub Secret and Manual E2E Verification

This task is manual and cannot be fully automated.

- [ ] **Step 1: Add the ECDSA private key as a GitHub secret**

Go to: `https://github.com/Abhishekmitra-slg/CullSnap/settings/secrets/actions`

Add new secret:
- Name: `CULLSNAP_UPDATE_SIGNING_KEY`
- Value: Contents of `/tmp/cullsnap_update.pem` (generated in Task 1)

- [ ] **Step 2: Clean up the local private key**

```bash
rm /tmp/cullsnap_update.pem
```

- [ ] **Step 3: Run full test suite**

```bash
go test ./internal/... -v
```
Expected: All PASS

- [ ] **Step 4: Run `wails dev` to verify app starts and settings show**

```bash
wails dev
```
Expected: App starts, Settings modal shows "Updates" section, About modal has "Check for Updates" button.

- [ ] **Step 5: Commit any final adjustments (if any)**

Stage only specific files that were changed:
```bash
git status
# Stage only the files you modified, e.g.:
# git add internal/updater/updater.go frontend/src/components/UpdateToast.tsx
git commit -m "chore: final adjustments for auto-update feature"
```
