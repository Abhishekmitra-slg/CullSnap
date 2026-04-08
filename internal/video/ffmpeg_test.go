package video

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveFFmpegURLs_KnownPlatforms(t *testing.T) {
	tests := []struct {
		goos   string
		goarch string
	}{
		{"darwin", "arm64"},
		{"darwin", "amd64"},
		{"linux", "amd64"},
		{"windows", "amd64"},
	}

	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			result := resolveFFmpegURLs(tt.goos, tt.goarch)
			if result.err != nil {
				t.Errorf("unexpected error: %v", result.err)
			}
			if result.ffmpegURL == "" {
				t.Error("ffmpegURL is empty")
			}
			if result.ffprobeURL == "" {
				t.Error("ffprobeURL is empty")
			}
			if !strings.HasPrefix(result.ffmpegURL, "https://") {
				t.Errorf("ffmpegURL is not a valid HTTPS URL: %s", result.ffmpegURL)
			}
			if !strings.HasPrefix(result.ffprobeURL, "https://") {
				t.Errorf("ffprobeURL is not a valid HTTPS URL: %s", result.ffprobeURL)
			}
			if !result.isGz {
				t.Error("isGz should be true for all platforms (eugeneware/ffmpeg-static uses .gz)")
			}
		})
	}
}

func TestResolveFFmpegURLs_UnsupportedPlatform(t *testing.T) {
	result := resolveFFmpegURLs("plan9", "arm64")
	if result.err == nil {
		t.Error("expected error for unsupported platform, got nil")
	}
	if result.ffmpegURL != "" {
		t.Errorf("expected empty ffmpegURL for unsupported platform, got %s", result.ffmpegURL)
	}
}

func TestResolveFFmpegURLs_URLContainsPlatformSlug(t *testing.T) {
	tests := []struct {
		goos        string
		goarch      string
		ffmpegSlug  string
		ffprobeSlug string
	}{
		{"darwin", "arm64", "ffmpeg-darwin-arm64", "ffprobe-darwin-arm64"},
		{"darwin", "amd64", "ffmpeg-darwin-x64", "ffprobe-darwin-x64"},
		{"linux", "amd64", "ffmpeg-linux-x64", "ffprobe-linux-x64"},
		{"windows", "amd64", "ffmpeg-win32-x64", "ffprobe-win32-x64"},
	}

	for _, tt := range tests {
		t.Run(tt.goos+"/"+tt.goarch, func(t *testing.T) {
			result := resolveFFmpegURLs(tt.goos, tt.goarch)
			if result.err != nil {
				t.Fatalf("unexpected error: %v", result.err)
			}
			if !strings.Contains(result.ffmpegURL, tt.ffmpegSlug) {
				t.Errorf("ffmpegURL %q does not contain expected slug %q", result.ffmpegURL, tt.ffmpegSlug)
			}
			if !strings.Contains(result.ffprobeURL, tt.ffprobeSlug) {
				t.Errorf("ffprobeURL %q does not contain expected slug %q", result.ffprobeURL, tt.ffprobeSlug)
			}
		})
	}
}

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
