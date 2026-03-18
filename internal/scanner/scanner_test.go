package scanner

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestScanDirectory_ReturnsBeforeFFprobe(t *testing.T) {
	dir := t.TempDir()
	fakeMp4 := filepath.Join(dir, "test.mp4")
	if err := os.WriteFile(fakeMp4, []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	photos, err := ScanDirectory(dir)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("ScanDirectory returned error: %v", err)
	}
	if len(photos) != 1 {
		t.Fatalf("Expected 1 photo, got %d", len(photos))
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("ScanDirectory took %v — expected < 500ms (ffprobe should not be called inline)", elapsed)
	}
	if photos[0].Duration != 0 {
		t.Errorf("Expected Duration=0 (not yet enriched), got %f", photos[0].Duration)
	}
}
