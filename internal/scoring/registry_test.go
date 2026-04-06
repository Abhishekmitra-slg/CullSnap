package scoring

import (
	"context"
	"errors"
	"image"
	"testing"
)

// mockPlugin is a minimal ScoringPlugin implementation for registry tests.
type mockPlugin struct {
	name      string
	category  PluginCategory
	models    []ModelSpec
	available bool
	initErr   error
	closeErr  error
}

func (m *mockPlugin) Name() string             { return m.name }
func (m *mockPlugin) Category() PluginCategory { return m.category }
func (m *mockPlugin) Models() []ModelSpec      { return m.models }
func (m *mockPlugin) Available() bool          { return m.available }
func (m *mockPlugin) Init(_ string) error      { return m.initErr }
func (m *mockPlugin) Close() error             { return m.closeErr }
func (m *mockPlugin) Process(_ context.Context, _ image.Image) (PluginResult, error) {
	return PluginResult{}, nil
}

// helpers

func newDetector(name string, avail bool) *mockPlugin {
	return &mockPlugin{name: name, category: CategoryDetection, available: avail}
}

func newQuality(name string, avail bool) *mockPlugin {
	return &mockPlugin{name: name, category: CategoryQuality, available: avail}
}

func newRecognition(name string, avail bool) *mockPlugin {
	return &mockPlugin{name: name, category: CategoryRecognition, available: avail}
}

// tests

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	p := newDetector("BlazeFace", true)
	r.Register(p)

	got := r.Get("BlazeFace")
	if got == nil {
		t.Fatal("Get() returned nil, want non-nil")
	}
	if got.Name() != "BlazeFace" {
		t.Errorf("Get().Name() = %q, want %q", got.Name(), "BlazeFace")
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()
	if got := r.Get("NonExistent"); got != nil {
		t.Errorf("Get() = %v, want nil", got)
	}
}

func TestRegistry_GetByCategory_MultipleDetectors(t *testing.T) {
	r := NewRegistry()
	r.Register(newDetector("DetectorA", true))
	r.Register(newDetector("DetectorB", true))
	r.Register(newQuality("QualityX", true))

	detectors := r.GetByCategory(CategoryDetection)
	if len(detectors) != 2 {
		t.Errorf("GetByCategory(Detection) len = %d, want 2", len(detectors))
	}

	quality := r.GetByCategory(CategoryQuality)
	if len(quality) != 1 {
		t.Errorf("GetByCategory(Quality) len = %d, want 1", len(quality))
	}

	recognition := r.GetByCategory(CategoryRecognition)
	if len(recognition) != 0 {
		t.Errorf("GetByCategory(Recognition) len = %d, want 0", len(recognition))
	}
}

func TestRegistry_Available_RequiresBothDetectionAndQuality(t *testing.T) {
	r := NewRegistry()

	// No plugins → not available.
	if r.Available() {
		t.Error("Available() = true with no plugins, want false")
	}

	// Only a detector.
	r.Register(newDetector("D", true))
	if r.Available() {
		t.Error("Available() = true with only detector, want false")
	}

	// Add a quality plugin → now available.
	r.Register(newQuality("Q", true))
	if !r.Available() {
		t.Error("Available() = false with both detector and quality, want true")
	}
}

func TestRegistry_Available_UnavailablePluginsDontCount(t *testing.T) {
	r := NewRegistry()
	r.Register(newDetector("D", false)) // not available
	r.Register(newQuality("Q", true))

	if r.Available() {
		t.Error("Available() = true when detector is unavailable, want false")
	}
}

func TestRegistry_AllModels_Aggregates(t *testing.T) {
	r := NewRegistry()

	p1 := &mockPlugin{
		name:     "P1",
		category: CategoryDetection,
		models: []ModelSpec{
			{Name: "ModelA", Filename: "a.onnx"},
			{Name: "ModelB", Filename: "b.onnx"},
		},
	}
	p2 := &mockPlugin{
		name:     "P2",
		category: CategoryQuality,
		models: []ModelSpec{
			{Name: "ModelC", Filename: "c.onnx"},
		},
	}
	r.Register(p1)
	r.Register(p2)

	models := r.AllModels()
	if len(models) != 3 {
		t.Errorf("AllModels() len = %d, want 3", len(models))
	}
}

func TestRegistry_InitAll_CriticalError(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockPlugin{
		name:     "BrokenDetector",
		category: CategoryDetection,
		initErr:  errors.New("model file missing"),
	})

	if err := r.InitAll("/lib"); err == nil {
		t.Error("InitAll() = nil, want error for failed Detection plugin")
	}
}

func TestRegistry_InitAll_RecognitionErrorSkipped(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockPlugin{
		name:     "BrokenRecognition",
		category: CategoryRecognition,
		initErr:  errors.New("optional model missing"),
	})

	if err := r.InitAll("/lib"); err != nil {
		t.Errorf("InitAll() = %v, want nil (recognition errors are non-fatal)", err)
	}
}

func TestRegistry_PluginStatuses(t *testing.T) {
	r := NewRegistry()
	r.Register(newDetector("D", true))
	r.Register(newQuality("Q", false))

	statuses := r.PluginStatuses()
	if len(statuses) != 2 {
		t.Fatalf("PluginStatuses() len = %d, want 2", len(statuses))
	}

	byName := make(map[string]PluginStatus, len(statuses))
	for _, s := range statuses {
		byName[s.Name] = s
	}

	if s := byName["D"]; !s.Available || s.Category != "detection" {
		t.Errorf("status D = %+v, want available detection", s)
	}
	if s := byName["Q"]; s.Available || s.Category != "quality" {
		t.Errorf("status Q = %+v, want unavailable quality", s)
	}
}

func TestRegistry_CloseAll_LogsErrors(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockPlugin{
		name:     "CloseErr",
		category: CategoryDetection,
		closeErr: errors.New("close failed"),
	})
	r.Register(newQuality("Q", true))

	// Should not panic and should not return an error.
	r.CloseAll()
}
