package logger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInit(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	if err := Init(logFile); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if Log == nil {
		t.Fatal("Expected Log to be initialized, got nil")
	}

	if LogPath != logFile {
		t.Errorf("Expected LogPath to be %s, got %s", logFile, LogPath)
	}

	// Write a test log
	Log.Info("Test message")

	// Verify file was created and written to
	info, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("Failed to stat log file: %v", err)
	}

	if info.Size() == 0 {
		t.Errorf("Expected log file to have content, but size is 0")
	}
}
