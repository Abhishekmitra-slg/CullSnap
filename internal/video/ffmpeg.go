package video

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"cullsnap/internal/logger"
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

	// maxFFmpegBinaryBytes caps extraction size to 512 MB to prevent zip/gzip bomb DoS.
	maxFFmpegBinaryBytes = 512 * 1024 * 1024
)

var (
	binDir      string
	ffmpegPath  string
	ffprobePath string
)

// ffmpegURLs holds the resolved download URLs for ffmpeg and ffprobe.
type ffmpegURLs struct {
	ffmpegURL  string
	ffprobeURL string
	isGz       bool
	err        error
}

const ffmpegStaticBase = "https://github.com/eugeneware/ffmpeg-static/releases/download/b6.1.1/"

// resolveFFmpegURLs returns hardcoded download URLs for the given OS/arch combination.
// All platforms use eugeneware/ffmpeg-static GitHub release assets (.gz format).
func resolveFFmpegURLs(goos, goarch string) ffmpegURLs {
	type platformURLs struct {
		ffmpeg  string
		ffprobe string
	}

	// Map of "goos/goarch" -> asset name slugs from eugeneware/ffmpeg-static b6.1.1.
	platforms := map[string]platformURLs{
		"darwin/arm64":  {ffmpeg: "ffmpeg-darwin-arm64.gz", ffprobe: "ffprobe-darwin-arm64.gz"},
		"darwin/amd64":  {ffmpeg: "ffmpeg-darwin-x64.gz", ffprobe: "ffprobe-darwin-x64.gz"},
		"linux/amd64":   {ffmpeg: "ffmpeg-linux-x64.gz", ffprobe: "ffprobe-linux-x64.gz"},
		"windows/amd64": {ffmpeg: "ffmpeg-win32-x64.gz", ffprobe: "ffprobe-win32-x64.gz"},
	}

	key := goos + "/" + goarch
	p, ok := platforms[key]
	if !ok {
		return ffmpegURLs{err: fmt.Errorf("unsupported platform: %s/%s", goos, goarch)}
	}

	return ffmpegURLs{
		ffmpegURL:  ffmpegStaticBase + p.ffmpeg,
		ffprobeURL: ffmpegStaticBase + p.ffprobe,
		isGz:       true,
	}
}

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

	// Check if already installed.
	if _, err := os.Stat(ffmpegPath); err == nil {
		if _, err := os.Stat(ffprobePath); err == nil {
			return nil // Already installed
		}
	}

	return downloadFFmpeg()
}

func downloadFFmpeg() error {
	logger.Log.Info("Downloading FFmpeg for video support — this may take a minute")

	urls := resolveFFmpegURLs(runtime.GOOS, runtime.GOARCH)
	if urls.err != nil {
		return fmt.Errorf("cannot provision FFmpeg: %w", urls.err)
	}

	logger.Log.Info("Resolved FFmpeg download URLs",
		"goos", runtime.GOOS,
		"goarch", runtime.GOARCH,
		"ffmpegURL", urls.ffmpegURL,
		"ffprobeURL", urls.ffprobeURL,
		"isGz", urls.isGz,
	)

	// All supported platforms currently use .gz format from eugeneware/ffmpeg-static.
	if urls.isGz {
		if err := downloadAndExtractGz(urls.ffmpegURL, ffmpegPath); err != nil {
			return fmt.Errorf("failed to download ffmpeg: %w", err)
		}
		if err := downloadAndExtractGz(urls.ffprobeURL, ffprobePath); err != nil {
			// ffprobe is non-fatal — ffmpeg alone covers most functionality.
			logger.Log.Warn("ffprobe download failed, continuing without it", "error", err)
		}
	} else {
		if err := downloadAndExtractZip(urls.ffmpegURL, ffmpegPath, "ffmpeg"); err != nil {
			return err
		}
		if err := downloadAndExtractZip(urls.ffprobeURL, ffprobePath, "ffprobe"); err != nil {
			return err
		}
	}

	logger.Log.Info("FFmpeg downloaded successfully")
	return nil
}

func downloadAndExtractGz(url, destPath string) error {
	// #nosec G107 -- URL is a hardcoded GitHub release asset; not derived from user input.
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

	// Use CopyN to cap extraction size and prevent gzip bomb DoS (CWE-400).
	if _, err := io.CopyN(outFile, gzReader, maxFFmpegBinaryBytes); err != nil && err != io.EOF {
		return fmt.Errorf("failed to write %s: %w", destPath, err)
	}
	return nil
}

func downloadAndExtractZip(url, destPath, binName string) error {
	// #nosec G107 -- URL is a hardcoded GitHub release asset; not derived from user input.
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Cap download to prevent zip bomb (CWE-400).
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxFFmpegBinaryBytes))
	if err != nil {
		return fmt.Errorf("failed to read response body from %s: %w", url, err)
	}

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
		return fmt.Errorf("failed to open zip entry %s: %w", file.Name, err)
	}
	defer func() { _ = zippedFile.Close() }()

	outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", destPath, err)
	}
	defer func() { _ = outFile.Close() }()

	// Cap extraction size to prevent zip bomb DoS (CWE-400).
	if _, err := io.CopyN(outFile, zippedFile, maxFFmpegBinaryBytes); err != nil && err != io.EOF {
		return fmt.Errorf("failed to extract zip entry: %w", err)
	}
	return nil
}

// FFmpegPath returns the resolved path to the ffmpeg binary, or "" if not available.
func FFmpegPath() string {
	return ffmpegPath
}

// GetDuration returns the duration of a video in seconds.
func GetDuration(path string) (float64, error) {
	if ffprobePath == "" {
		return 0, fmt.Errorf("ffprobe not available — cannot get video duration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), durationTimeout)
	defer cancel()

	// ffprobePath is set to a fixed path under ~/.cullsnap/bin/ during Init; not user input. #nosec G204
	cmd := exec.CommandContext(ctx, ffprobePath, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
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
	// ffmpegPath is set to a fixed path under ~/.cullsnap/bin/ during Init; not user input. #nosec G204
	cmd := exec.CommandContext(ctx, ffmpegPath, "-y", "-ss", "0.5", "-i", videoPath, "-vframes", "1", "-update", "1", "-q:v", "2", "-f", "mjpeg", outPath) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Log.Warn("ffmpeg thumbnail first attempt failed, trying fallback", "error", err, "output", string(out))
		// Fallback to start of file — reuse same timeout context.
		// ffmpegPath is set to a fixed path under ~/.cullsnap/bin/ during Init; not user input. #nosec G204
		cmdFallback := exec.CommandContext(ctx, ffmpegPath, "-y", "-i", videoPath, "-vframes", "1", "-update", "1", "-q:v", "2", "-f", "mjpeg", outPath) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
		if outF, errF := cmdFallback.CombinedOutput(); errF != nil {
			logger.Log.Warn("ffmpeg thumbnail fallback also failed", "error", errF, "output", string(outF))
			return fmt.Errorf("ffmpeg thumbnail extraction failed for %s: %w", videoPath, errF)
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
	// ffmpegPath is set to a fixed path under ~/.cullsnap/bin/ during Init; not user input. #nosec G204
	cmd := exec.CommandContext(ctx, ffmpegPath, "-y", "-ss", startStr, "-i", src, "-to", endStr, "-c", "copy", dest) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
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

	// ffmpegPath is set to a fixed path under ~/.cullsnap/bin/ during Init; not user input. #nosec G204
	cmd := exec.CommandContext(ctx, ffmpegPath, "-version") // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
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
