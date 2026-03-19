package export

import (
	"cullsnap/internal/model"
	"os"
	"path/filepath"
	"testing"
)

func TestExportSelections(t *testing.T) {
	// Setup Source Directory
	srcDir, err := os.MkdirTemp("", "cullsnap_export_src")
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	defer os.RemoveAll(srcDir)

	// Setup Destination Directory
	destDir, err := os.MkdirTemp("", "cullsnap_export_dest")
	if err != nil {
		t.Fatalf("Failed to create dest dir: %v", err)
	}
	defer os.RemoveAll(destDir)

	// Create source files
	srcFiles := []string{"photo1.jpg", "photo2.jpg"}
	var photos []model.Photo

	for _, f := range srcFiles {
		path := filepath.Join(srcDir, f)
		if err := os.WriteFile(path, []byte("test data"), 0o644); err != nil {
			t.Fatalf("Failed to create source file: %v", err)
		}
		photos = append(photos, model.Photo{Path: path})
	}

	// Test Export
	count, err := ExportSelections(photos, destDir)
	if err != nil {
		t.Fatalf("ExportSelections failed: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected 2 exported files, got %d", count)
	}

	// Verify files existence in dest
	for _, f := range srcFiles {
		destPath := filepath.Join(destDir, f)
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			t.Errorf("File %s was not copied to destination", f)
		}
	}
}

func TestUniquePath(t *testing.T) {
	// Setup temp dir
	tempDir, err := os.MkdirTemp("", "cullsnap_unique_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	baseFile := filepath.Join(tempDir, "test.jpg")
	os.WriteFile(baseFile, []byte{}, 0o644)

	// First collision
	newPath := uniquePath(baseFile)
	expected1 := filepath.Join(tempDir, "test_1.jpg")
	if newPath != expected1 {
		t.Errorf("Expected %s, got %s", expected1, newPath)
	}

	// Create the colliding file
	os.WriteFile(expected1, []byte{}, 0o644)

	// Second collision
	newPath2 := uniquePath(baseFile)
	expected2 := filepath.Join(tempDir, "test_2.jpg")
	if newPath2 != expected2 {
		t.Errorf("Expected %s, got %s", expected2, newPath2)
	}
}
