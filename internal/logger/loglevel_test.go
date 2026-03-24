package logger

import (
	"log/slog"
	"os"
	"testing"
)

func TestLogLevel_Default(t *testing.T) {
	os.Unsetenv("CULLSNAP_LOG_LEVEL")
	if got := logLevel(); got != slog.LevelInfo {
		t.Errorf("logLevel() = %v, want %v", got, slog.LevelInfo)
	}
}

func TestLogLevel_Debug(t *testing.T) {
	t.Setenv("CULLSNAP_LOG_LEVEL", "debug")
	if got := logLevel(); got != slog.LevelDebug {
		t.Errorf("logLevel() = %v, want %v", got, slog.LevelDebug)
	}
}

func TestLogLevel_Warn(t *testing.T) {
	t.Setenv("CULLSNAP_LOG_LEVEL", "warn")
	if got := logLevel(); got != slog.LevelWarn {
		t.Errorf("logLevel() = %v, want %v", got, slog.LevelWarn)
	}
}

func TestLogLevel_Error(t *testing.T) {
	t.Setenv("CULLSNAP_LOG_LEVEL", "error")
	if got := logLevel(); got != slog.LevelError {
		t.Errorf("logLevel() = %v, want %v", got, slog.LevelError)
	}
}

func TestLogLevel_CaseInsensitive(t *testing.T) {
	t.Setenv("CULLSNAP_LOG_LEVEL", "DEBUG")
	if got := logLevel(); got != slog.LevelDebug {
		t.Errorf("logLevel('DEBUG') = %v, want %v", got, slog.LevelDebug)
	}
}

func TestLogLevel_Unknown(t *testing.T) {
	t.Setenv("CULLSNAP_LOG_LEVEL", "verbose")
	if got := logLevel(); got != slog.LevelInfo {
		t.Errorf("logLevel('verbose') = %v, want %v (default)", got, slog.LevelInfo)
	}
}
