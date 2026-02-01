package storage

import (
	"os"
	"testing"
)

func TestSQLiteStore(t *testing.T) {
	dbPath := "test_cullsnap.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	sessionID := "test-session"
	path1 := "/path/to/photo1.jpg"
	path2 := "/path/to/photo2.jpg"

	// Test SaveSelection (Select)
	if err := store.SaveSelection(path1, sessionID, true); err != nil {
		t.Fatalf("Failed to save selection: %v", err)
	}
	if err := store.SaveSelection(path2, sessionID, true); err != nil {
		t.Fatalf("Failed to save selection: %v", err)
	}

	// Test GetSelections
	selections, err := store.GetSelections(sessionID)
	if err != nil {
		t.Fatalf("Failed to get selections: %v", err)
	}

	if len(selections) != 2 {
		t.Errorf("Expected 2 selections, got %d", len(selections))
	}
	if !selections[path1] || !selections[path2] {
		t.Errorf("Selections map is missing expected paths")
	}

	// Test SaveSelection (Deselect)
	if err := store.SaveSelection(path1, sessionID, false); err != nil {
		t.Fatalf("Failed to deselect: %v", err)
	}

	selections, err = store.GetSelections(sessionID)
	if err != nil {
		t.Fatalf("Failed to get selections: %v", err)
	}

	if len(selections) != 1 {
		t.Errorf("Expected 1 selection, got %d", len(selections))
	}
	if selections[path1] {
		t.Errorf("Path1 should be deselected")
	}
}
