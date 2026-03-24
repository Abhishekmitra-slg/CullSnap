//go:build darwin

package heic

import (
	"cullsnap/internal/logger"
	"fmt"
	"os"
	"os/exec"
)

func convertPlatform(heicPath, jpegPath string, useSips bool) error {
	if useSips {
		err := convertSips(heicPath, jpegPath)
		if err == nil {
			return nil
		}
		logger.Log.Debug("heic: sips failed, falling back to ffmpeg", "error", err)
	}
	return convertFFmpeg(heicPath, jpegPath)
}

func convertSips(heicPath, jpegPath string) error {
	cmd := exec.Command("sips", "-s", "format", "jpeg", heicPath, "--out", jpegPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sips failed: %w (output: %s)", err, string(out))
	}
	info, err := os.Stat(jpegPath)
	if err != nil || info.Size() == 0 {
		_ = os.Remove(jpegPath)
		return fmt.Errorf("sips produced empty output for %s", heicPath)
	}
	logger.Log.Debug("heic: sips conversion complete", "input", heicPath, "outputSize", info.Size())
	return nil
}
