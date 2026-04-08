package scanner

import (
	"cullsnap/internal/model"
	"fmt"
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
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("Failed to create subdirectories: %v", err)
		}
		if err := os.WriteFile(path, []byte("dummy content"), 0o644); err != nil {
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
	if err := os.WriteFile(fakeMp4, []byte("fake"), 0o644); err != nil {
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

func TestScanDirectoryStream(t *testing.T) {
	dir := t.TempDir()

	// Create 7 test files across photo and video types
	names := []string{
		"a.jpg", "b.jpg", "c.png", "d.jpeg",
		"e.jpg", "f.mp4", "g.jpg",
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Non-image file should be skipped
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("skip"), 0o644)

	var batches [][]model.Photo
	err := ScanDirectoryStream(dir, 3, func(batch []model.Photo, done bool) {
		cp := make([]model.Photo, len(batch))
		copy(cp, batch)
		batches = append(batches, cp)
	})
	if err != nil {
		t.Fatal(err)
	}

	// 7 files / batch size 3 = 3 batches (3, 3, 1)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(batches))
	}
	if len(batches[0]) != 3 {
		t.Errorf("batch 0: expected 3 items, got %d", len(batches[0]))
	}
	if len(batches[1]) != 3 {
		t.Errorf("batch 1: expected 3 items, got %d", len(batches[1]))
	}
	if len(batches[2]) != 1 {
		t.Errorf("batch 2: expected 1 item, got %d", len(batches[2]))
	}

	// Total count should be 7
	total := 0
	for _, b := range batches {
		total += len(b)
	}
	if total != 7 {
		t.Errorf("expected 7 total photos, got %d", total)
	}
}

func TestScanDirectoryStream_SmallDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "only.jpg"), []byte("fake"), 0o644)

	var batches [][]model.Photo
	err := ScanDirectoryStream(dir, 50, func(batch []model.Photo, done bool) {
		cp := make([]model.Photo, len(batch))
		copy(cp, batch)
		batches = append(batches, cp)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Single file should produce exactly 1 batch with done=true
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
	if len(batches[0]) != 1 {
		t.Errorf("expected 1 photo, got %d", len(batches[0]))
	}
}

func TestScanDirectoryStream_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	var called int
	err := ScanDirectoryStream(dir, 50, func(batch []model.Photo, done bool) {
		called++
	})
	if err != nil {
		t.Fatal(err)
	}
	// Empty dir should still call batchFn once with done=true and empty batch
	if called != 1 {
		t.Errorf("expected 1 callback for empty dir, got %d", called)
	}
}

func TestScanDirectoryStream_DoneFlag(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("img%d.jpg", i)), []byte("x"), 0o644)
	}

	var doneValues []bool
	err := ScanDirectoryStream(dir, 3, func(batch []model.Photo, done bool) {
		doneValues = append(doneValues, done)
	})
	if err != nil {
		t.Fatal(err)
	}

	// 5 files / 3 = 2 batches. Only the last should have done=true
	if len(doneValues) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(doneValues))
	}
	if doneValues[0] {
		t.Error("first batch should have done=false")
	}
	if !doneValues[1] {
		t.Error("last batch should have done=true")
	}
}
