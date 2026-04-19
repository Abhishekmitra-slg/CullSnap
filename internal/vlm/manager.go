package vlm

import (
	"context"
	"cullsnap/internal/logger"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ManagerState represents the lifecycle state of the VLM Manager.
type ManagerState int

const (
	StateOff          ManagerState = iota // provider not running
	StateStarting                         // provider.Start in progress
	StateReady                            // provider running, idle
	StateBusy                             // inference in flight
	StateIdle                             // running but idle timer counting down
	StateShuttingDown                     // Stop called, provider.Stop in progress
	StateCrashed                          // provider exited unexpectedly
	StateError                            // unrecoverable error (max restarts exceeded)
)

// String returns a human-readable label for the state.
func (s ManagerState) String() string {
	switch s {
	case StateOff:
		return "off"
	case StateStarting:
		return "starting"
	case StateReady:
		return "ready"
	case StateBusy:
		return "busy"
	case StateIdle:
		return "idle"
	case StateShuttingDown:
		return "shutting_down"
	case StateCrashed:
		return "crashed"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// ManagerEvent is emitted on the events channel whenever the manager changes
// state or something noteworthy occurs.
type ManagerEvent struct {
	Type      string    `json:"type"`
	State     string    `json:"state"`
	Message   string    `json:"message"`
	Details   any       `json:"details,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// ManagerStatus is a point-in-time snapshot of Manager health.
type ManagerStatus struct {
	State        string `json:"state"`
	ModelName    string `json:"model_name"`
	Backend      string `json:"backend"`
	Uptime       string `json:"uptime"`
	RestartCount int    `json:"restart_count"`
	InferCount   int    `json:"infer_count"`
	RAMUsageMB   int64  `json:"ram_usage_mb"`
}

// Manager controls the lifecycle of a VLMProvider: starting, stopping, idle
// timeout, health monitoring, and crash recovery.
type Manager struct {
	mu                 sync.Mutex
	state              ManagerState
	provider           VLMProvider
	config             ManagerConfig
	idleTimer          *time.Timer
	restartCount       int
	lastCrash          time.Time
	events             chan ManagerEvent
	startedAt          time.Time
	lastInfer          time.Time
	inferCount         int
	stopHealth         context.CancelFunc
	healthCtx          context.Context // context for health loop + crash recovery
	customInstructions string          // sanitized user prompt suffix, sourced from app config
}

// NewManager creates a Manager in StateOff. events may be nil; if non-nil,
// state-change notifications are sent non-blocking.
func NewManager(cfg ManagerConfig, events chan ManagerEvent) *Manager {
	return &Manager{
		state:  StateOff,
		config: cfg,
		events: events,
	}
}

// SetProvider replaces the provider. Must be called before EnsureRunning.
func (m *Manager) SetProvider(p VLMProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.provider = p
	if logger.Log != nil {
		logger.Log.Debug("vlm: manager: provider set",
			slog.String("provider", p.Name()),
		)
	}
}

// State returns the current ManagerState.
func (m *Manager) State() ManagerState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Config returns the current ManagerConfig.
func (m *Manager) Config() ManagerConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.config
}

// UpdateConfig replaces the current config atomically.
func (m *Manager) UpdateConfig(cfg ManagerConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = cfg
	if logger.Log != nil {
		logger.Log.Debug("vlm: manager: config updated",
			slog.Bool("keepAlive", cfg.KeepAlive),
			slog.Duration("idleTimeout", cfg.IdleTimeout),
		)
	}
}

// SetCustomInstructions caches the sanitized custom-instruction suffix that
// backends append to the system prompt on every inference. The caller must
// pass an already-sanitized string; the manager does NOT re-sanitize so the
// cached hash stays stable across sanitizer rule changes.
func (m *Manager) SetCustomInstructions(sanitized string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.customInstructions = sanitized
	if logger.Log != nil {
		logger.Log.Debug("vlm: manager: custom instructions updated",
			slog.Int("length", len(sanitized)),
		)
	}
}

// CustomInstructions returns the currently cached suffix. Backends call this
// once per inference rather than reading from storage to keep the hot path
// off the SQLite mutex.
func (m *Manager) CustomInstructions() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.customInstructions
}

// EnsureRunning guarantees the provider is running. It is safe to call
// concurrently and repeatedly.
func (m *Manager) EnsureRunning(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch m.state {
	case StateReady, StateBusy, StateIdle:
		// Already up — reset the idle timer and return.
		m.resetIdleTimerLocked()
		if logger.Log != nil {
			logger.Log.Debug("vlm: manager: already running", slog.String("state", m.state.String()))
		}
		return nil
	case StateStarting:
		// Another goroutine is already starting; caller should wait and retry.
		if logger.Log != nil {
			logger.Log.Debug("vlm: manager: already starting")
		}
		return nil
	case StateShuttingDown:
		return fmt.Errorf("vlm: manager: cannot start while shutting down")
	}

	// StateOff / StateCrashed / StateError — attempt to start.
	return m.startLocked(ctx)
}

// startLocked starts the provider. mu MUST be held on entry; it is released
// during the blocking provider.Start call and re-acquired on return.
func (m *Manager) startLocked(ctx context.Context) error {
	if m.provider == nil {
		return fmt.Errorf("vlm: manager: no provider set")
	}

	m.state = StateStarting
	m.emitLocked("state_change", "provider starting")

	if logger.Log != nil {
		logger.Log.Debug("vlm: manager: calling provider.Start",
			slog.String("provider", m.provider.Name()),
		)
	}

	// Release lock during blocking Start.
	m.mu.Unlock()
	err := m.provider.Start(ctx)
	m.mu.Lock()

	if err != nil {
		m.state = StateError
		m.emitLocked("error", fmt.Sprintf("provider.Start failed: %v", err))
		if logger.Log != nil {
			logger.Log.Debug("vlm: manager: provider.Start failed", slog.Any("error", err))
		}
		return fmt.Errorf("vlm: manager: start: %w", err)
	}

	m.state = StateReady
	m.startedAt = time.Now()
	m.resetIdleTimerLocked()
	m.startHealthMonitorLocked()
	m.emitLocked("state_change", "provider ready")

	if logger.Log != nil {
		logger.Log.Debug("vlm: manager: provider started",
			slog.String("provider", m.provider.Name()),
		)
	}

	return nil
}

// Stop shuts the provider down gracefully. It is idempotent.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()

	if m.state == StateOff {
		m.mu.Unlock()
		return nil
	}
	if m.state == StateShuttingDown {
		m.mu.Unlock()
		return nil
	}

	m.state = StateShuttingDown
	m.cancelIdleTimerLocked()
	m.cancelHealthMonitorLocked()
	m.emitLocked("state_change", "shutting down")

	p := m.provider
	m.mu.Unlock()

	if logger.Log != nil {
		logger.Log.Debug("vlm: manager: stopping provider")
	}

	stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var stopErr error
	if p != nil {
		stopErr = p.Stop(stopCtx)
	}

	m.mu.Lock()
	m.state = StateOff
	m.emitLocked("state_change", "provider stopped")
	m.mu.Unlock()

	if logger.Log != nil {
		logger.Log.Debug("vlm: manager: provider stopped", slog.Any("error", stopErr))
	}

	return stopErr
}

// ResetIdleTimer resets the idle timer from outside the manager (e.g. after
// inference). Safe to call without holding the lock.
func (m *Manager) ResetIdleTimer() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resetIdleTimerLocked()
}

// resetIdleTimerLocked resets the idle timer. mu MUST be held.
func (m *Manager) resetIdleTimerLocked() {
	m.cancelIdleTimerLocked()

	if m.config.KeepAlive || m.config.IdleTimeout <= 0 {
		return
	}

	timeout := m.config.IdleTimeout
	m.idleTimer = time.AfterFunc(timeout, func() {
		if logger.Log != nil {
			logger.Log.Debug("vlm: manager: idle timeout reached, stopping")
		}
		_ = m.Stop(context.Background())
	})
}

// cancelIdleTimerLocked cancels the idle timer if set. mu MUST be held.
func (m *Manager) cancelIdleTimerLocked() {
	if m.idleTimer != nil {
		m.idleTimer.Stop()
		m.idleTimer = nil
	}
}

// startHealthMonitorLocked launches the background health-check goroutine.
// mu MUST be held.
func (m *Manager) startHealthMonitorLocked() {
	m.cancelHealthMonitorLocked()

	ctx, cancel := context.WithCancel(context.Background())
	m.stopHealth = cancel
	m.healthCtx = ctx

	go m.healthLoop(ctx)

	if logger.Log != nil {
		logger.Log.Debug("vlm: manager: health monitor started")
	}
}

// cancelHealthMonitorLocked stops the health monitor goroutine. mu MUST be held.
func (m *Manager) cancelHealthMonitorLocked() {
	if m.stopHealth != nil {
		m.stopHealth()
		m.stopHealth = nil
	}
}

// healthLoop runs periodic Health() checks every 10 seconds and calls
// handleCrash after 2 consecutive failures.
func (m *Manager) healthLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	failures := 0

	for {
		select {
		case <-ctx.Done():
			if logger.Log != nil {
				logger.Log.Debug("vlm: manager: health loop exiting")
			}
			return
		case <-ticker.C:
			m.mu.Lock()
			p := m.provider
			st := m.state
			m.mu.Unlock()

			if st != StateReady && st != StateBusy && st != StateIdle {
				// No point checking health when not running.
				continue
			}

			if p == nil {
				continue
			}

			if logger.Log != nil {
				logger.Log.Debug("vlm: manager: health check")
			}

			healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := p.Health(healthCtx)
			cancel()

			if err != nil {
				failures++
				if logger.Log != nil {
					logger.Log.Debug("vlm: manager: health check failed",
						slog.Int("failures", failures),
						slog.Any("error", err),
					)
				}
				if failures >= 2 {
					m.handleCrash()
					return
				}
			} else {
				failures = 0
			}
		}
	}
}

// handleCrash is called when the health monitor detects a dead provider. It
// increments restartCount, applies exponential backoff, and attempts to
// restart. If MaxRestarts is exceeded it sets StateError permanently.
func (m *Manager) handleCrash() {
	m.mu.Lock()
	m.state = StateCrashed
	m.lastCrash = time.Now()
	m.restartCount++
	restartCount := m.restartCount
	maxRestarts := m.config.MaxRestarts
	backoffBase := m.config.RestartBackoff
	m.emitLocked("crash", fmt.Sprintf("provider crashed (restart %d/%d)", restartCount, maxRestarts))
	m.mu.Unlock()

	if logger.Log != nil {
		logger.Log.Debug("vlm: manager: provider crashed",
			slog.Int("restartCount", restartCount),
			slog.Int("maxRestarts", maxRestarts),
		)
	}

	if restartCount > maxRestarts {
		m.mu.Lock()
		m.state = StateError
		m.emitLocked("error", "max restarts exceeded")
		m.mu.Unlock()

		if logger.Log != nil {
			logger.Log.Debug("vlm: manager: max restarts exceeded, giving up")
		}
		return
	}

	// Exponential backoff: 2s, 4s, 8s, …
	backoff := backoffBase
	for i := 1; i < restartCount; i++ {
		backoff *= 2
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: manager: waiting before restart", slog.Duration("backoff", backoff))
	}

	// Use context-aware sleep so shutdown cancels the backoff wait.
	m.mu.Lock()
	ctx := m.healthCtx
	m.mu.Unlock()

	if ctx == nil {
		// Health monitor was cancelled — Stop() was called during crash handling.
		return
	}

	select {
	case <-ctx.Done():
		// Manager is shutting down — don't restart.
		if logger.Log != nil {
			logger.Log.Debug("vlm: manager: crash recovery cancelled by shutdown")
		}
		return
	case <-time.After(backoff):
	}

	m.mu.Lock()
	// Re-check state after sleep — Stop() may have run.
	if m.state == StateOff || m.state == StateShuttingDown {
		m.mu.Unlock()
		return
	}
	err := m.startLocked(context.Background())
	if err != nil {
		// startLocked already set StateError.
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
}

// emitLocked sends a ManagerEvent to the events channel without blocking.
// mu MUST be held.
func (m *Manager) emitLocked(eventType, message string) {
	if m.events == nil {
		return
	}
	evt := ManagerEvent{
		Type:      eventType,
		State:     m.state.String(),
		Message:   message,
		Timestamp: time.Now(),
	}
	select {
	case m.events <- evt:
	default:
		// Channel full — drop the event to avoid blocking.
	}
}

// Status returns a point-in-time snapshot of the manager.
func (m *Manager) Status() ManagerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	var uptime string
	if !m.startedAt.IsZero() && (m.state == StateReady || m.state == StateBusy || m.state == StateIdle) {
		uptime = time.Since(m.startedAt).Truncate(time.Second).String()
	}

	status := ManagerStatus{
		State:        m.state.String(),
		RestartCount: m.restartCount,
		InferCount:   m.inferCount,
		Uptime:       uptime,
	}

	if m.provider != nil {
		info := m.provider.ModelInfo()
		status.ModelName = info.Name
		status.Backend = info.Backend
		status.RAMUsageMB = info.RAMUsage / (1024 * 1024)
	}

	return status
}

// ScorePhoto ensures the provider is running and delegates to provider.ScorePhoto.
// The cached custom-instruction suffix is stamped onto the request so backends
// don't need access to app config.
func (m *Manager) ScorePhoto(ctx context.Context, req ScoreRequest) (*VLMScore, error) {
	if err := m.EnsureRunning(ctx); err != nil {
		return nil, err
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: manager: ScorePhoto",
			slog.String("path", req.PhotoPath),
		)
	}

	m.mu.Lock()
	p := m.provider
	if req.CustomInstructions == "" {
		req.CustomInstructions = m.customInstructions
	}
	m.mu.Unlock()

	if p == nil {
		return nil, fmt.Errorf("vlm: manager: no provider")
	}

	result, err := p.ScorePhoto(ctx, req)

	m.mu.Lock()
	m.lastInfer = time.Now()
	if err == nil {
		m.inferCount++
	}
	m.resetIdleTimerLocked()
	m.mu.Unlock()

	return result, err
}

// RankPhotos ensures the provider is running and delegates to provider.RankPhotos.
// The cached custom-instruction suffix is stamped onto the request so backends
// don't need access to app config.
func (m *Manager) RankPhotos(ctx context.Context, req RankRequest) (*RankingResult, error) {
	if err := m.EnsureRunning(ctx); err != nil {
		return nil, err
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: manager: RankPhotos",
			slog.Int("photos", len(req.PhotoPaths)),
		)
	}

	m.mu.Lock()
	p := m.provider
	if req.CustomInstructions == "" {
		req.CustomInstructions = m.customInstructions
	}
	m.mu.Unlock()

	if p == nil {
		return nil, fmt.Errorf("vlm: manager: no provider")
	}

	result, err := p.RankPhotos(ctx, req)

	m.mu.Lock()
	m.lastInfer = time.Now()
	if err == nil {
		m.inferCount++
	}
	m.resetIdleTimerLocked()
	m.mu.Unlock()

	return result, err
}

// ProviderModelInfo returns the ModelInfo of the current provider, or an empty
// ModelInfo if no provider is set.
func (m *Manager) ProviderModelInfo() ModelInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.provider == nil {
		return ModelInfo{}
	}
	return m.provider.ModelInfo()
}
