package video

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// createMockBin creates a temporary shell script that prints the given output.
func createMockBin(t *testing.T, output string) string {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "mock_bin.sh")

	// The script will just print the output and exit with 0.
	scriptContent := fmt.Sprintf(`#!/bin/sh
echo "%s"
exit 0
`, output)

	if err := os.WriteFile(binPath, []byte(scriptContent), 0o755); err != nil {
		t.Fatalf("Failed to create mock bin: %v", err)
	}

	return binPath
}

func TestGetDuration(t *testing.T) {
	// Mock ffprobe to output "12.34" just like it would for duration
	mockFFprobe := createMockBin(t, "12.34")

	// Temporarily replace the global ffprobePath
	originalPath := ffprobePath
	ffprobePath = mockFFprobe
	defer func() { ffprobePath = originalPath }()

	duration, err := GetDuration("dummy_video.jpg")
	if err != nil {
		t.Fatalf("GetDuration failed: %v", err)
	}

	if duration != 12.34 {
		t.Errorf("Expected duration to be 12.34, got %f", duration)
	}
}

func TestExtractThumbnail(t *testing.T) {
	// Mock ffmpeg to just return success
	mockFFmpeg := createMockBin(t, "success")

	originalPath := ffmpegPath
	ffmpegPath = mockFFmpeg
	defer func() { ffmpegPath = originalPath }()

	err := ExtractThumbnail("in.mp4", "out.jpg")
	if err != nil {
		t.Fatalf("ExtractThumbnail failed: %v", err)
	}
}

func TestTrimVideo(t *testing.T) {
	// Mock ffmpeg to just return success
	mockFFmpeg := createMockBin(t, "trim success")

	originalPath := ffmpegPath
	ffmpegPath = mockFFmpeg
	defer func() { ffmpegPath = originalPath }()

	err := TrimVideo("in.mp4", "out.mp4", 1.5, 5.0)
	if err != nil {
		t.Fatalf("TrimVideo failed: %v", err)
	}
}
