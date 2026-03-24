package heic

import (
	"cullsnap/internal/logger"
	"cullsnap/internal/video"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ConvertToJPEG converts a HEIC/HEIF file to JPEG.
// If useSips is true (macOS only), it tries sips first with FFmpeg fallback.
func ConvertToJPEG(heicPath, jpegPath string, useSips bool) error {
	ext := strings.ToLower(filepath.Ext(heicPath))
	if ext != ".heic" && ext != ".heif" {
		return fmt.Errorf("heic: unsupported extension %q", ext)
	}
	if _, err := os.Stat(heicPath); err != nil {
		return fmt.Errorf("heic: input file not found: %w", err)
	}
	outDir := filepath.Dir(jpegPath)
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return fmt.Errorf("heic: cannot create output dir: %w", err)
	}
	logger.Log.Debug("heic: converting", "input", heicPath, "output", jpegPath, "useSips", useSips)
	return convertPlatform(heicPath, jpegPath, useSips)
}

// convertFFmpeg is the shared FFmpeg conversion used by all platforms.
func convertFFmpeg(heicPath, jpegPath string) error {
	ffmpegBin := video.FFmpegPath()
	if ffmpegBin == "" {
		return fmt.Errorf("heic: ffmpeg not available for HEIC conversion")
	}
	cmd := exec.Command(ffmpegBin, "-i", heicPath, "-frames:v", "1", "-y", jpegPath) // #nosec G204 -- ffmpegBin is resolved internally, not user input
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %w (output: %s)", err, string(out))
	}
	info, err := os.Stat(jpegPath)
	if err != nil || info.Size() == 0 {
		_ = os.Remove(jpegPath)
		return fmt.Errorf("ffmpeg produced empty output for %s", heicPath)
	}
	logger.Log.Debug("heic: ffmpeg conversion complete", "input", heicPath, "outputSize", info.Size())
	return nil
}
