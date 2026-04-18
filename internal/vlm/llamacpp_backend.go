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
	llamaCppBackendName   = "llamacpp"
	llamaCppCtxSize       = 4096
	llamaCppGPULayers     = 99
	llamaCppReadyTimeout  = 60 * time.Second
	llamaCppReadyPollMs   = 500 * time.Millisecond
	llamaCppStopGraceMs   = 5 * time.Second
	llamaCppTokenBytesLen = 32
	llamaCppMaxImages     = 5
	llamaCppHost          = "127.0.0.1"
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

	// Find a free port.
	port, err := findFreePort()
	if err != nil {
		return fmt.Errorf("vlm: llamacpp: find free port: %w", err)
	}

	// Generate a session token.
	token, err := generateSessionToken()
	if err != nil {
		return fmt.Errorf("vlm: llamacpp: generate session token: %w", err)
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
		return fmt.Errorf("vlm: llamacpp: create stderr pipe: %w", pipeErr)
	}

	if startErr := cmd.Start(); startErr != nil {
		cancelStderr()
		return fmt.Errorf("vlm: llamacpp: start process: %w", startErr)
	}

	// Drain stderr in background to prevent blocking.
	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-stderrCtx.Done():
				return
			default:
			}
			n, readErr := stderrPipe.Read(buf)
			if n > 0 && logger.Log != nil {
				logger.Log.Debug("vlm: llamacpp: server stderr", slog.String("output", string(buf[:n])))
			}
			if readErr != nil {
				return
			}
		}
	}()

	b.cmd = cmd
	b.port = port
	b.token = token
	b.cancelStderr = cancelStderr

	baseURL := fmt.Sprintf("http://%s:%d", llamaCppHost, port)

	// Wait for the server to be ready.
	if err := b.waitForReady(ctx, baseURL); err != nil {
		cancelStderr()
		_ = cmd.Process.Kill()
		b.cmd = nil
		b.cancelStderr = nil
		return fmt.Errorf("vlm: llamacpp: server did not become ready: %w", err)
	}

	b.client = NewClient(baseURL, token, b.modelEntry.Name)

	if logger.Log != nil {
		logger.Log.Debug("vlm: llamacpp: server ready",
			slog.Int("port", port),
			slog.String("model", b.modelEntry.Name),
		)
	}

	return nil
}

// waitForReady polls the /health endpoint until the server responds OK or the deadline passes.
func (b *LlamaCppBackend) waitForReady(ctx context.Context, baseURL string) error {
	deadline := time.Now().Add(llamaCppReadyTimeout)
	healthURL := baseURL + "/health"

	httpClient := &http.Client{Timeout: 2 * time.Second}

	if logger.Log != nil {
		logger.Log.Debug("vlm: llamacpp: waiting for server ready", slog.String("url", healthURL))
	}

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for llama-server at %s", llamaCppReadyTimeout, healthURL)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
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
		} else {
			if logger.Log != nil {
				logger.Log.Debug("vlm: llamacpp: health check error", slog.Any("err", err))
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(llamaCppReadyPollMs):
		}
	}
}

// Stop signals the llama-server process to terminate and waits for it to exit.
func (b *LlamaCppBackend) Stop(ctx context.Context) error {
	b.mu.Lock()
	cmd := b.cmd
	cancelStderr := b.cancelStderr
	b.cmd = nil
	b.client = nil
	b.cancelStderr = nil
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

	// Wait for process to exit with a grace period.
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if cancelStderr != nil {
			cancelStderr()
		}
		if logger.Log != nil {
			logger.Log.Debug("vlm: llamacpp: server exited", slog.Any("err", err))
		}
		return nil
	case <-time.After(llamaCppStopGraceMs):
		if logger.Log != nil {
			logger.Log.Debug("vlm: llamacpp: grace period elapsed, killing process")
		}
		_ = cmd.Process.Kill()
		<-done
		if cancelStderr != nil {
			cancelStderr()
		}
		return nil
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
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
	defer resp.Body.Close()

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

	systemPrompt := SystemPrompt("")
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
		photos = append(photos, Stage5Photo{
			Aesthetic: pc.Aesthetic,
			Sharpness: pc.Sharpness,
			FaceCount: pc.FaceCount,
			Issues:    pc.Issues,
		})
	}

	systemPrompt := SystemPrompt("")
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
