# CullSnap

Desktop photo/video culling tool. Wails v2 (Go backend) + React/Vite frontend.

## Commands

```bash
# Development
wails dev                    # Run app with hot reload (frontend + backend)
cd frontend && npm run dev   # Frontend only

# Build
make build                   # Lint + build (VERSION=vX.Y.Z for release)
make test                    # Go tests: go test ./internal/...
make lint                    # go fmt + go vet

# Frontend
cd frontend && npm ci        # Install deps
cd frontend && npx tsc --noEmit  # Type check

# Formatting — ALWAYS use golangci-lint, never standalone gofumpt
golangci-lint fmt ./internal/...

# Release — fully automated
git tag vX.Y.Z && git push origin vX.Y.Z
```

## Architecture

```
main.go              — Entry point, FileLoader (path allowlist), media server (127.0.0.1:34342), HEIC singleflight
internal/
  app/               — Core Wails-bound methods (app.go, ai_methods.go, cloud.go, device.go)
  cloudsource/       — Cloud framework: OAuth2 PKCE, token store (keychain + AES-256-GCM), mirror manager
    providers/       — googledrive/ (REST API), icloud/ (osascript + Photos.app)
  device/            — USB detection + Image Capture import (macOS), platform build tags
  scanner/           — Directory walker (jpg/png/heic/raw/video)
  image/             — Thumbnail generation + disk cache, HEIC conversion branch
  raw/               — RAW decoder: TIFF IFD (CR2/NEF/ARW/DNG/ORF/RW2/PEF/NRW/SRW), BMFF (CR3), RAF
  heic/              — HEIC/HEIF: sips (macOS) + FFmpeg fallback, platform build tags
  dedupe/            — dHash perceptual hashing, multi-factor quality scoring + AI score blending (60/40)
  export/            — File copy with video trim
  storage/           — SQLite (modernc.org/sqlite, pure Go) — config KV, selections, ratings, cloud data, VLM tables (vlm_scores, vlm_rankings, vlm_ranking_groups, vlm_token_usage)
  video/             — FFmpeg provisioning, trim, thumbnails
  updater/           — Auto-update: state machine, GitHub releases, ECDSA+SHA256 verification
  logger/            — Logging
  model/             — Shared types
  scoring/           — AI scoring: plugin architecture, ONNX Runtime (purego), face detection (SCRFD), face recognition (ArcFace), sharpness (Laplacian), pipeline orchestrator, agglomerative clustering
  vlm/               — VLM integration: Gemma 4 via llama.cpp/MLX subprocess, manager state machine, hardware probe, prompt engine, JSON parser, model registry, provisioner
frontend/src/
  App.tsx            — Main app shell
  components/        — Grid, Viewer, Sidebar, SettingsModal, AboutModal, CloudSourceModal, DeviceImportModal, AIProgressModal, AIPanel, AIOnboardingModal, BottomBar, etc.
  main.tsx           — React entry point
frontend/wailsjs/    — Generated Wails TS bindings (MUST be committed — CI runs tsc before wails build)
```

## Key Patterns

- **Media server** bound to 127.0.0.1 with directory allowlist (security hardened)
- **Viewer** uses direct `<img src>` URLs, NOT fetch->blob (WKWebView cross-origin issue)
- **Version embedding**: `-ldflags "-X main.version=vX.Y.Z"` in Makefile and release.yml
- **HEIC conversion** uses `singleflight` to dedup concurrent requests
- **Dedup hashes/quality scores** computed from 300px cached thumbnails (not originals) for USB drive perf
- **Platform build tags** on device/ and heic/ packages for cross-platform compilation
- **No CGO**: All inference (ONNX, future VLM) uses purego or external subprocesses. Keeps cross-compilation clean and CI simple
- **External binary pattern**: FFmpeg and ONNX Runtime are provisioned at runtime to `~/.cullsnap/`. New runtimes (llama-server, MLX) follow this same pattern

## AI Scoring

- **Plugin architecture**: `ScoringPlugin` interface, `Registry` for registration, `Pipeline` for orchestration
- **ONNX Models**: SCRFD-2.5GF (face detection, ~3MB), ArcFace-MobileFaceNet (face recognition, ~14MB)
- **VLM Models**: Gemma 4 E4B (~2.8GB) / E2B (~1.5GB) — downloaded at runtime, managed by `internal/vlm/`
- **Runtime**: ONNX Runtime via `github.com/shota3506/onnxruntime-purego` — no CGO, downloads `.dylib`/`.so` at runtime
- **Worker pool**: `NumCPU/2` goroutines, bounded 1-8, shared ONNX sessions with per-model mutex
- **Score weights**: User-configurable (aesthetic/sharpness/face/eyes/composition), persisted to config KV, auto-normalized
- **VLM pipeline**: Stage 4 (individual scoring) + Stage 5 (pairwise ranking) run after ONNX stages. Adaptive threshold sends only top-N photos to VLM
- **VLM runtime**: Manager state machine (8 states), dual backends (llama.cpp + MLX), idle timeout, crash recovery with exponential backoff
- **Face clustering**: Agglomerative with cosine similarity, threshold 0.45, capped at 2000 faces
- **Platform**: macOS + Linux (ONNX via purego). Windows stubbed (Available()=false)
- **Models stored**: `~/.cullsnap/models/` (ONNX + GGUF), ONNX Runtime lib in `~/.cullsnap/lib/`, llama-server in `~/.cullsnap/bin/`

## Gotchas

- **`window.confirm()`/`alert()`/`prompt()` DO NOT WORK** in Wails WKWebView — return false silently. Use inline confirmation dialogs
- **Formatting**: Use `golangci-lint fmt`, NEVER standalone `gofumpt` — version mismatch causes CI failures
- **Import ordering**: golangci-lint v2 wants `cullsnap/*` alphabetically mixed with stdlib; aliased imports in separate group
- **Commit hook**: `commit-msg` hook blocks AI attribution (Co-Authored-By AI lines). Enforced
- **Wails TS bindings** (`frontend/wailsjs/`) must be committed — CI needs them for `tsc`
- **SanitizeID** must reject "." and ".." to prevent path traversal via public Wails endpoints
- **VLM JSON parsing**: Non-greedy regex `\{.*?\}` breaks on nested objects — use brace-depth counting (`extractFirstJSONObject`)
- **SQLiteStore mutex**: Every write method MUST acquire `s.mu.Lock()` — missing mutex on `AssignFaceToCluster` was a review finding
- **VLM pipeline slice passing**: Pass full `toScore` slice to VLM stages, not `toScore[:scored]` — `scored` is an atomic counter, not an index
- **App.Shutdown()**: Must close `vlmEvents` channel in shutdown to prevent goroutine leak from `forwardVLMEvents`

## CI (GitHub Actions)

- **ci.yml**: lint (golangci-lint v2.1.6 from source), test (race + coverage >= 53%), build, frontend-lint (tsc), security (govulncheck + gitleaks)
- **release.yml**: Wails build (mac/windows/linux), extract macOS binary, SHA256 checksums + ECDSA signature, GitHub Release, homebrew-tap update
- **Go 1.25**, Node 22

## Code Style

- golangci-lint v2 config in `.golangci.yml` — gofumpt formatter, gosec/gocritic/staticcheck enabled
- gosec exclusions documented inline (G107, G110, G115, G204, G301, G302, G304, G401, G501) — all justified for desktop app context
- Always include debug logging in new code
- Test files excluded from gosec/errcheck

## Model Selection for Token Optimization

Route tasks to the lowest-cost model that maintains quality. Do not default to Opus.

| Task | Model | Why |
|------|-------|-----|
| File searches, simple renames, formatting fixes, grep/glob lookups | **Haiku** | Mechanical work, no reasoning needed |
| Everyday coding, bug fixes, refactors, tests, docs, PR reviews | **Sonnet** | Handles ~90% of dev tasks without compromise |
| Complex architecture, multi-file system design, deep cross-codebase reasoning, zero-shot problem solving | **Opus** | Only when Sonnet would need multiple attempts |

**Rules:**
1. **Start low, escalate on failure.** Try Sonnet first for coding tasks. Only escalate to Opus if the task requires synthesizing 10+ files or deep architectural reasoning.
2. **Use Haiku for subagents** doing mechanical work: file searches, simple code generation, formatting, data extraction.
3. **Use Sonnet for subagents** doing code review, test writing, or moderate refactoring.
4. **Reserve Opus for the main agent** when orchestrating complex multi-step plans or when a subagent's Sonnet-level output was insufficient.
5. **Keep prompts lean.** Use XML tags for structure. Skip conversational filler. Request concise output ("3-bullet summary" not "a summary").
