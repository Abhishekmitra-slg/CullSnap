package app

import (
	"os"
	"path/filepath"
	"testing"

	"cullsnap/internal/storage"
)

func newTestStore(t *testing.T) (*storage.SQLiteStore, error) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	return storage.NewSQLiteStore(dbPath)
}

func TestParseContributors_SingleEntry(t *testing.T) {
	raw := `- name: Alice Smith
  github: alicesmith
  role: Creator
  bio: Builds great tools.
`
	contributors := parseContributors(raw)
	if len(contributors) != 1 {
		t.Fatalf("expected 1 contributor, got %d", len(contributors))
	}
	c := contributors[0]
	if c.Name != "Alice Smith" {
		t.Errorf("expected name 'Alice Smith', got %q", c.Name)
	}
	if c.GitHub != "alicesmith" {
		t.Errorf("expected github 'alicesmith', got %q", c.GitHub)
	}
	if c.Role != "Creator" {
		t.Errorf("expected role 'Creator', got %q", c.Role)
	}
	if c.Bio != "Builds great tools." {
		t.Errorf("expected bio 'Builds great tools.', got %q", c.Bio)
	}
	if c.Avatar != "https://github.com/alicesmith.png" {
		t.Errorf("expected avatar URL, got %q", c.Avatar)
	}
}

func TestParseContributors_MultipleEntries(t *testing.T) {
	raw := `- name: Alice
  github: alice
  role: Lead
  bio: First contributor.
- name: Bob
  github: bob
  role: Contributor
  bio: Second contributor.
`
	contributors := parseContributors(raw)
	if len(contributors) != 2 {
		t.Fatalf("expected 2 contributors, got %d", len(contributors))
	}
	if contributors[0].Name != "Alice" {
		t.Errorf("first contributor name: got %q", contributors[0].Name)
	}
	if contributors[1].Name != "Bob" {
		t.Errorf("second contributor name: got %q", contributors[1].Name)
	}
	if contributors[1].GitHub != "bob" {
		t.Errorf("second contributor github: got %q", contributors[1].GitHub)
	}
}

func TestParseContributors_EmptyInput(t *testing.T) {
	contributors := parseContributors("")
	if len(contributors) != 0 {
		t.Errorf("expected 0 contributors for empty input, got %d", len(contributors))
	}
}

func TestParseContributors_MissingFields(t *testing.T) {
	raw := `- name: Minimal
  github: minimal
`
	contributors := parseContributors(raw)
	if len(contributors) != 1 {
		t.Fatalf("expected 1 contributor, got %d", len(contributors))
	}
	if contributors[0].Role != "" {
		t.Errorf("expected empty role, got %q", contributors[0].Role)
	}
	if contributors[0].Bio != "" {
		t.Errorf("expected empty bio, got %q", contributors[0].Bio)
	}
}

func TestParseContributors_WithYAMLSeparator(t *testing.T) {
	raw := `---
- name: Test
  github: test
  role: Dev
  bio: Testing.
`
	contributors := parseContributors(raw)
	if len(contributors) != 1 {
		t.Fatalf("expected 1 contributor, got %d", len(contributors))
	}
	if contributors[0].Name != "Test" {
		t.Errorf("expected name 'Test', got %q", contributors[0].Name)
	}
}

func TestGetAboutInfo(t *testing.T) {
	store, err := newTestStore(t)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer func() { _ = store.Close() }()

	a := NewApp(store)
	a.Version = "v1.0.0-test"
	a.ContributorsRaw = `- name: Tester
  github: tester
  role: QA
  bio: Tests things.
`
	a.cfg = &AppConfig{}

	info := a.GetAboutInfo()

	if info.Version != "v1.0.0-test" {
		t.Errorf("expected version 'v1.0.0-test', got %q", info.Version)
	}
	if info.License != "AGPL-3.0" {
		t.Errorf("expected license 'AGPL-3.0', got %q", info.License)
	}
	if info.GoVersion == "" {
		t.Error("expected non-empty Go version")
	}
	if info.SQLiteVersion == "" {
		t.Error("expected non-empty SQLite version")
	}
	if info.Repo != "https://github.com/Abhishekmitra-slg/CullSnap" {
		t.Errorf("expected repo URL, got %q", info.Repo)
	}
	if len(info.Contributors) != 1 {
		t.Fatalf("expected 1 contributor, got %d", len(info.Contributors))
	}
	if info.Contributors[0].Name != "Tester" {
		t.Errorf("expected contributor 'Tester', got %q", info.Contributors[0].Name)
	}
}

func TestGetAboutInfo_NoContributors(t *testing.T) {
	store, err := newTestStore(t)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer func() { _ = store.Close() }()

	a := NewApp(store)
	a.Version = "dev"
	a.ContributorsRaw = ""
	a.cfg = &AppConfig{}

	info := a.GetAboutInfo()
	if len(info.Contributors) != 0 {
		t.Errorf("expected 0 contributors, got %d", len(info.Contributors))
	}
}

func TestGetSQLiteVersion(t *testing.T) {
	store, err := newTestStore(t)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer func() { _ = store.Close() }()

	ver, err := store.GetSQLiteVersion()
	if err != nil {
		t.Fatalf("GetSQLiteVersion failed: %v", err)
	}
	if ver == "" {
		t.Error("expected non-empty SQLite version")
	}
	// SQLite version format: "3.x.y"
	if len(ver) < 3 || ver[0] != '3' {
		t.Errorf("unexpected SQLite version format: %q", ver)
	}
}

// newTestStore helper is unused warning suppressor
var _ = os.Remove
