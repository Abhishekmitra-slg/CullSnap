package app

import (
	"cullsnap/internal/device"
	"cullsnap/internal/logger"
	"fmt"
	"os"
	"path/filepath"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// GetConnectedDevices returns the list of currently connected Apple mobile devices.
func (a *App) GetConnectedDevices() []device.Device {
	if a.deviceDetector == nil {
		logger.Log.Debug("device: GetConnectedDevices called but detector is nil")
		return nil
	}
	devices := a.deviceDetector.ConnectedDevices()
	logger.Log.Debug("device: GetConnectedDevices", "count", len(devices))
	return devices
}

// ImportFromDevice imports photos from a connected device via Image Capture.
// Returns the import directory path. On partial failure the directory may
// still contain some imported files.
func (a *App) ImportFromDevice(serial string) (string, error) {
	baseDir := filepath.Join(a.cfg.CacheDir, "imports")
	logger.Log.Info("device: starting import", "serial", serial, "baseDir", baseDir)

	importDir, count, err := device.ImportFromDevice(a.ctx, serial, baseDir)
	if err != nil {
		logger.Log.Error("device: import failed", "serial", serial, "error", err, "partialCount", count)
		wailsRuntime.EventsEmit(a.ctx, "device-import-error", map[string]interface{}{
			"serial": serial,
			"error":  err.Error(),
			"count":  count,
			"path":   importDir,
		})
		// Still allow access to partial content
		if importDir != "" && count > 0 && a.OnAllowDir != nil {
			a.OnAllowDir(importDir)
		}
		return importDir, err
	}

	if a.OnAllowDir != nil {
		a.OnAllowDir(importDir)
	}
	wailsRuntime.EventsEmit(a.ctx, "device-import-complete", map[string]interface{}{
		"serial": serial,
		"count":  count,
		"path":   importDir,
	})
	logger.Log.Info("device: import complete", "serial", serial, "files", count, "path", importDir)
	return importDir, nil
}

// CancelImport cancels an in-progress device import.
func (a *App) CancelImport(serial string) error {
	logger.Log.Info("device: cancel import requested", "serial", serial)
	return fmt.Errorf("import cancellation not yet implemented")
}

// ImportStats holds disk usage information for the device import cache.
type ImportStats struct {
	TotalBytes  int64            `json:"totalBytes"`
	DeviceStats map[string]int64 `json:"deviceStats"` // serial -> bytes
}

// GetImportStats returns disk usage statistics for the device import cache.
func (a *App) GetImportStats() (ImportStats, error) {
	importDir := filepath.Join(a.cfg.CacheDir, "imports")
	stats := ImportStats{
		DeviceStats: make(map[string]int64),
	}

	entries, err := os.ReadDir(importDir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Log.Debug("device: import dir does not exist, returning empty stats")
			return stats, nil
		}
		logger.Log.Error("device: failed to read import dir", "error", err)
		return stats, fmt.Errorf("failed to read import directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		deviceDir := filepath.Join(importDir, entry.Name())
		var deviceTotal int64
		walkErr := filepath.Walk(deviceDir, func(_ string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !info.IsDir() {
				deviceTotal += info.Size()
			}
			return nil
		})
		if walkErr != nil {
			logger.Log.Error("device: failed to walk device import dir", "dir", deviceDir, "error", walkErr)
			continue
		}
		stats.DeviceStats[entry.Name()] = deviceTotal
		stats.TotalBytes += deviceTotal
	}

	logger.Log.Debug("device: import stats", "totalBytes", stats.TotalBytes, "devices", len(stats.DeviceStats))
	return stats, nil
}

// ClearImportCache removes the cached import directory for a specific device.
func (a *App) ClearImportCache(serial string) error {
	importDir := filepath.Join(a.cfg.CacheDir, "imports", device.SanitizeSerial(serial))
	logger.Log.Info("device: clearing import cache", "serial", serial, "dir", importDir)
	if err := os.RemoveAll(importDir); err != nil {
		logger.Log.Error("device: failed to clear import cache", "serial", serial, "error", err)
		return fmt.Errorf("failed to clear import cache: %w", err)
	}
	logger.Log.Info("device: import cache cleared", "serial", serial)
	return nil
}
