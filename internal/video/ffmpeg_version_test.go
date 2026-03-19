package video

import (
	"testing"
)

func TestGetFFmpegVersion_NotInstalled(t *testing.T) {
	// Save and clear ffmpegPath to simulate FFmpeg not being installed
	original := ffmpegPath
	ffmpegPath = ""
	defer func() { ffmpegPath = original }()

	ver := GetFFmpegVersion()
	if ver != "not installed" {
		t.Errorf("expected 'not installed' when ffmpegPath is empty, got %q", ver)
	}
}

func TestGetFFmpegVersion_InvalidPath(t *testing.T) {
	original := ffmpegPath
	ffmpegPath = "/nonexistent/ffmpeg"
	defer func() { ffmpegPath = original }()

	ver := GetFFmpegVersion()
	if ver != "not installed" {
		t.Errorf("expected 'not installed' for invalid path, got %q", ver)
	}
}
