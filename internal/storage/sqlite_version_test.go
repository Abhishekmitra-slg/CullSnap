package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetSQLiteVersion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "version_test.db")
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ver, err := store.GetSQLiteVersion()
	if err != nil {
		t.Fatalf("GetSQLiteVersion failed: %v", err)
	}
	if ver == "" {
		t.Fatal("expected non-empty SQLite version")
	}
	// Version should be like "3.x.y"
	if !strings.HasPrefix(ver, "3.") {
		t.Errorf("expected version starting with '3.', got %q", ver)
	}
	t.Logf("SQLite version: %s", ver)
}
