package video

import (
	"testing"
)

func TestFFmpegPath_Default(t *testing.T) {
	// When not initialized, FFmpegPath returns the module-level default
	original := ffmpegPath
	ffmpegPath = ""
	defer func() { ffmpegPath = original }()

	if got := FFmpegPath(); got != "" {
		t.Errorf("FFmpegPath() = %q, want empty string when not initialized", got)
	}
}

func TestFFmpegPath_AfterSet(t *testing.T) {
	original := ffmpegPath
	ffmpegPath = "/usr/local/bin/ffmpeg"
	defer func() { ffmpegPath = original }()

	if got := FFmpegPath(); got != "/usr/local/bin/ffmpeg" {
		t.Errorf("FFmpegPath() = %q, want %q", got, "/usr/local/bin/ffmpeg")
	}
}

func TestTrimVideo_NoFFmpeg(t *testing.T) {
	original := ffmpegPath
	ffmpegPath = ""
	defer func() { ffmpegPath = original }()

	err := TrimVideo("in.mp4", "out.mp4", 0, 5)
	if err == nil {
		t.Error("expected error when ffmpeg not available")
	}
}

func TestGetFFmpegVersion_MockBin(t *testing.T) {
	mockBin := createMockBin(t, "ffmpeg version 6.1.1 Copyright (c) 2000-2023")

	original := ffmpegPath
	ffmpegPath = mockBin
	defer func() { ffmpegPath = original }()

	ver := GetFFmpegVersion()
	if ver != "6.1.1" {
		t.Errorf("expected '6.1.1', got %q", ver)
	}
}

func TestGetDuration_InvalidOutput(t *testing.T) {
	mockBin := createMockBin(t, "not_a_number")

	original := ffprobePath
	ffprobePath = mockBin
	defer func() { ffprobePath = original }()

	_, err := GetDuration("test.mp4")
	if err == nil {
		t.Error("expected error for non-numeric duration output")
	}
}
