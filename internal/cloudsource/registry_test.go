package cloudsource

import (
	"context"
	"testing"
)

type mockProvider struct {
	id   string
	name string
}

func (m *mockProvider) ID() string            { return m.id }
func (m *mockProvider) DisplayName() string   { return m.name }
func (m *mockProvider) IsAvailable() bool     { return true }
func (m *mockProvider) IsAuthenticated() bool { return false }

func (m *mockProvider) Authenticate(_ context.Context) error { return nil }

func (m *mockProvider) ListAlbums(_ context.Context) ([]Album, error) { return nil, nil }

func (m *mockProvider) ListMediaInAlbum(_ context.Context, _ string) ([]RemoteMedia, error) {
	return nil, nil
}

func (m *mockProvider) Download(_ context.Context, _ RemoteMedia, _ string, _ func(int64, int64)) error {
	return nil
}

func (m *mockProvider) Disconnect() error { return nil }

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	p := &mockProvider{id: "test", name: "Test Provider"}
	r.Register(p)

	got, ok := r.Get("test")
	if !ok {
		t.Fatal("provider not found after Register")
	}
	if got.ID() != "test" {
		t.Errorf("ID = %q, want %q", got.ID(), "test")
	}
	if got.DisplayName() != "Test Provider" {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName(), "Test Provider")
	}
}

func TestRegistry_All_Order(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockProvider{id: "b", name: "B"})
	r.Register(&mockProvider{id: "a", name: "A"})

	all := r.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(all))
	}
	if all[0].ProviderID != "b" {
		t.Errorf("first should be 'b' (insertion order), got %q", all[0].ProviderID)
	}
	if all[1].ProviderID != "a" {
		t.Errorf("second should be 'a', got %q", all[1].ProviderID)
	}
}

func TestRegistry_All_Status(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockProvider{id: "test", name: "Test"})

	all := r.All()
	if len(all) != 1 {
		t.Fatalf("expected 1, got %d", len(all))
	}
	s := all[0]
	if s.ProviderID != "test" {
		t.Errorf("ProviderID = %q, want %q", s.ProviderID, "test")
	}
	if s.DisplayName != "Test" {
		t.Errorf("DisplayName = %q, want %q", s.DisplayName, "Test")
	}
	if !s.IsAvailable {
		t.Error("IsAvailable should be true (mock returns true)")
	}
	if s.IsConnected {
		t.Error("IsConnected should be false (mock returns false)")
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("should not find nonexistent provider")
	}
}

func TestRegistry_DuplicateRegister(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockProvider{id: "x", name: "V1"})
	r.Register(&mockProvider{id: "x", name: "V2"})

	all := r.All()
	if len(all) != 1 {
		t.Fatalf("duplicate register should not duplicate in order, got %d", len(all))
	}
	if all[0].DisplayName != "V2" {
		t.Errorf("should have updated to V2, got %q", all[0].DisplayName)
	}

	got, ok := r.Get("x")
	if !ok {
		t.Fatal("provider 'x' not found")
	}
	if got.DisplayName() != "V2" {
		t.Errorf("Get should return updated provider, got %q", got.DisplayName())
	}
}

func TestRegistry_EmptyAll(t *testing.T) {
	r := NewRegistry()
	all := r.All()
	if len(all) != 0 {
		t.Errorf("empty registry should return 0 providers, got %d", len(all))
	}
}

func TestRegistry_MultipleProviders(t *testing.T) {
	r := NewRegistry()
	for _, id := range []string{"alpha", "beta", "gamma", "delta"} {
		r.Register(&mockProvider{id: id, name: id + "-name"})
	}

	all := r.All()
	if len(all) != 4 {
		t.Fatalf("expected 4 providers, got %d", len(all))
	}

	// Verify insertion order preserved
	expected := []string{"alpha", "beta", "gamma", "delta"}
	for i, e := range expected {
		if all[i].ProviderID != e {
			t.Errorf("all[%d].ProviderID = %q, want %q", i, all[i].ProviderID, e)
		}
	}
}
