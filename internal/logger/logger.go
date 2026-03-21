package logger

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	Log     *slog.Logger
	LogPath string
)

// Init initializes the global logger to write to the specified file.
func Init(filename string) error {
	if filepath.IsAbs(filename) {
		LogPath = filename
	} else {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		LogPath = filepath.Join(wd, filename)
	}

	file, err := os.OpenFile(LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		return err
	}

	handler := slog.NewJSONHandler(file, &slog.HandlerOptions{Level: logLevel()})
	Log = slog.New(handler)

	Log.Info("Logger initialized", "path", LogPath)
	return nil
}

func logLevel() slog.Level {
	switch strings.ToLower(os.Getenv("CULLSNAP_LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// OpenLogFile opens the log file in the default OS application.
func OpenLogFile() error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", LogPath)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", LogPath)
	default: // linux
		cmd = exec.Command("xdg-open", LogPath)
	}
	return cmd.Start()
}
