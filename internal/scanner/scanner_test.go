package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDirectory(t *testing.T) {
	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "cullsnap_scan_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create dummy files
	files := []string{
		"test1.jpg",
		"test2.png",
		"ignore.txt",
		"subfolder/test3.jpeg",
	}

	for _, f := range files {
		path := filepath.Join(tempDir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("Failed to create subdirectories: %v", err)
		}
		if err := os.WriteFile(path, []byte("dummy content"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", path, err)
		}
	}

	// Test ScanDirectory
	photos, err := ScanDirectory(tempDir)
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}

	// Expect 3 photos (jpg, png, jpeg), ignore txt
	if len(photos) != 3 {
		t.Errorf("Expected 3 photos, got %d", len(photos))
	}

	// Verify paths are absolute or correct relative to scan root
	found := make(map[string]bool)
	for _, p := range photos {
		found[filepath.Base(p.Path)] = true
	}

	if !found["test1.jpg"] || !found["test2.png"] || !found["test3.jpeg"] {
		t.Error("Missing expected files in scan results")
	}
}
