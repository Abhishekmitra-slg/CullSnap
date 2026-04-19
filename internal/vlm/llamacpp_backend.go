package vlm

import (
	"context"
	"crypto/rand"
	"cullsnap/internal/logger"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

const (
	llamaCppBackendName     = "llamacpp"
	llamaCppCtxSize         = 4096
	llamaCppGPULayers       = 99
	llamaCppReadyTimeout    = 60 * time.Second
	llamaCppReadyPoll       = 500 * time.Millisecond
	llamaCppStopGrace       = 5 * time.Second
	llamaCppTokenBytesLen   = 32
	llamaCppMaxImages       = 5
	llamaCppHost            = "127.0.0.1"
	llamaCppMaxStartRetries = 3
)

var llamaCppDefaultTokenBudgets = []int{70, 140, 280, 560, 1120}

// LlamaCppBackend implements VLMProvider using a local llama-server process.
type LlamaCppBackend struct {
	mu           sync.Mutex
	binaryPath   string
	modelPath    string
	modelEntry   ModelEntry
	cmd          *exec.Cmd
	port         int
	token        string
	client       *Client
	cancelStderr context.CancelFunc
	stderrBuf    *boundedBuffer
	// procDone is closed once cmd.Wait() returns; waitErr holds the exit error.
	// Shared between waitForReady (fast crash detection) and Stop (exit coordination).
	procDone chan struct{}
	waitErr  error
}

// NewLlamaCppBackend creates a new LlamaCppBackend with the given binary and model paths.
func NewLlamaCppBackend(binaryPath, modelPath string, entry ModelEntry) *LlamaCppBackend {
	if logger.Log != nil {
		logger.Log.Debug("vlm: llamacpp: creating backend",
			slog.String("binaryPath", binaryPath),
			slog.String("modelPath", modelPath),
			slog.String("model", entry.Name),
			slog.String("variant", entry.Variant),
		)
	}
	return &LlamaCppBackend{
		binaryPath: binaryPath,
		modelPath:  modelPath,
		modelEntry: entry,
	}
}

// Name returns the backend identifier.
func (b *LlamaCppBackend) Name() string {
	return llamaCppBackendName
}

// ModelInfo returns metadata about the loaded model.
func (b *LlamaCppBackend) ModelInfo() ModelInfo {
	b.mu.Lock()
	entry := b.modelEntry
	b.mu.Unlock()

	return ModelInfo{
		Name:         entry.Name,
		Variant:      entry.Variant,
		SizeBytes:    entry.SizeBytes,
		RAMUsage:     entry.RAMUsage,
		Backend:      llamaCppBackendName,
		MaxImages:    llamaCppMaxImages,
		TokenBudgets: llamaCppDefaultTokenBudgets,
	}
}

// Start launches the llama-server subprocess and waits for it to be ready.
// Wraps startOnceLocked in a bounded retry loop so a rare TOCTOU port
// collision between findFreePort() and subprocess bind cannot strand the
// caller on a 60s waitForReady timeout.
func (b *LlamaCppBackend) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if logger.Log != nil {
		logger.Log.Debug("vlm: llamacpp: Start called",
			slog.String("binaryPath", b.binaryPath),
			slog.String("modelPath", b.modelPath),
		)
	}

	// Verify binary exists.
	if _, err := os.Stat(b.binaryPath); err != nil {
		if logger.Log != nil {
			logger.Log.Debug("vlm: llamacpp: binary not found", slog.String("path", b.binaryPath), slog.Any("err", err))
		}
		return fmt.Errorf("vlm: llamacpp: binary not found at %q: %w", b.binaryPath, err)
	}

	// Verify model exists.
	if _, err := os.Stat(b.modelPath); err != nil {
		if logger.Log != nil {
			logger.Log.Debug("vlm: llamacpp: model not found", slog.String("path", b.modelPath), slog.Any("err", err))
		}
		return fmt.Errorf("vlm: llamacpp: model not found at %q: %w", b.modelPath, err)
	}

	var lastErr error
	for attempt := 1; attempt <= llamaCppMaxStartRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err, retriable := b.startOnceLocked(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		if !retriable {
			return err
		}
		if logger.Log != nil {
			logger.Log.Warn("vlm: llamacpp: start attempt failed, retrying",
				slog.Int("attempt", attempt),
				slog.Int("maxAttempts", llamaCppMaxStartRetries),
				slog.Any("err", err),
			)
		}
	}
	return fmt.Errorf("vlm: llamacpp: failed after %d start attempts: %w", llamaCppMaxStartRetries, lastErr)
}

// startOnceLocked performs a single launch attempt. Must be called with b.mu held.
// The bool return indicates whether the caller should retry: true for failures
// after cmd.Start() succeeded (covers port TOCTOU, transient subprocess crashes),
// false for pre-exec failures (port alloc, token gen, pipe setup, cmd.Start).
func (b *LlamaCppBackend) startOnceLocked(ctx context.Context) (error, bool) {
	// Find a free port.
	port, err := findFreePort()
	if err != nil {
		return fmt.Errorf("vlm: llamacpp: find free port: %w", err), false
	}

	// Generate a session token.
	token, err := generateSessionToken()
	if err != nil {
		return fmt.Errorf("vlm: llamacpp: generate session token: %w", err), false
	}

	args := []string{
		"--model", b.modelPath,
		"--host", llamaCppHost,
		"--port", fmt.Sprintf("%d", port),
		"--ctx-size", fmt.Sprintf("%d", llamaCppCtxSize),
		"--n-gpu-layers", fmt.Sprintf("%d", llamaCppGPULayers),
		"--api-key", token,
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: llamacpp: launching server",
			slog.Int("port", port),
			slog.String("host", llamaCppHost),
		)
	}

	// Set up stderr capture with a cancellable context.
	stderrCtx, cancelStderr := context.WithCancel(context.Background())

	// Use background context for the subprocess — its lifetime is managed by Stop(),
	// not by the transient caller's context. The caller's ctx is only used for waitForReady.
	cmd := exec.CommandContext(context.Background(), b.binaryPath, args...) // #nosec G204 -- binaryPath provisioned by CullSnap // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	stderrPipe, pipeErr := cmd.StderrPipe()
	if pipeErr != nil {
		cancelStderr()
		return fmt.Errorf("vlm: llamacpp: create stderr pipe: %w", pipeErr), false
	}

	if startErr := cmd.Start(); startErr != nil {
		cancelStderr()
		return fmt.Errorf("vlm: llamacpp: start process: %w", startErr), false
	}

	// Capture stderr into a bounded ring for crash diagnostics, while still logging live.
	stderrBuf := newBoundedBuffer(8192)
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-stderrCtx.Done():
				return
			default:
			}
			n, readErr := stderrPipe.Read(buf)
			if n > 0 {
				_, _ = stderrBuf.Write(buf[:n])
				if logger.Log != nil {
					logger.Log.Debug("vlm: llamacpp: server stderr", slog.String("output", string(buf[:n])))
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	// Initialize all backend state BEFORE spawning the Wait goroutine so that
	// the goroutine's write to b.waitErr (for fast-exiting processes like bad
	// binaries) does not race with our initialization. After close(procDone)
	// the Go memory model synchronizes later reads of b.waitErr with that write.
	procDone := make(chan struct{})
	b.cmd = cmd
	b.port = port
	b.token = token
	b.cancelStderr = cancelStderr
	b.stderrBuf = stderrBuf
	b.procDone = procDone
	b.waitErr = nil

	go func() {
		err := cmd.Wait()
		b.waitErr = err
		close(procDone)
	}()

	baseURL := fmt.Sprintf("http://%s:%d", llamaCppHost, port)

	// Wait for the server to be ready.
	if err := b.waitForReady(ctx, baseURL); err != nil {
		cancelStderr()
		// If the process is still alive, kill it; otherwise Kill is a harmless no-op.
		_ = cmd.Process.Kill()
		<-procDone // ensure Wait goroutine has returned before clearing state
		b.cmd = nil
		b.cancelStderr = nil
		b.procDone = nil
		b.stderrBuf = nil
		// Caller context cancellation is terminal — do not retry on the caller's behalf.
		retriable := ctx.Err() == nil
		return fmt.Errorf("vlm: llamacpp: server did not become ready: %w", err), retriable
	}

	b.client = NewClient(baseURL, token, b.modelEntry.Name)

	if logger.Log != nil {
		logger.Log.Debug("vlm: llamacpp: server ready",
			slog.Int("port", port),
			slog.String("model", b.modelEntry.Name),
		)
	}

	return nil, false
}

// waitForReady polls /health until the server responds OK, the deadline expires,
// or the subprocess exits early (crash). Early exit is detected via procDone and
// reported with captured stderr so the user gets actionable diagnostics.
func (b *LlamaCppBackend) waitForReady(ctx context.Context, baseURL string) error {
	procDone := b.procDone
	stderrBuf := b.stderrBuf

	deadline := time.Now().Add(llamaCppReadyTimeout)
	healthURL := baseURL + "/health"

	httpClient := &http.Client{Timeout: 2 * time.Second}

	if logger.Log != nil {
		logger.Log.Debug("vlm: llamacpp: waiting for server ready", slog.String("url", healthURL))
	}

	crashErr := func() error {
		// Safe to read b.waitErr without mutex — receive on procDone
		// happens-after the close, which happens-after the write.
		exit := "process exited"
		if b.waitErr != nil {
			exit = b.waitErr.Error()
		}
		tail := ""
		if stderrBuf != nil {
			tail = stderrBuf.String()
		}
		if tail != "" {
			return fmt.Errorf("llama-server crashed during startup: %s\nstderr: %s", exit, tail)
		}
		return fmt.Errorf("llama-server crashed during startup: %s", exit)
	}

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for llama-server at %s", llamaCppReadyTimeout, healthURL)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-procDone:
			return crashErr()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return fmt.Errorf("build health request: %w", err)
		}

		resp, err := httpClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				if logger.Log != nil {
					logger.Log.Debug("vlm: llamacpp: health check passed")
				}
				return nil
			}
			if logger.Log != nil {
				logger.Log.Debug("vlm: llamacpp: health check non-200", slog.Int("status", resp.StatusCode))
			}
		} else if logger.Log != nil {
			logger.Log.Debug("vlm: llamacpp: health check error", slog.Any("err", err))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-procDone:
			return crashErr()
		case <-time.After(llamaCppReadyPoll):
		}
	}
}

// Stop signals the llama-server process to terminate and waits for it to exit.
// Uses the shared procDone channel populated by the Wait goroutine spawned in Start.
func (b *LlamaCppBackend) Stop(ctx context.Context) error {
	b.mu.Lock()
	cmd := b.cmd
	cancelStderr := b.cancelStderr
	procDone := b.procDone
	b.cmd = nil
	b.client = nil
	b.cancelStderr = nil
	b.procDone = nil
	b.stderrBuf = nil
	b.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		if logger.Log != nil {
			logger.Log.Debug("vlm: llamacpp: Stop called but no process running")
		}
		return nil
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: llamacpp: sending Interrupt to server", slog.Int("pid", cmd.Process.Pid))
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		if logger.Log != nil {
			logger.Log.Debug("vlm: llamacpp: interrupt failed, killing", slog.Any("err", err))
		}
		_ = cmd.Process.Kill()
	}

	select {
	case <-procDone:
		if cancelStderr != nil {
			cancelStderr()
		}
		if logger.Log != nil {
			logger.Log.Debug("vlm: llamacpp: server exited", slog.Any("err", b.waitErr))
		}
		return nil
	case <-time.After(llamaCppStopGrace):
		if logger.Log != nil {
			logger.Log.Debug("vlm: llamacpp: grace period elapsed, killing process")
		}
		_ = cmd.Process.Kill()
		<-procDone
		if cancelStderr != nil {
			cancelStderr()
		}
		return nil
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-procDone
		if cancelStderr != nil {
			cancelStderr()
		}
		return ctx.Err()
	}
}

// Health checks whether the llama-server is reachable.
func (b *LlamaCppBackend) Health(ctx context.Context) error {
	b.mu.Lock()
	port := b.port
	b.mu.Unlock()

	if port == 0 {
		return fmt.Errorf("vlm: llamacpp: server not started")
	}

	healthURL := fmt.Sprintf("http://%s:%d/health", llamaCppHost, port)

	if logger.Log != nil {
		logger.Log.Debug("vlm: llamacpp: Health check", slog.String("url", healthURL))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return fmt.Errorf("vlm: llamacpp: health: build request: %w", err)
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vlm: llamacpp: health: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // HTTP response body close

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("vlm: llamacpp: health: server returned %d", resp.StatusCode)
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: llamacpp: Health check passed")
	}

	return nil
}

// ScorePhoto sends a single photo to the VLM for scoring.
func (b *LlamaCppBackend) ScorePhoto(ctx context.Context, req ScoreRequest) (*VLMScore, error) {
	b.mu.Lock()
	client := b.client
	b.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("vlm: llamacpp: ScorePhoto: server not started")
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: llamacpp: ScorePhoto",
			slog.String("photoPath", req.PhotoPath),
			slog.Int("tokenBudget", req.TokenBudget),
			slog.Int("faceCount", req.FaceCount),
			slog.Float64("sharpness", req.Sharpness),
		)
	}

	systemPrompt := SystemPrompt(req.CustomInstructions)
	userPrompt := Stage4Prompt(Stage4Input{
		Context:        req.Context,
		FaceCount:      req.FaceCount,
		SharpnessScore: req.Sharpness,
	})

	maxTokens := req.TokenBudget
	if maxTokens <= 0 {
		maxTokens = llamaCppDefaultTokenBudgets[0]
	}

	raw, tokens, err := client.ChatCompletion(ctx, systemPrompt, userPrompt, []string{req.PhotoPath}, maxTokens)
	if err != nil {
		return nil, fmt.Errorf("vlm: llamacpp: ScorePhoto: chat completion: %w", err)
	}

	score, err := ParseVLMScore(raw)
	if err != nil {
		return nil, fmt.Errorf("vlm: llamacpp: ScorePhoto: parse score: %w", err)
	}
	if valErr := score.Validate(); valErr != nil {
		return nil, fmt.Errorf("vlm: llamacpp: ScorePhoto: validation: %w", valErr)
	}

	score.TokensUsed = tokens

	if logger.Log != nil {
		logger.Log.Debug("vlm: llamacpp: ScorePhoto complete",
			slog.Int("tokensUsed", tokens),
			slog.Float64("aesthetic", score.Aesthetic),
		)
	}

	return score, nil
}

// RankPhotos sends a batch of photos to the VLM for comparative ranking.
func (b *LlamaCppBackend) RankPhotos(ctx context.Context, req RankRequest) (*RankingResult, error) {
	b.mu.Lock()
	client := b.client
	b.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("vlm: llamacpp: RankPhotos: server not started")
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: llamacpp: RankPhotos",
			slog.Int("photoCount", len(req.PhotoPaths)),
			slog.String("useCase", req.UseCase),
			slog.Int("tokenBudget", req.TokenBudget),
		)
	}

	photos := make([]Stage5Photo, 0, len(req.PhotoScores))
	for _, pc := range req.PhotoScores {
		photos = append(photos, Stage5Photo(pc))
	}

	systemPrompt := SystemPrompt(req.CustomInstructions)
	userPrompt := Stage5Prompt(Stage5Input{
		Photos:  photos,
		UseCase: req.UseCase,
	})

	maxTokens := req.TokenBudget
	if maxTokens <= 0 {
		maxTokens = llamaCppDefaultTokenBudgets[3] // 560 for ranking
	}

	raw, tokens, err := client.ChatCompletion(ctx, systemPrompt, userPrompt, req.PhotoPaths, maxTokens)
	if err != nil {
		return nil, fmt.Errorf("vlm: llamacpp: RankPhotos: chat completion: %w", err)
	}

	result, err := ParseRankingResult(raw, req.PhotoPaths)
	if err != nil {
		return nil, fmt.Errorf("vlm: llamacpp: RankPhotos: parse ranking: %w", err)
	}

	result.TokensUsed = tokens

	if logger.Log != nil {
		logger.Log.Debug("vlm: llamacpp: RankPhotos complete",
			slog.Int("tokensUsed", tokens),
			slog.Int("ranked", len(result.Ranked)),
		)
	}

	return result, nil
}

// generateSessionToken returns a cryptographically random 64-char hex string.
func generateSessionToken() (string, error) {
	buf := make([]byte, llamaCppTokenBytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("vlm: llamacpp: generate token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
