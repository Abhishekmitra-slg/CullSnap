package vlm

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockProvider is a test double for VLMProvider.
type mockProvider struct {
	mu          sync.Mutex
	startErr    error
	stopErr     error
	healthErr   error
	started     bool
	stopped     bool
	healthCalls int
	scoreResult *VLMScore
	scoreErr    error
}

func (p *mockProvider) Name() string { return "mock" }

func (p *mockProvider) Start(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.startErr != nil {
		return p.startErr
	}
	p.started = true
	return nil
}

func (p *mockProvider) Stop(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = true
	if p.stopErr != nil {
		return p.stopErr
	}
	return nil
}

func (p *mockProvider) Health(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.healthCalls++
	return p.healthErr
}

func (p *mockProvider) ScorePhoto(_ context.Context, _ ScoreRequest) (*VLMScore, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.scoreResult, p.scoreErr
}

func (p *mockProvider) RankPhotos(_ context.Context, _ RankRequest) (*RankingResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return &RankingResult{Explanation: "mock ranking"}, nil
}

func (p *mockProvider) ModelInfo() ModelInfo {
	return ModelInfo{Name: "mock-model", Backend: "mock"}
}

// TestManagerInitialState verifies that a newly created manager starts in StateOff.
func TestManagerInitialState(t *testing.T) {
	cfg := DefaultManagerConfig()
	m := NewManager(cfg, nil)
	if got := m.State(); got != StateOff {
		t.Errorf("initial state = %v, want StateOff", got)
	}
}

// TestManagerStartStop verifies EnsureRunning transitions to StateReady and
// Stop transitions back to StateOff.
func TestManagerStartStop(t *testing.T) {
	cfg := DefaultManagerConfig()
	cfg.KeepAlive = true // prevent idle timer from interfering

	events := make(chan ManagerEvent, 16)
	m := NewManager(cfg, events)

	mp := &mockProvider{}
	m.SetProvider(mp)

	ctx := context.Background()

	if err := m.EnsureRunning(ctx); err != nil {
		t.Fatalf("EnsureRunning error: %v", err)
	}

	if got := m.State(); got != StateReady {
		t.Errorf("after EnsureRunning state = %v, want StateReady", got)
	}

	mp.mu.Lock()
	started := mp.started
	mp.mu.Unlock()

	if !started {
		t.Error("expected provider.started = true after EnsureRunning")
	}

	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop error: %v", err)
	}

	if got := m.State(); got != StateOff {
		t.Errorf("after Stop state = %v, want StateOff", got)
	}
}

// TestManagerIdleTimeout verifies that the manager stops after IdleTimeout
// when KeepAlive is false.
func TestManagerIdleTimeout(t *testing.T) {
	cfg := DefaultManagerConfig()
	cfg.KeepAlive = false
	cfg.IdleTimeout = 50 * time.Millisecond

	m := NewManager(cfg, nil)
	mp := &mockProvider{}
	m.SetProvider(mp)

	if err := m.EnsureRunning(context.Background()); err != nil {
		t.Fatalf("EnsureRunning error: %v", err)
	}

	// Wait long enough for the idle timer to fire.
	time.Sleep(200 * time.Millisecond)

	if got := m.State(); got != StateOff {
		t.Errorf("after idle timeout state = %v, want StateOff", got)
	}
}

// TestManagerKeepAliveIgnoresIdleTimeout verifies that when KeepAlive is true
// the idle timeout does NOT stop the manager.
func TestManagerKeepAliveIgnoresIdleTimeout(t *testing.T) {
	cfg := DefaultManagerConfig()
	cfg.KeepAlive = true
	cfg.IdleTimeout = 50 * time.Millisecond

	m := NewManager(cfg, nil)
	mp := &mockProvider{}
	m.SetProvider(mp)

	if err := m.EnsureRunning(context.Background()); err != nil {
		t.Fatalf("EnsureRunning error: %v", err)
	}

	// Wait past what would be the idle timeout.
	time.Sleep(200 * time.Millisecond)

	if got := m.State(); got == StateOff {
		t.Errorf("with KeepAlive=true manager stopped after idle timeout — should stay running")
	}

	_ = m.Stop(context.Background())
}

// TestManagerEnsureRunningIdempotent verifies that calling EnsureRunning
// multiple times on an already-running manager is a no-op.
func TestManagerEnsureRunningIdempotent(t *testing.T) {
	cfg := DefaultManagerConfig()
	cfg.KeepAlive = true

	m := NewManager(cfg, nil)
	mp := &mockProvider{}
	m.SetProvider(mp)

	ctx := context.Background()

	for i := range 3 {
		if err := m.EnsureRunning(ctx); err != nil {
			t.Fatalf("EnsureRunning call %d error: %v", i+1, err)
		}
	}

	if got := m.State(); got != StateReady {
		t.Errorf("after 3x EnsureRunning state = %v, want StateReady", got)
	}

	_ = m.Stop(ctx)
}

// TestManagerConfigUpdate verifies Config returns current config and UpdateConfig replaces it.
func TestManagerConfigUpdate(t *testing.T) {
	cfg := DefaultManagerConfig()
	cfg.KeepAlive = false
	cfg.IdleTimeout = 10 * time.Second

	m := NewManager(cfg, nil)
	got := m.Config()
	if got.KeepAlive != false || got.IdleTimeout != 10*time.Second {
		t.Errorf("Config() = %+v, want KeepAlive=false IdleTimeout=10s", got)
	}

	newCfg := ManagerConfig{KeepAlive: true, IdleTimeout: 5 * time.Minute}
	m.UpdateConfig(newCfg)
	if got := m.Config(); got.KeepAlive != true || got.IdleTimeout != 5*time.Minute {
		t.Errorf("after UpdateConfig: Config() = %+v, want KeepAlive=true IdleTimeout=5m", got)
	}
}

// TestManagerStatusOff verifies Status on an un-started manager returns StateOff info.
func TestManagerStatusOff(t *testing.T) {
	m := NewManager(DefaultManagerConfig(), nil)
	s := m.Status()
	if s.State != StateOff.String() {
		t.Errorf("Status().State = %q, want %q", s.State, StateOff.String())
	}
	if s.Uptime != "" {
		t.Errorf("Status().Uptime = %q, want empty", s.Uptime)
	}
}

// TestManagerStatusRunning verifies Status reports model info when provider is set and running.
func TestManagerStatusRunning(t *testing.T) {
	cfg := DefaultManagerConfig()
	cfg.KeepAlive = true
	m := NewManager(cfg, nil)
	m.SetProvider(&mockProvider{})

	if err := m.EnsureRunning(context.Background()); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	defer m.Stop(context.Background())

	s := m.Status()
	if s.State != StateReady.String() {
		t.Errorf("Status().State = %q, want %q", s.State, StateReady.String())
	}
	if s.ModelName != "mock-model" || s.Backend != "mock" {
		t.Errorf("Status() model = %q/%q, want mock-model/mock", s.ModelName, s.Backend)
	}
}

// TestManagerProviderModelInfoNil verifies ProviderModelInfo returns empty info with no provider.
func TestManagerProviderModelInfoNil(t *testing.T) {
	m := NewManager(DefaultManagerConfig(), nil)
	info := m.ProviderModelInfo()
	if info.Name != "" || info.Backend != "" {
		t.Errorf("ProviderModelInfo() with no provider = %+v, want empty", info)
	}
}

// TestManagerProviderModelInfoSet verifies ProviderModelInfo delegates to the set provider.
func TestManagerProviderModelInfoSet(t *testing.T) {
	m := NewManager(DefaultManagerConfig(), nil)
	m.SetProvider(&mockProvider{})
	info := m.ProviderModelInfo()
	if info.Name != "mock-model" || info.Backend != "mock" {
		t.Errorf("ProviderModelInfo() = %+v, want mock-model/mock", info)
	}
}

// TestManagerResetIdleTimer verifies ResetIdleTimer is safe to call even when no timer is set.
func TestManagerResetIdleTimer(t *testing.T) {
	m := NewManager(DefaultManagerConfig(), nil)
	// Should be safe to call before any timer is set — exercises the cancelLocked no-op branch.
	m.ResetIdleTimer()

	// Reset while running exercises the AfterFunc branch.
	cfg := DefaultManagerConfig()
	cfg.IdleTimeout = 10 * time.Minute
	m2 := NewManager(cfg, nil)
	m2.SetProvider(&mockProvider{})
	if err := m2.EnsureRunning(context.Background()); err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	defer m2.Stop(context.Background())
	m2.ResetIdleTimer()
}
