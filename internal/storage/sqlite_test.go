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

func TestRecents(t *testing.T) {
	dbPath := "test_recents.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	if err := store.AddRecent("/foo/bar"); err != nil {
		t.Fatalf("Failed to add recent: %v", err)
	}

	if err := store.AddRecent("/foo/baz"); err != nil {
		t.Fatalf("Failed to add recent: %v", err)
	}

	recents, err := store.GetRecents()
	if err != nil {
		t.Fatalf("Failed to get recents: %v", err)
	}

	if len(recents) != 2 {
		t.Errorf("Expected 2 recents, got %d", len(recents))
	}
	// /foo/baz was added last, should be first
	if recents[0] != "/foo/baz" {
		t.Errorf("Expected most recent to be /foo/baz")
	}
}

func TestExportedStatus(t *testing.T) {
	dbPath := "test_exported.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	dir := "/home/user/photos"
	file1 := dir + "/img1.jpg"
	file2 := dir + "/img2.jpg"

	if err := store.MarkExported(file1); err != nil {
		t.Fatalf("Failed to mark exported: %v", err)
	}

	exported, err := store.GetExportedInDirectory(dir)
	if err != nil {
		t.Fatalf("Failed to get exported: %v", err)
	}

	if len(exported) != 1 {
		t.Errorf("Expected 1 exported item, got %d", len(exported))
	}
	if !exported[file1] {
		t.Errorf("Expected file1 to be exported")
	}
	if exported[file2] {
		t.Errorf("Expected file2 to NOT be exported")
	}
}

func TestRatings(t *testing.T) {
	dbPath := "test_ratings.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	dir := "/home/user/images"
	img1 := dir + "/1.jpg"
	img2 := dir + "/2.jpg"

	if err := store.SaveRating(img1, 5); err != nil {
		t.Fatalf("Failed to save rating: %v", err)
	}
	if err := store.SaveRating(img2, 3); err != nil {
		t.Fatalf("Failed to save rating: %v", err)
	}

	ratings, err := store.GetRatingsInDirectory(dir)
	if err != nil {
		t.Fatalf("Failed to get ratings: %v", err)
	}

	if len(ratings) != 2 {
		t.Errorf("Expected 2 ratings, got %d", len(ratings))
	}

	if ratings[img1] != 5 {
		t.Errorf("Expected img1 rating 5, got %d", ratings[img1])
	}

	if err := store.SaveRating(img1, 0); err != nil {
		t.Fatalf("Failed to save rating (delete): %v", err)
	}

	ratings, _ = store.GetRatingsInDirectory(dir)
	if len(ratings) != 1 {
		t.Errorf("Expected 1 rating after deletion, got %d", len(ratings))
	}
}

