# Auto-Update Feature Design

## Overview

CullSnap will check for new releases on GitHub and offer seamless in-place updates. Users control behavior via a three-state setting: Off, Notify Only (default), or Auto-Update.

## Architecture

### Library

[creativeprojects/go-selfupdate](https://github.com/creativeprojects/go-selfupdate) — a pure Go library that detects the latest GitHub release, downloads the platform-specific binary, verifies integrity, and replaces the running executable with rollback on failure.

### New Package: `internal/updater/`

Single `Updater` struct responsible for:

1. Reading the `autoUpdate` config value from SQLite on initialization
2. Spawning a background goroutine (if not `"off"`) that:
   - Checks GitHub Releases immediately on startup
   - Rechecks every 6 hours via `time.Ticker`
3. Emitting Wails events to the frontend based on update state
4. Exposing methods for the frontend to trigger download and restart

```go
type State int

const (
    StateIdle State = iota
    StateChecking
    StateDownloading
    StateReady
    StateError
)

type Updater struct {
    currentVersion string
    repoSlug       string              // "Abhishekmitra-slg/CullSnap"
    ctx            context.Context     // Wails app context (for events + shutdown)
    store          *storage.SQLiteStore
    publicKey      []byte              // embedded ECDSA P-256 public key
    mu             sync.Mutex          // protects state transitions
    state          State               // current update state
    latestRelease  *selfupdate.Release // cached release info after check
}
```

**State machine:** All methods check the current state before acting. Invalid transitions are no-ops (e.g., calling `DownloadUpdate()` while already downloading). Changing the config to `"off"` sets state to `StateIdle` — any in-progress download is cancelled via context.

**Constructor:** `NewUpdater(ctx, store, version, publicKey)` — reads `autoUpdate` config from the passed `AppConfig` (single source of truth — the Updater does not read from the store directly), returns `*Updater`.

**go-selfupdate asset name matching:** CullSnap's release assets use a non-standard naming convention (`CullSnap-darwin-universal`, `CullSnap-windows-amd64.exe`, etc.) that doesn't match go-selfupdate's default `{repo}_{version}_{os}_{arch}` pattern. Configure `selfupdate.Config` with a custom `Filters` regex to match the actual asset names:

```go
config := selfupdate.Config{
    Filters: []string{"CullSnap-"},  // match all CullSnap assets
}
```

The `Updater` then selects the correct asset based on `runtime.GOOS` and `runtime.GOARCH` mapping:
- `darwin/*` → `CullSnap-darwin-universal`
- `windows/amd64` → `CullSnap-windows-amd64.exe`
- `linux/amd64` → `CullSnap-linux-amd64`

**Dev build guard:** If `currentVersion == "dev"` or does not parse as valid semver, the updater skips all checks regardless of config. Logs: `"Skipping update checks: dev build"`.

**Lifecycle:**
- `Start()` — called from `App.Startup()`. If mode is not `"off"` and version is valid semver, launches background goroutine. The goroutine listens on `a.ctx.Done()` for shutdown (same pattern as `emitSystemMetrics`).
- `CheckNow()` — manual check, exposed to frontend for the "Check for Updates" button in About modal. Includes a 60-second cooldown to prevent GitHub API rate-limit exhaustion from rapid clicks.
- `DownloadUpdate()` — async: returns immediately, spawns goroutine that downloads and applies the update, emitting events. Called by frontend in notify-only mode when user clicks "Download & Install".
- `RestartForUpdate()` — relaunches the binary and quits the current process.

### Integration Points

**`main.go`:**
- Embed public key via `//go:embed keys/update_signing.pub`
- Pass to `App` struct alongside version string

**`internal/app/app.go`:**
- Create `Updater` in `Startup()`, pass Wails context, store, version, and public key
- Expose `DownloadUpdate()`, `RestartForUpdate()`, and `CheckForUpdate()` as Wails-bound methods
- Updater stops automatically via `a.ctx.Done()` (no separate `Stop()` call needed — Wails cancels the context on shutdown)

**`internal/app/config.go`:**
- Add `AutoUpdate string` field to `AppConfig` struct (values: `"off"`, `"notify"`, `"auto"`, default: `"notify"`)
- Update `loadOrInitConfig()` and `persistConfig()` to handle the new field
- Persist/load via existing `store.GetConfig("autoUpdate")` / `store.SetConfig("autoUpdate", value)`

## Security Model

### Dual Verification: SHA256 + ECDSA

Every release includes three integrity assets:
1. `checksums.txt` — SHA256 hashes of all release binaries
2. `checksums.txt.sig` — ECDSA signature of `checksums.txt`
3. The ECDSA P-256 public key, embedded in the binary at compile time

**Verification via go-selfupdate's built-in validator:**

The library provides `NewChecksumWithECDSAValidator()` which chains ECDSA signature verification of `checksums.txt` with SHA256 checksum verification of release assets. Configure it in the `selfupdate.Config.Validator` field:

```go
validator := &selfupdate.ChecksumWithECDSAValidator{
    PublicKey: ecdsaPublicKey, // parsed from embedded PEM
}
config := selfupdate.Config{
    Validator: validator,
}
```

This handles the full verification flow internally — no custom verification code needed.

### Key Management

- **Private key:** Stored as GitHub Actions secret `CULLSNAP_UPDATE_SIGNING_KEY`. Never in the repository.
- **Public key:** Committed to `keys/update_signing.pub`. Embedded in the binary via `go:embed`.
- **Algorithm:** ECDSA with P-256 curve (FIPS 186-3 compatible, required by go-selfupdate's crypto).

### Additional Security Properties

- **HTTPS only:** All communication via GitHub API over TLS.
- **No downgrade attacks:** Semver comparison ensures only newer versions are offered.
- **Rollback on failure:** go-selfupdate backs up the current binary before replacement. On write failure, the backup is restored automatically.
- **No telemetry:** The only network call is to `api.github.com/repos/Abhishekmitra-slg/CullSnap/releases`. No PII, no analytics.
- **Rate limit safe:** GitHub allows 60 unauthenticated API requests/hour. At 1 check per 6 hours, usage is minimal.

## Platform-Specific Considerations

### macOS (.app Bundle)

go-selfupdate replaces a single binary at the path returned by `os.Executable()`, which on macOS is `.app/Contents/MacOS/CullSnap`. This works for binary-only updates, but if a new version changes `Info.plist`, icons, or other bundle resources, only replacing the inner binary would leave the app in an inconsistent state.

**Solution:** The CI release pipeline will produce an additional **standalone binary** asset (`CullSnap-darwin-universal`) alongside the existing `.app` zip. The updater targets this standalone binary for self-update. The `.app` zip remains available for fresh installs and Homebrew.

Since CullSnap is not currently code-signed or notarized, replacing the inner binary does not invalidate any signature. When code signing is added in the future, the update strategy must be revisited (options: full bundle replacement, re-signing, or Sparkle framework).

### Windows

Replacing an executable in protected directories (e.g., `Program Files`) may require elevated permissions. go-selfupdate uses `os.Rename` which can fail without admin rights. If this occurs, the `update:error` event will surface the message: "Update failed: permission denied. Try running CullSnap as administrator, or move it to a user-writable directory."

### Linux

No special considerations — the binary is typically in a user-writable location. Standard `os.Rename` works.

## User Control

### Three-State Setting

| Mode | Behavior |
|------|----------|
| **Off** | No network calls. No update checks. Completely silent. |
| **Notify Only** (default) | Checks on startup + every 6 hours. Shows toast when update is available. User must explicitly click "Download & Install". |
| **Auto-Update** | Same check schedule. Downloads and applies the update automatically. Shows toast with "Restart Now" option. Update takes effect on next natural app launch even if toast is dismissed. |

### Config Storage

- Key: `autoUpdate` in SQLite `app_config` table
- Values: `"off"` | `"notify"` | `"auto"`
- Default: `"notify"`
- Added to `AppConfig` struct as `AutoUpdate string \`json:"autoUpdate"\``
- **Runtime config changes:** Changing the setting from "off" to "notify"/"auto" (or vice versa) takes effect on **next app launch**. The background goroutine is started once during `Startup()` based on the initial config. This avoids complexity of starting/stopping goroutines mid-session. The Settings UI will display a note: "Changes take effect after restart."

## Frontend Integration

### Settings Modal

New "Updates" section added below existing "Performance Tuning" section in `SettingsModal.tsx`:
- Three-state dropdown (Off / Notify Only / Auto-Update)
- Helper text explaining each mode
- Current version display

### About Modal

Add a "Check for Updates" button next to the version display. Calls `App.CheckForUpdate()`. Shows a brief spinner while checking, then either "You're up to date" or transitions to the update toast flow.

### Toast Notification Component

New `UpdateToast.tsx` component, rendered in `App.tsx`. Four visual states:

1. **Update Available** (green border) — version number, "Download & Install" button, "Later" button, dismiss X. **Notify mode only.**
2. **Downloading** (blue border) — version number, indeterminate spinner (no progress bar — go-selfupdate does not expose download progress).
3. **Update Ready** (green border) — "Applied to disk" message, "Restart Now" button, "Later" button.
4. **Error** (red border) — error message (e.g., "Signature verification failed. Update skipped for safety.")

Position: bottom-right of app window. Non-blocking. Dismissable. Reappears on next check cycle if dismissed — **in notify mode only** (in auto mode, the flow progresses from checking directly to downloading to ready, with no dismissable "available" state).

### Wails Events (Go → Frontend)

| Event | Payload | When |
|-------|---------|------|
| `update:available` | `{ version: string, releaseURL: string }` | New version detected, waiting for user action (notify mode only) |
| `update:downloading` | `{ version: string }` | Download in progress (indeterminate — no progress tracking) |
| `update:ready` | `{ version: string }` | Binary replaced on disk, ready for restart |
| `update:error` | `{ message: string }` | Any failure (network, checksum, signature, disk, permissions) |

### Wails-Bound Methods (Frontend → Go)

| Method | Purpose |
|--------|---------|
| `App.CheckForUpdate()` | Manual check from About modal button |
| `App.DownloadUpdate()` | User clicked "Download & Install" in notify mode. Async — returns immediately, emits events. |
| `App.RestartForUpdate()` | User clicked "Restart Now" |

## Restart Mechanism

When user clicks "Restart Now":
1. `os.Executable()` resolves the current binary path
2. `exec.Command(exePath).Start()` launches the new binary
3. `runtime.Quit(a.ctx)` exits the current process via Wails runtime

On macOS, the binary is inside `.app/Contents/MacOS/` — launching it re-opens the app naturally.

## CI Pipeline Changes

### One-Time Setup

1. Generate ECDSA P-256 key pair:
   ```bash
   openssl ecparam -genkey -name prime256v1 -noout -out cullsnap_update.pem
   openssl ec -in cullsnap_update.pem -pubout -out keys/update_signing.pub
   ```
2. Store private key content as GitHub secret: `CULLSNAP_UPDATE_SIGNING_KEY`
3. Commit `keys/update_signing.pub` to the repository

### Release Workflow Additions

**Build job additions (macOS):**
- After the Wails build, extract the inner binary from the `.app` bundle and upload it as a separate artifact:
  ```bash
  cp build/bin/CullSnap.app/Contents/MacOS/CullSnap build/bin/CullSnap-darwin-universal
  ```
- The artifact upload step must include `CullSnap-darwin-universal` alongside the existing zip.
- This standalone binary is what the updater targets for self-update. The `.app` zip remains for fresh installs and Homebrew.

**Release job additions** (after downloading all build artifacts):

1. **Flatten artifacts and generate checksums** (bare filenames for go-selfupdate compatibility):
   ```bash
   mkdir -p release_assets
   find artifacts -name 'CullSnap-*' -type f -exec cp {} release_assets/ \;
   cd release_assets
   sha256sum CullSnap-* | sort > checksums.txt
   ```

2. **Sign checksums:**
   ```bash
   echo "$CULLSNAP_UPDATE_SIGNING_KEY" > /tmp/signing.pem
   openssl dgst -sha256 -sign /tmp/signing.pem -out checksums.txt.sig checksums.txt
   rm /tmp/signing.pem
   ```

3. **Upload** `checksums.txt` and `checksums.txt.sig` as additional release assets.

## Testing Strategy

### Unit Tests (`internal/updater/`)

- Version comparison: current >= latest returns no update
- Dev build guard: `"dev"` version skips all checks
- Config parsing: "off" / "notify" / "auto" modes set correctly
- State machine: invalid transitions are no-ops
- Checksum verification: valid checksum passes, tampered fails
- Signature verification: valid signature passes, invalid key or tampered file fails
- Mock GitHub source (go-selfupdate supports custom `Source` interface) to avoid network in CI

### Integration Test

- Use go-selfupdate's local/mock source to simulate the full check → download → verify flow without network access

### Manual E2E Test (Pre-First-Release)

- Build a `v0.0.1-test` binary
- Create a `v0.0.2-test` GitHub release with checksums and signature
- Verify the full update cycle on macOS, Windows, and Linux

## Dependencies

### New Go Dependencies

- `github.com/creativeprojects/go-selfupdate` — core update library

### New Files

- `internal/updater/updater.go` — Updater struct, state machine, background check loop, download, verify, apply
- `internal/updater/updater_test.go` — unit tests
- `keys/update_signing.pub` — ECDSA public key (committed to repo)
- `frontend/src/components/UpdateToast.tsx` — toast notification component

### Modified Files

- `main.go` — embed public key, pass to App
- `internal/app/app.go` — create Updater in Startup, expose DownloadUpdate/RestartForUpdate/CheckForUpdate
- `internal/app/config.go` — add AutoUpdate field to AppConfig, update loadOrInitConfig/persistConfig
- `frontend/src/components/SettingsModal.tsx` — add Updates section with dropdown
- `frontend/src/components/AboutModal.tsx` — add "Check for Updates" button
- `frontend/src/App.tsx` — render UpdateToast, listen for Wails events
- `.github/workflows/release.yml` — add standalone macOS binary, checksum generation, and signing steps
- `go.mod` / `go.sum` — new dependency
