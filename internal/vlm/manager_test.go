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
