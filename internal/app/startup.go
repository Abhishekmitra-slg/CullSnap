package app

import (
	"cullsnap/internal/logger"
	"cullsnap/internal/storage"
	"strings"
	"time"
)

const quickRestartThreshold = 30 * time.Second

// isQuickRestart returns true if the app restarted within the threshold.
func isQuickRestart(lastStart, now time.Time) bool {
	if lastStart.IsZero() {
		return false
	}
	return now.Sub(lastStart) < quickRestartThreshold
}

// logStartupContext logs diagnostic information about the application startup.
func logStartupContext(store *storage.SQLiteStore, version string) {
	lastStartStr, _ := store.GetConfig("last_startup_time")
	now := time.Now()

	if lastStartStr != "" {
		lastStart, err := time.Parse(time.RFC3339, lastStartStr)
		if err == nil {
			elapsed := now.Sub(lastStart)
			logger.Log.Info("startup: previous run",
				"lastStart", lastStart.Format(time.RFC3339),
				"elapsed", elapsed.String())

			if isQuickRestart(lastStart, now) {
				logger.Log.Warn("startup: quick restart detected — may indicate crash loop",
					"elapsed", elapsed.String())
			}
		} else {
			logger.Log.Debug("startup: could not parse last_startup_time", "raw", lastStartStr, "error", err)
		}
	} else {
		logger.Log.Info("startup: first run (no previous startup recorded)")
	}

	// Record this startup
	if err := store.SetConfig("last_startup_time", now.Format(time.RFC3339)); err != nil {
		logger.Log.Warn("startup: failed to persist last_startup_time — crash-loop detection disabled", "error", err)
	}

	// Log whether this is a dev build
	if version == "dev" || version == "" || strings.Contains(version, "-dev") {
		logger.Log.Info("startup: development build detected (hot-reloads expected)")
	}
}
