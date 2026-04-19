//go:build darwin && arm64

package vlm

import (
	"context"
	"cullsnap/internal/logger"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const (
	mlxReadyPollInterval = 1 * time.Second
	mlxReadyDeadline     = 120 * time.Second
	mlxTokenBudgetBase   = 70
)

// mlxTokenBudgets is the standard token-budget ladder for VLM calls.
var mlxTokenBudgets = []int{70, 140, 280, 560, 1120}

// MLXBackend launches and manages an mlx_vlm.server subprocess on Apple Silicon.
type MLXBackend struct {
	mu           sync.Mutex
	venvPath     string
	modelPath    string
	modelEntry   ModelEntry
	cmd          *exec.Cmd
	port         int
	client       *Client
	stderrBuf    *boundedBuffer
	cancelStderr context.CancelFunc
	// procDone is closed once cmd.Wait() returns; waitErr holds the exit error.
	procDone chan struct{}
	waitErr  error
}

// NewMLXBackend returns an MLXBackend configured for the given venv and model path.
// venvPath is typically ~/.cullsnap/mlx-venv/ and modelPath points to the downloaded
// MLX model directory under ~/.cullsnap/models/.
func NewMLXBackend(venvPath, modelPath string, entry ModelEntry) *MLXBackend {
	if logger.Log != nil {
		logger.Log.Debug("vlm: mlx: NewMLXBackend",
			slog.String("venvPath", venvPath),
			slog.String("modelPath", modelPath),
			slog.String("model", entry.Name),
			slog.String("variant", entry.Variant),
		)
	}
	return &MLXBackend{
		venvPath:   venvPath,
		modelPath:  modelPath,
		modelEntry: entry,
	}
}

// Name returns the backend identifier.
func (b *MLXBackend) Name() string { return "mlx" }

// Available always returns true on Apple Silicon.
func (b *MLXBackend) Available() bool { return true }

// ModelInfo returns metadata about the currently configured model.
func (b *MLXBackend) ModelInfo() ModelInfo {
	return ModelInfo{
		Name:         b.modelEntry.Name,
		Variant:      b.modelEntry.Variant,
		SizeBytes:    b.modelEntry.SizeBytes,
		RAMUsage:     b.modelEntry.RAMUsage,
		Backend:      "mlx",
		MaxImages:    5,
		TokenBudgets: mlxTokenBudgets,
	}
}

// Start verifies prerequisites, allocates a port, and launches the mlx_vlm.server
// subprocess. It blocks until the server is ready or the context deadline is reached.
func (b *MLXBackend) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if logger.Log != nil {
		logger.Log.Debug("vlm: mlx: Start called",
			slog.String("venvPath", b.venvPath),
			slog.String("modelPath", b.modelPath),
		)
	}

	// Verify python3 interpreter exists inside the venv.
	python3 := filepath.Join(b.venvPath, "bin", "python3")
	if _, err := os.Stat(python3); err != nil {
		return fmt.Errorf("vlm: mlx: python3 not found at %q — is the venv provisioned? (%w)", python3, err)
	}

	// Verify model directory exists.
	if _, err := os.Stat(b.modelPath); err != nil {
		return fmt.Errorf("vlm: mlx: model not found at %q (%w)", b.modelPath, err)
	}

	port, err := findFreePort()
	if err != nil {
		return fmt.Errorf("vlm: mlx: find free port: %w", err)
	}
	b.port = port

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	// MLX server has no auth token.
	b.client = NewClient(baseURL, "", b.modelEntry.Name)

	if logger.Log != nil {
		logger.Log.Debug("vlm: mlx: launching server",
			slog.String("python3", python3),
			slog.String("model", b.modelPath),
			slog.Int("port", port),
		)
	}

	// Use background context for the subprocess — its lifetime is managed by Stop(),
	// not by the transient caller's context. The caller's ctx is only used for waitForReady.
	// #nosec G204 -- python3 and modelPath are provisioner-controlled paths, not user input
	cmd := exec.CommandContext(context.Background(), python3, // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
		"-m", "mlx_vlm.server",
		"--model", b.modelPath,
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", port),
	)

	// Pipe stdout+stderr so we can both mirror them to our stderr (for live
	// debugging) and retain a bounded tail for crash diagnostics.
	stdoutPipe, soutErr := cmd.StdoutPipe()
	if soutErr != nil {
		return fmt.Errorf("vlm: mlx: stdout pipe: %w", soutErr)
	}
	stderrPipe, serrErr := cmd.StderrPipe()
	if serrErr != nil {
		return fmt.Errorf("vlm: mlx: stderr pipe: %w", serrErr)
	}
	stderrBuf := newBoundedBuffer(8192)
	stderrCtx, cancelStderr := context.WithCancel(context.Background())

	if startErr := cmd.Start(); startErr != nil {
		cancelStderr()
		return fmt.Errorf("vlm: mlx: start subprocess: %w", startErr)
	}

	// Drain both pipes; tee into os.Stderr and into the ring.
	drain := func(r io.Reader, tag string) {
		buf := make([]byte, 4096)
		for {
			select {
			case <-stderrCtx.Done():
				return
			default:
			}
			n, readErr := r.Read(buf)
			if n > 0 {
				_, _ = stderrBuf.Write(buf[:n])
				_, _ = os.Stderr.Write(buf[:n])
				if logger.Log != nil {
					logger.Log.Debug("vlm: mlx: subprocess output", slog.String("stream", tag), slog.String("output", string(buf[:n])))
				}
			}
			if readErr != nil {
				return
			}
		}
	}
	go drain(stdoutPipe, "stdout")
	go drain(stderrPipe, "stderr")

	// Initialize all backend state BEFORE spawning the Wait goroutine to avoid
	// racing on b.waitErr if the subprocess exits immediately.
	procDone := make(chan struct{})
	b.cmd = cmd
	b.port = port
	b.stderrBuf = stderrBuf
	b.cancelStderr = cancelStderr
	b.procDone = procDone
	b.waitErr = nil

	go func() {
		err := cmd.Wait()
		b.waitErr = err
		close(procDone)
	}()

	if logger.Log != nil {
		logger.Log.Debug("vlm: mlx: subprocess started, waiting for ready", slog.Int("pid", cmd.Process.Pid))
	}

	if readyErr := b.waitForReady(ctx, baseURL); readyErr != nil {
		// Attempt cleanup on failure. If the process already exited, Kill is a harmless no-op.
		_ = cmd.Process.Kill()
		<-procDone
		cancelStderr()
		b.cmd = nil
		b.cancelStderr = nil
		b.procDone = nil
		b.stderrBuf = nil
		return fmt.Errorf("vlm: mlx: server did not become ready: %w", readyErr)
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: mlx: server ready", slog.Int("port", port))
	}
	return nil
}

// waitForReady polls GET /v1/models every second until the server responds with
// HTTP 200, the 120-second deadline expires, or the subprocess exits early.
func (b *MLXBackend) waitForReady(ctx context.Context, baseURL string) error {
	procDone := b.procDone
	stderrBuf := b.stderrBuf

	deadline := time.Now().Add(mlxReadyDeadline)
	hc := &http.Client{Timeout: 5 * time.Second}
	modelsURL := baseURL + "/v1/models"

	crashErr := func() error {
		exit := "process exited"
		if b.waitErr != nil {
			exit = b.waitErr.Error()
		}
		tail := ""
		if stderrBuf != nil {
			tail = stderrBuf.String()
		}
		if tail != "" {
			return fmt.Errorf("mlx server crashed during startup: %s\nstderr: %s", exit, tail)
		}
		return fmt.Errorf("mlx server crashed during startup: %s", exit)
	}

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s", mlxReadyDeadline)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-procDone:
			return crashErr()
		default:
		}

		resp, err := hc.Get(modelsURL) //nolint:gosec // URL constructed from localhost + port
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				if logger.Log != nil {
					logger.Log.Debug("vlm: mlx: /v1/models returned 200 — server ready")
				}
				return nil
			}
			if logger.Log != nil {
				logger.Log.Debug("vlm: mlx: /v1/models not ready yet", slog.Int("status", resp.StatusCode))
			}
		} else if logger.Log != nil {
			logger.Log.Debug("vlm: mlx: /v1/models poll error", slog.String("err", err.Error()))
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-procDone:
			return crashErr()
		case <-time.After(mlxReadyPollInterval):
		}
	}
}

// Stop sends SIGINT to the server subprocess, waits for it to exit, and falls
// back to SIGKILL if the process has not terminated within 5 seconds. Coordination
// uses the shared procDone channel populated by the Wait goroutine in Start.
func (b *MLXBackend) Stop(_ context.Context) error {
	b.mu.Lock()
	cmd := b.cmd
	procDone := b.procDone
	cancelStderr := b.cancelStderr
	if cmd == nil || cmd.Process == nil {
		b.mu.Unlock()
		if logger.Log != nil {
			logger.Log.Debug("vlm: mlx: Stop called but no running process")
		}
		return nil
	}
	proc := cmd.Process
	b.cmd = nil
	b.client = nil
	b.cancelStderr = nil
	b.procDone = nil
	b.stderrBuf = nil
	b.mu.Unlock()

	if logger.Log != nil {
		logger.Log.Debug("vlm: mlx: sending SIGINT to server", slog.Int("pid", proc.Pid))
	}

	if err := proc.Signal(os.Interrupt); err != nil {
		if logger.Log != nil {
			logger.Log.Warn("vlm: mlx: SIGINT failed, sending SIGKILL", slog.String("err", err.Error()))
		}
		_ = proc.Kill()
	}

	select {
	case <-procDone:
		if logger.Log != nil {
			logger.Log.Debug("vlm: mlx: server exited cleanly")
		}
	case <-time.After(5 * time.Second):
		if logger.Log != nil {
			logger.Log.Warn("vlm: mlx: server did not exit within 5s, killing")
		}
		_ = proc.Kill()
		<-procDone
	}

	if cancelStderr != nil {
		cancelStderr()
	}
	return nil
}

// Health performs a GET /v1/models health check against the running server.
func (b *MLXBackend) Health(ctx context.Context) error {
	b.mu.Lock()
	client := b.client
	port := b.port
	b.mu.Unlock()

	if client == nil {
		return fmt.Errorf("vlm: mlx: server not started")
	}

	hc := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/models", port)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("vlm: mlx: health: build request: %w", err)
	}

	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("vlm: mlx: health: request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // HTTP response body close

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("vlm: mlx: health: unexpected status %d", resp.StatusCode)
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: mlx: health check passed", slog.Int("port", port))
	}
	return nil
}

// ScorePhoto sends a single-photo scoring request to the MLX VLM server and
// returns a parsed VLMScore.
func (b *MLXBackend) ScorePhoto(ctx context.Context, req ScoreRequest) (*VLMScore, error) {
	b.mu.Lock()
	client := b.client
	b.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("vlm: mlx: server not started")
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: mlx: ScorePhoto",
			slog.String("photo", req.PhotoPath),
			slog.Int("tokenBudget", req.TokenBudget),
			slog.Int("faceCount", req.FaceCount),
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
		maxTokens = mlxTokenBudgetBase
	}

	raw, tokens, err := client.ChatCompletion(ctx, systemPrompt, userPrompt, []string{req.PhotoPath}, maxTokens)
	if err != nil {
		return nil, fmt.Errorf("vlm: mlx: ScorePhoto: chat completion: %w", err)
	}

	score, err := ParseVLMScore(raw)
	if err != nil {
		return nil, fmt.Errorf("vlm: mlx: ScorePhoto: parse: %w", err)
	}
	if valErr := score.Validate(); valErr != nil {
		return nil, fmt.Errorf("vlm: mlx: ScorePhoto: validation: %w", valErr)
	}
	score.TokensUsed = tokens

	if logger.Log != nil {
		logger.Log.Debug("vlm: mlx: ScorePhoto complete",
			slog.Int("tokens", tokens),
			slog.Float64("aesthetic", score.Aesthetic),
		)
	}
	return score, nil
}

// RankPhotos sends a batch ranking request to the MLX VLM server and returns a
// parsed RankingResult.
func (b *MLXBackend) RankPhotos(ctx context.Context, req RankRequest) (*RankingResult, error) {
	b.mu.Lock()
	client := b.client
	b.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("vlm: mlx: server not started")
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: mlx: RankPhotos",
			slog.Int("photoCount", len(req.PhotoPaths)),
			slog.String("useCase", req.UseCase),
			slog.Int("tokenBudget", req.TokenBudget),
		)
	}

	photos := make([]Stage5Photo, len(req.PhotoScores))
	for i, pc := range req.PhotoScores {
		photos[i] = Stage5Photo(pc)
	}

	systemPrompt := SystemPrompt("")
	userPrompt := Stage5Prompt(Stage5Input{
		Photos:  photos,
		UseCase: req.UseCase,
	})

	maxTokens := req.TokenBudget
	if maxTokens <= 0 {
		maxTokens = mlxTokenBudgetBase
	}

	raw, tokens, err := client.ChatCompletion(ctx, systemPrompt, userPrompt, req.PhotoPaths, maxTokens)
	if err != nil {
		return nil, fmt.Errorf("vlm: mlx: RankPhotos: chat completion: %w", err)
	}

	result, err := ParseRankingResult(raw, req.PhotoPaths)
	if err != nil {
		return nil, fmt.Errorf("vlm: mlx: RankPhotos: parse: %w", err)
	}
	result.TokensUsed = tokens

	if logger.Log != nil {
		logger.Log.Debug("vlm: mlx: RankPhotos complete",
			slog.Int("tokens", tokens),
			slog.Int("rankedCount", len(result.Ranked)),
		)
	}
	return result, nil
}
