package app

import (
	"context"
	"cullsnap/internal/cloudsource"
	"cullsnap/internal/logger"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// GetCloudSources returns available cloud providers and their auth status.
func (a *App) GetCloudSources() []cloudsource.CloudSourceStatus {
	logger.Log.Debug("cloud: listing cloud sources")
	return a.cloudRegistry.All()
}

// AuthenticateCloudSource starts the OAuth flow for a provider.
func (a *App) AuthenticateCloudSource(providerID string) error {
	logger.Log.Info("cloud: authenticating provider", "providerID", providerID)
	source, ok := a.cloudRegistry.Get(providerID)
	if !ok {
		return fmt.Errorf("unknown cloud provider: %s", providerID)
	}
	err := source.Authenticate(a.ctx)
	if err != nil {
		logger.Log.Error("cloud: authentication failed", "providerID", providerID, "error", err)
		runtime.EventsEmit(a.ctx, "cloud-auth-error", map[string]string{
			"provider": providerID,
			"error":    err.Error(),
		})
		return err
	}
	logger.Log.Info("cloud: authentication complete", "providerID", providerID)
	runtime.EventsEmit(a.ctx, "cloud-auth-complete", map[string]string{
		"provider": providerID,
	})
	return nil
}

// DisconnectCloudSource removes auth for a provider and clears its mirrors.
func (a *App) DisconnectCloudSource(providerID string) error {
	logger.Log.Info("cloud: disconnecting provider", "providerID", providerID)
	source, ok := a.cloudRegistry.Get(providerID)
	if !ok {
		return fmt.Errorf("unknown cloud provider: %s", providerID)
	}
	if err := source.Disconnect(); err != nil {
		logger.Log.Error("cloud: disconnect failed", "providerID", providerID, "error", err)
		return err
	}
	logger.Log.Info("cloud: provider disconnected", "providerID", providerID)
	return nil
}

// ListCloudAlbums returns albums/folders from an authenticated provider.
func (a *App) ListCloudAlbums(providerID string) ([]cloudsource.Album, error) {
	logger.Log.Debug("cloud: listing albums", "providerID", providerID)
	source, ok := a.cloudRegistry.Get(providerID)
	if !ok {
		return nil, fmt.Errorf("unknown cloud provider: %s", providerID)
	}
	if !source.IsAuthenticated() {
		return nil, fmt.Errorf("provider %s is not authenticated", providerID)
	}
	albums, err := source.ListAlbums(a.ctx)
	if err != nil {
		logger.Log.Error("cloud: list albums failed", "providerID", providerID, "error", err)
		return nil, err
	}
	logger.Log.Debug("cloud: albums listed", "providerID", providerID, "count", len(albums))
	return albums, nil
}

// MirrorCloudAlbum downloads an album to a local mirror directory.
// Returns the mirror directory path. Emits progress and result events.
// Partial success (some photos failed) returns the mirror path without error;
// the frontend receives a cloud-download-result event with failure details.
// Total failure (listing crash, dir creation failure) returns an error.
func (a *App) MirrorCloudAlbum(providerID, albumID, albumTitle string) (string, error) {
	logger.Log.Info("cloud: starting album mirror", "providerID", providerID, "albumID", albumID, "title", albumTitle)
	source, ok := a.cloudRegistry.Get(providerID)
	if !ok {
		return "", fmt.Errorf("unknown cloud provider: %s", providerID)
	}

	emitStatus := func(phase string) {
		runtime.EventsEmit(a.ctx, "cloud-download-progress", map[string]interface{}{
			"provider": providerID,
			"albumID":  albumID,
			"phase":    phase,
		})
	}

	// Build album struct from parameters — avoids redundant ListAlbums call
	// which is expensive for AppleScript-based providers like iCloud (~30-40s)
	album := cloudsource.Album{
		ID:    albumID,
		Title: albumTitle,
	}

	emitStatus("Reading album contents...")

	// Create cancellable context
	ctx, cancel := context.WithCancel(a.ctx)
	key := providerID + ":" + albumID
	a.mirrorMu.Lock()
	a.mirrorCancels[key] = cancel
	a.mirrorMu.Unlock()

	defer func() {
		a.mirrorMu.Lock()
		delete(a.mirrorCancels, key)
		a.mirrorMu.Unlock()
		cancel()
	}()

	// Start mirror with progress events
	result, err := a.mirrorManager.MirrorAlbum(ctx, source, album, func(downloaded, total int, currentFile string) {
		logger.Log.Debug("cloud: mirror progress", "providerID", providerID, "albumID", albumID,
			"downloaded", downloaded, "total", total, "file", currentFile)
		runtime.EventsEmit(a.ctx, "cloud-download-progress", map[string]interface{}{
			"provider":    providerID,
			"albumID":     albumID,
			"downloaded":  downloaded,
			"total":       total,
			"currentFile": currentFile,
		})
	})

	// Notify frontend of evictions regardless of error
	if len(result.Evicted) > 0 {
		runtime.EventsEmit(a.ctx, "cloud-cache-evicted", result.Evicted)
	}

	if err != nil {
		// Total failure — listing crash, Photos.app not running, dir creation failure
		logger.Log.Error("cloud: mirror failed (total)", "providerID", providerID, "albumID", albumID, "error", err)
		runtime.EventsEmit(a.ctx, "cloud-download-error", map[string]string{
			"provider": providerID,
			"albumID":  albumID,
			"error":    err.Error(),
		})
		// Still register mirrorDir — partial content may be usable
		if result.Dir != "" {
			a.OnAllowDir(result.Dir)
		}
		return result.Dir, err
	}

	// Register mirror dir with media server
	a.OnAllowDir(result.Dir)

	// Build error list for frontend
	type downloadErrorJSON struct {
		Filename string `json:"filename"`
		MediaID  string `json:"mediaID"`
		Reason   string `json:"reason"`
	}
	errorList := make([]downloadErrorJSON, 0, len(result.Errors))
	for _, e := range result.Errors {
		errorList = append(errorList, downloadErrorJSON{
			Filename: e.Filename,
			MediaID:  e.MediaID,
			Reason:   e.Reason,
		})
	}

	// Emit structured result event for 3-state frontend UX
	runtime.EventsEmit(a.ctx, "cloud-download-result", map[string]interface{}{
		"provider":   providerID,
		"albumID":    albumID,
		"albumTitle": albumTitle,
		"path":       result.Dir,
		"succeeded":  result.Succeeded,
		"skipped":    result.Skipped,
		"failed":     result.Failed,
		"total":      result.Succeeded + result.Skipped + result.Failed,
		"errors":     errorList,
	})

	if result.Failed == 0 {
		logger.Log.Info("cloud: album mirrored successfully",
			"providerID", providerID, "albumID", albumID,
			"succeeded", result.Succeeded, "skipped", result.Skipped)
	} else {
		logger.Log.Warn("cloud: album mirrored with partial failures",
			"providerID", providerID, "albumID", albumID,
			"succeeded", result.Succeeded, "skipped", result.Skipped, "failed", result.Failed)
	}

	// Also emit legacy cloud-download-complete with result counts for backward compat
	runtime.EventsEmit(a.ctx, "cloud-download-complete", map[string]interface{}{
		"provider":  providerID,
		"albumID":   albumID,
		"path":      result.Dir,
		"succeeded": result.Succeeded,
		"skipped":   result.Skipped,
		"failed":    result.Failed,
	})

	return result.Dir, nil
}

// CancelMirror cancels an in-progress mirror download.
func (a *App) CancelMirror(providerID, albumID string) error {
	key := providerID + ":" + albumID
	logger.Log.Info("cloud: cancelling mirror", "key", key)
	a.mirrorMu.Lock()
	cancel, ok := a.mirrorCancels[key]
	a.mirrorMu.Unlock()
	if !ok {
		return fmt.Errorf("no active mirror for %s", key)
	}
	cancel()
	return nil
}

// MirrorStats holds disk usage info for cloud mirrors.
type MirrorStats struct {
	TotalMB int64 `json:"totalMB"`
}

// GetMirrorStats returns disk usage of all cloud mirrors.
func (a *App) GetMirrorStats() (MirrorStats, error) {
	stats, err := a.mirrorManager.Cache.GetCacheStats()
	if err != nil {
		logger.Log.Error("cloud: failed to get mirror stats", "error", err)
		return MirrorStats{}, err
	}
	return MirrorStats{TotalMB: stats.TotalBytes / (1024 * 1024)}, nil
}

// ClearCloudMirror removes the local mirror for a specific album.
// If both providerID and albumID are empty, clears all cached albums.
func (a *App) ClearCloudMirror(providerID, albumID string) error {
	if providerID == "" && albumID == "" {
		return a.ClearAllCache()
	}
	return a.DeleteCachedAlbum(providerID, albumID)
}

// GetCacheStats returns aggregate cache usage information.
func (a *App) GetCacheStats() (cloudsource.CacheStats, error) {
	stats, err := a.mirrorManager.Cache.GetCacheStats()
	if err != nil {
		logger.Log.Error("cloud: failed to get cache stats", "error", err)
		return cloudsource.CacheStats{}, err
	}
	logger.Log.Debug("cloud: cache stats", "totalBytes", stats.TotalBytes,
		"albums", stats.AlbumCount, "limitBytes", stats.LimitBytes)
	return stats, nil
}

// ListCachedAlbums returns per-album cache details for the settings UI.
func (a *App) ListCachedAlbums() ([]cloudsource.CachedAlbum, error) {
	albums, err := a.mirrorManager.Cache.ListCachedAlbums()
	if err != nil {
		logger.Log.Error("cloud: failed to list cached albums", "error", err)
		return nil, err
	}
	logger.Log.Debug("cloud: listed cached albums", "count", len(albums))
	return albums, nil
}

// DeleteCachedAlbum removes a single album's mirror cache.
func (a *App) DeleteCachedAlbum(providerID, albumID string) error {
	logger.Log.Info("cloud: deleting cached album", "providerID", providerID, "albumID", albumID)
	if err := a.mirrorManager.Cache.DeleteAlbum(providerID, albumID); err != nil {
		logger.Log.Error("cloud: failed to delete cached album", "error", err)
		return err
	}
	return nil
}

// ClearAllCache removes all cached cloud albums.
func (a *App) ClearAllCache() error {
	logger.Log.Info("cloud: clearing all cache")
	if err := a.mirrorManager.Cache.ClearAll(); err != nil {
		logger.Log.Error("cloud: failed to clear all cache", "error", err)
		return err
	}
	return nil
}
