package video

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"cullsnap/internal/logger"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	thumbnailTimeout = 60 * time.Second
	durationTimeout  = 30 * time.Second
	trimTimeout      = 10 * time.Minute
	versionTimeout   = 10 * time.Second
)

var (
	binDir      string
	ffmpegPath  string
	ffprobePath string
)

// Init initializes the video package by ensuring FFmpeg is available.
func Init() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	binDir = filepath.Join(home, ".cullsnap", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}

	ffmpegPath = filepath.Join(binDir, "ffmpeg"+ext)
	ffprobePath = filepath.Join(binDir, "ffprobe"+ext)

	// Check if already installed
	if _, err := os.Stat(ffmpegPath); err == nil {
		if _, err := os.Stat(ffprobePath); err == nil {
			return nil // Already installed
		}
	}

	// Download from ffbinaries
	return downloadFFmpeg()
}

// Structs for ffbinaries API
type ffbinariesResponse struct {
	Bin struct {
		Windows64 struct {
			Ffmpeg  string `json:"ffmpeg"`
			Ffprobe string `json:"ffprobe"`
		} `json:"windows-64"`
		MacArm struct {
			Ffmpeg  string `json:"ffmpeg"`
			Ffprobe string `json:"ffprobe"`
		} `json:"osx-arm-64"`
		Mac64 struct {
			Ffmpeg  string `json:"ffmpeg"`
			Ffprobe string `json:"ffprobe"`
		} `json:"osx-64"`
		Linux64 struct {
			Ffmpeg  string `json:"ffmpeg"`
			Ffprobe string `json:"ffprobe"`
		} `json:"linux-64"`
	} `json:"bin"`
}

func downloadFFmpeg() error {
	fmt.Println("Downloading FFmpeg for video support... This may take a minute.")

	var ffmpegURL, ffprobeURL string
	isGz := false

	// Handle Apple Silicon Native
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		logger.Log.Info("Detected Apple Silicon, downloading native ARM64 binary")
		// Using eugeneware/ffmpeg-static releases for native arm64 (both binaries are here)
		ffmpegURL = "https://github.com/eugeneware/ffmpeg-static/releases/download/b6.1.1/ffmpeg-darwin-arm64.gz"
		ffprobeURL = "https://github.com/eugeneware/ffmpeg-static/releases/download/b6.1.1/ffprobe-darwin-arm64.gz"
		isGz = true
	} else {
		resp, err := http.Get("https://ffbinaries.com/api/v1/version/latest")
		if err != nil {
			return fmt.Errorf("failed to fetch ffbinaries API: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		var apiResp ffbinariesResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
			return fmt.Errorf("failed to decode ffbinaries API: %w", err)
		}

		switch runtime.GOOS {
		case "windows":
			ffmpegURL = apiResp.Bin.Windows64.Ffmpeg
			ffprobeURL = apiResp.Bin.Windows64.Ffprobe
		case "darwin":
			ffmpegURL = apiResp.Bin.Mac64.Ffmpeg
			ffprobeURL = apiResp.Bin.Mac64.Ffprobe
		case "linux":
			ffmpegURL = apiResp.Bin.Linux64.Ffmpeg
			ffprobeURL = apiResp.Bin.Linux64.Ffprobe
		default:
			return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
		}
	}

	if isGz {
		if err := downloadAndExtractGz(ffmpegURL, ffmpegPath); err != nil {
			return err
		}
		if err := downloadAndExtractGz(ffprobeURL, ffprobePath); err != nil {
			// If native probe fails, fallback to x64 for probe is fine
			logger.Log.Warn("Native ffprobe download failed, skipping (using ffmpeg only for now or existing)")
		}
	} else {
		if err := downloadAndExtractZip(ffmpegURL, ffmpegPath, "ffmpeg"); err != nil {
			return err
		}
		if err := downloadAndExtractZip(ffprobeURL, ffprobePath, "ffprobe"); err != nil {
			return err
		}
	}

	fmt.Println("FFmpeg downloaded successfully.")
	return nil
}

func downloadAndExtractGz(url, destPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download %s: %s", url, resp.Status)
	}

	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer func() { _ = outFile.Close() }()

	if _, err := io.Copy(outFile, gzReader); err != nil {
		return err
	}
	return nil
}

func downloadAndExtractZip(url, destPath, binName string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// ffbinaries serves zip files
	zipReader, err := zip.NewReader(bytes.NewReader(bodyBytes), int64(len(bodyBytes)))
	if err != nil {
		return fmt.Errorf("failed to read zip: %w", err)
	}

	for _, file := range zipReader.File {
		if strings.HasPrefix(file.Name, binName) {
			return extractZipEntry(file, destPath)
		}
	}

	return fmt.Errorf("executable %s not found in zip", binName)
}

// extractZipEntry extracts a single zip file entry to destPath.
func extractZipEntry(file *zip.File, destPath string) error {
	zippedFile, err := file.Open()
	if err != nil {
		return err
	}
	defer func() { _ = zippedFile.Close() }()

	outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer func() { _ = outFile.Close() }()

	if _, err := io.Copy(outFile, zippedFile); err != nil {
		return err
	}
	return nil
}

// FFmpegPath returns the resolved path to the ffmpeg binary, or "" if not available.
func FFmpegPath() string {
	return ffmpegPath
}

// GetDuration returns the duration of a video in seconds.
func GetDuration(path string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), durationTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffprobePath, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	strOut := strings.TrimSpace(string(out))
	duration, err := strconv.ParseFloat(strOut, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}
	return duration, nil
}

// ExtractThumbnail extracts the first frame of a video to a standard JPEG file.
func ExtractThumbnail(videoPath, outPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), thumbnailTimeout)
	defer cancel()

	// Move -ss before -i for fast seeking. Use -update 1 to treat output as a single image.
	// Explicitly set -f mjpeg because output path ends in .tmp
	cmd := exec.CommandContext(ctx, ffmpegPath, "-y", "-ss", "0.5", "-i", videoPath, "-vframes", "1", "-update", "1", "-q:v", "2", "-f", "mjpeg", outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Log.Debug("FFmpeg thumbnail extraction failed at 0.5s", "error", err, "output", string(out))
		// Fallback to start of file — reuse same timeout context.
		cmdFallback := exec.CommandContext(ctx, ffmpegPath, "-y", "-i", videoPath, "-vframes", "1", "-update", "1", "-q:v", "2", "-f", "mjpeg", outPath)
		if outF, errF := cmdFallback.CombinedOutput(); errF != nil {
			logger.Log.Debug("FFmpeg thumbnail extraction fallback failed", "error", errF, "output", string(outF))
			return errF
		}
	}
	return nil
}

// TrimVideo losslessly trims a video.
func TrimVideo(src, dest string, start, end float64) error {
	if ffmpegPath == "" {
		return fmt.Errorf("ffmpeg not available — cannot trim video")
	}
	startStr := fmt.Sprintf("%.3f", start)
	endStr := fmt.Sprintf("%.3f", end)

	ctx, cancel := context.WithTimeout(context.Background(), trimTimeout)
	defer cancel()

	// Place -ss before -i for fast seeking (same as ExtractThumbnail).
	// With input-side seek, -to is an absolute output timestamp which is correct
	// since TrimEnd is measured from the start of the file.
	cmd := exec.CommandContext(ctx, ffmpegPath, "-y", "-ss", startStr, "-i", src, "-to", endStr, "-c", "copy", dest)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg trim failed: %s", string(out))
	}
	return nil
}

// GetFFmpegVersion returns the FFmpeg version string, or "not installed" if unavailable.
func GetFFmpegVersion() string {
	if ffmpegPath == "" {
		return "not installed"
	}
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpegPath, "-version")
	out, err := cmd.Output()
	if err != nil {
		return "not installed"
	}
	// First line: "ffmpeg version N.N.N ..."
	firstLine := strings.SplitN(string(out), "\n", 2)[0]
	parts := strings.Fields(firstLine)
	if len(parts) >= 3 {
		return parts[2]
	}
	return firstLine
}
