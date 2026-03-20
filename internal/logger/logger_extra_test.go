package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitRelativePath(t *testing.T) {
	// Use a relative filename and verify LogPath becomes absolute.
	const relName = "test_relative.log"

	if err := Init(relName); err != nil {
		t.Fatalf("Init with relative path failed: %v", err)
	}
	defer os.Remove(LogPath) // clean up

	if !filepath.IsAbs(LogPath) {
		t.Errorf("Expected LogPath to be absolute, got %s", LogPath)
	}

	if !strings.HasSuffix(LogPath, relName) {
		t.Errorf("Expected LogPath to end with %s, got %s", relName, LogPath)
	}
}

func TestInitInvalidPath(t *testing.T) {
	err := Init("/nonexistent/dir/file.log")
	if err == nil {
		t.Fatal("Expected Init to return an error for an invalid path, got nil")
	}
}

func TestInitWritesJSONFormat(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "json_test.log")

	if err := Init(logFile); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	Log.Info("json format check")

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `"msg":`) {
		t.Errorf("Expected JSON log output containing \"msg\":, got:\n%s", content)
	}
}

func TestOpenLogFile(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "open_test.log")

	if err := Init(logFile); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// OpenLogFile shells out to an OS command; we just verify it does not panic.
	// The returned error is acceptable (e.g. no display in CI).
	_ = OpenLogFile()
}

func TestMultipleInitCalls(t *testing.T) {
	dir := t.TempDir()

	first := filepath.Join(dir, "first.log")
	second := filepath.Join(dir, "second.log")

	if err := Init(first); err != nil {
		t.Fatalf("First Init failed: %v", err)
	}
	if LogPath != first {
		t.Errorf("Expected LogPath %s after first Init, got %s", first, LogPath)
	}

	if err := Init(second); err != nil {
		t.Fatalf("Second Init failed: %v", err)
	}
	if LogPath != second {
		t.Errorf("Expected LogPath %s after second Init, got %s", second, LogPath)
	}

	// Verify the second log file is functional.
	Log.Info("after second init")

	data, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("Failed to read second log file: %v", err)
	}
	if len(data) == 0 {
		t.Error("Expected second log file to have content, but it is empty")
	}
}
