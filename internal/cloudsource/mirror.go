package cloudsource

import (
	"context"
	"cullsnap/internal/logger"
	"cullsnap/internal/storage"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// MirrorManager handles downloading cloud files to local mirror directories.
type MirrorManager struct {
	baseDir    string // ~/.cache/CullSnap/cloud/
	store      *storage.SQLiteStore
	maxCacheMB int
	workers    int
}

// NewMirrorManager creates a MirrorManager.
func NewMirrorManager(baseDir string, store *storage.SQLiteStore, maxCacheMB int, workers int) *MirrorManager {
	if workers < 1 {
		workers = 4
	}
	return &MirrorManager{
		baseDir:    baseDir,
		store:      store,
		maxCacheMB: maxCacheMB,
		workers:    workers,
	}
}

// effectiveWorkers returns the worker count for a source. Sources that
// implement SequentialDownloader with IsSequentialDownload() == true
// are limited to 1 worker to avoid concurrent access issues.
func (m *MirrorManager) effectiveWorkers(source CloudSource) int {
	if seq, ok := source.(SequentialDownloader); ok && seq.IsSequentialDownload() {
		return 1
	}
	return m.workers
}

// MirrorPath returns the local directory for a provider/album.
func (m *MirrorManager) MirrorPath(providerID, albumID string) string {
	return filepath.Join(m.baseDir, SanitizeID(providerID), SanitizeID(albumID))
}

// EnsureMirrorDir creates the mirror directory with proper permissions.
func (m *MirrorManager) EnsureMirrorDir(providerID, albumID string) (string, error) {
	dir := m.MirrorPath(providerID, albumID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mirror: failed to create dir: %w", err)
	}
	return dir, nil
}

// MirrorAlbum downloads missing/stale files from a cloud album to the local mirror.
// Returns the mirror directory path. Emits progress via progressFn(downloaded, total).
// Supports cancellation via ctx.
func (m *MirrorManager) MirrorAlbum(
	ctx context.Context,
	source CloudSource,
	album Album,
	progressFn func(downloaded, total int),
) (string, error) {
	providerID := source.ID()
	mirrorDir, err := m.EnsureMirrorDir(providerID, album.ID)
	if err != nil {
		return "", err
	}

	// List remote media
	mediaItems, err := source.ListMediaInAlbum(ctx, album.ID)
	if err != nil {
		return "", fmt.Errorf("mirror: list media failed: %w", err)
	}

	logger.Log.Debug("mirror: starting album mirror",
		"provider", providerID, "album", album.Title,
		"items", len(mediaItems), "mirrorDir", mirrorDir)

	// Check disk space
	var totalBytes int64
	for _, item := range mediaItems {
		totalBytes += item.SizeBytes
	}
	if err := m.CheckDiskSpace(totalBytes); err != nil {
		return mirrorDir, err // return mirrorDir so partial content is accessible
	}

	// Filter to items needing download (not already mirrored or stale)
	var toDownload []RemoteMedia
	for _, item := range mediaItems {
		localPath := filepath.Join(mirrorDir, SanitizeID(item.ID)+filepath.Ext(item.Filename))
		meta, metaErr := m.store.GetCloudMediaMeta(localPath)
		if metaErr == nil && !meta.RemoteUpdatedAt.Before(item.UpdatedAt) {
			// Already mirrored and up to date
			continue
		}
		toDownload = append(toDownload, item)
	}

	logger.Log.Debug("mirror: download queue", "total", len(mediaItems), "toDownload", len(toDownload))

	if len(toDownload) == 0 {
		// Everything already mirrored
		if progressFn != nil {
			progressFn(len(mediaItems), len(mediaItems))
		}
		return mirrorDir, nil
	}

	// Download with worker pool
	var (
		wg        sync.WaitGroup
		completed int
		mu        sync.Mutex
		firstErr  error
	)

	work := make(chan RemoteMedia, len(toDownload))
	workers := m.effectiveWorkers(source)
	logger.Log.Debug("mirror: using worker count", "workers", workers, "provider", providerID)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range work {
				select {
				case <-ctx.Done():
					return
				default:
				}

				localPath := filepath.Join(mirrorDir, SanitizeID(item.ID)+filepath.Ext(item.Filename))
				dlErr := source.Download(ctx, item, localPath, nil)

				mu.Lock()
				if dlErr != nil && firstErr == nil {
					firstErr = dlErr
					logger.Log.Error("mirror: download failed", "file", item.Filename, "error", dlErr)
				}
				if dlErr == nil {
					// Save metadata for staleness detection
					_ = m.store.SaveCloudMediaMeta(localPath, item.ID, providerID, item.UpdatedAt)
				}
				completed++
				c := completed
				mu.Unlock()

				if progressFn != nil {
					progressFn(len(mediaItems)-len(toDownload)+c, len(mediaItems))
				}
			}
		}()
	}

	for _, item := range toDownload {
		work <- item
	}
	close(work)
	wg.Wait()

	// Save mirror record
	_ = m.store.SaveCloudMirror(providerID, album.ID, album.Title, mirrorDir)

	if firstErr != nil {
		return mirrorDir, fmt.Errorf("mirror: some downloads failed: %w", firstErr)
	}

	logger.Log.Info("mirror: album mirrored", "provider", providerID, "album", album.Title, "files", len(mediaItems))
	return mirrorDir, nil
}

// CheckDiskSpace verifies sufficient space for the estimated download.
func (m *MirrorManager) CheckDiskSpace(estimatedBytes int64) error {
	// Check against configured max cache size
	currentUsage, err := m.DiskUsage()
	if err != nil {
		logger.Log.Warn("mirror: could not determine disk usage", "error", err)
		return nil // proceed optimistically
	}

	maxBytes := int64(m.maxCacheMB) * 1024 * 1024
	if currentUsage+estimatedBytes > maxBytes {
		return fmt.Errorf("mirror: download (%d MB) would exceed cache limit (%d MB, %d MB used)",
			estimatedBytes/(1024*1024), m.maxCacheMB, currentUsage/(1024*1024))
	}
	return nil
}

// DiskUsage calculates total bytes used by the mirror directory.
func (m *MirrorManager) DiskUsage() (int64, error) {
	var total int64
	err := filepath.Walk(m.baseDir, func(_ string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // skip unreadable files
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// ClearMirror removes the mirror directory for a specific album.
func (m *MirrorManager) ClearMirror(providerID, albumID string) error {
	dir := m.MirrorPath(providerID, albumID)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("mirror: failed to clear: %w", err)
	}
	return m.store.DeleteCloudMirror(providerID, albumID)
}
