package cloudsource

import (
	"context"
	"cullsnap/internal/logger"
	"cullsnap/internal/storage"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DownloadError records a single failed photo download during a mirror operation.
type DownloadError struct {
	Filename string `json:"filename"`
	MediaID  string `json:"mediaID"`
	Reason   string `json:"reason"` // "exported_0_files", "timeout", "osascript_failed"
}

// MirrorResult is returned by MirrorAlbum. It distinguishes between total failures
// (returned as error) and partial failures (returned as MirrorResult with Errors populated).
type MirrorResult struct {
	Dir       string          `json:"dir"`
	Evicted   []EvictedAlbum  `json:"evicted"`
	Succeeded int             `json:"succeeded"`
	Skipped   int             `json:"skipped"`
	Failed    int             `json:"failed"`
	Errors    []DownloadError `json:"errors"`
}

// MirrorManager handles downloading cloud files to local mirror directories.
type MirrorManager struct {
	baseDir string // ~/.cache/CullSnap/cloud/
	store   *storage.SQLiteStore
	workers int
	Cache   *CacheManager
}

// NewMirrorManager creates a MirrorManager.
func NewMirrorManager(baseDir string, store *storage.SQLiteStore, maxCacheMB int, workers int) *MirrorManager {
	if workers < 1 {
		workers = 4
	}
	return &MirrorManager{
		baseDir: baseDir,
		store:   store,
		workers: workers,
		Cache:   NewCacheManager(baseDir, store, maxCacheMB),
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
// Returns a MirrorResult with the mirror directory path, evicted albums, and per-file errors.
// The error return is reserved for total failures (listing crash, dir creation failure).
// Partial failures (some photos could not be exported) are reported via MirrorResult.Errors.
// Emits progress via progressFn(downloaded, total, currentFile).
// Supports cancellation via ctx.
func (m *MirrorManager) MirrorAlbum(
	ctx context.Context,
	source CloudSource,
	album Album,
	progressFn func(downloaded, total int, currentFile string),
) (MirrorResult, error) {
	providerID := source.ID()
	mirrorDir, err := m.EnsureMirrorDir(providerID, album.ID)
	if err != nil {
		return MirrorResult{}, err
	}

	// List remote media
	mediaItems, err := source.ListMediaInAlbum(ctx, album.ID)
	if err != nil {
		return MirrorResult{}, fmt.Errorf("mirror: list media failed: %w", err)
	}

	logger.Log.Debug("mirror: starting album mirror",
		"provider", providerID, "album", album.Title,
		"items", len(mediaItems), "mirrorDir", mirrorDir)

	// Evict stale albums if needed to free space
	var totalBytes int64
	for _, item := range mediaItems {
		totalBytes += item.SizeBytes
	}
	evicted, evictErr := m.Cache.EvictIfNeeded(totalBytes, providerID, album.ID)
	if evictErr != nil {
		return MirrorResult{Dir: mirrorDir, Evicted: evicted}, evictErr
	}
	if len(evicted) > 0 {
		logger.Log.Info("mirror: evicted albums to free space", "count", len(evicted))
	}

	// Filter to items needing download (not already mirrored or stale)
	var toDownload []RemoteMedia
	skipped := 0
	for _, item := range mediaItems {
		localPath := filepath.Join(mirrorDir, SanitizeID(item.ID)+filepath.Ext(item.Filename))
		// Check both: metadata exists AND local file is actually on disk
		meta, metaErr := m.store.GetCloudMediaMeta(localPath)
		if metaErr == nil && !meta.RemoteUpdatedAt.Before(item.UpdatedAt) {
			if _, statErr := os.Stat(localPath); statErr == nil {
				// Already mirrored, up to date, and file exists on disk
				skipped++
				continue
			}
			// Metadata exists but file is missing — re-download
			logger.Log.Info("mirror: file missing despite metadata, re-downloading", "file", item.Filename)
		}
		toDownload = append(toDownload, item)
	}

	logger.Log.Debug("mirror: download queue", "total", len(mediaItems), "toDownload", len(toDownload), "skipped", skipped)

	if len(toDownload) == 0 {
		// Everything already mirrored
		if progressFn != nil {
			progressFn(len(mediaItems), len(mediaItems), "")
		}
		return MirrorResult{
			Dir:       mirrorDir,
			Evicted:   evicted,
			Succeeded: skipped, // all were skipped (already present)
			Skipped:   skipped,
			Failed:    0,
			Errors:    nil,
		}, nil
	}

	// Download with worker pool — collect ALL errors (not just the first)
	var (
		wg        sync.WaitGroup
		completed int
		mu        sync.Mutex
		dlErrors  []DownloadError
		succeeded int
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
				if dlErr != nil {
					reason := classifyDownloadError(dlErr)
					logger.Log.Warn("mirror: failed download",
						"file", item.Filename, "reason", reason, "error", dlErr)
					dlErrors = append(dlErrors, DownloadError{
						Filename: item.Filename,
						MediaID:  item.ID,
						Reason:   reason,
					})
				} else {
					// Save metadata for staleness detection
					_ = m.store.SaveCloudMediaMeta(localPath, item.ID, providerID, item.UpdatedAt)
					succeeded++
				}
				completed++
				c := completed
				mu.Unlock()

				if progressFn != nil {
					progressFn(len(mediaItems)-len(toDownload)+c, len(mediaItems), item.Filename)
				}
			}
		}()
	}

	for _, item := range toDownload {
		work <- item
	}
	close(work)
	wg.Wait()

	// Save mirror record (even on partial success)
	_ = m.store.SaveCloudMirror(providerID, album.ID, album.Title, mirrorDir)

	total := len(mediaItems)
	failed := len(dlErrors)
	logger.Log.Info("mirror: completed",
		"albumID", album.ID,
		"succeeded", succeeded,
		"skipped", skipped,
		"failed", failed,
		"total", total)

	return MirrorResult{
		Dir:       mirrorDir,
		Evicted:   evicted,
		Succeeded: succeeded,
		Skipped:   skipped,
		Failed:    failed,
		Errors:    dlErrors,
	}, nil
}

// classifyDownloadError maps a download error to a short reason string for the frontend.
func classifyDownloadError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case containsAny(msg, "exported 0 files", "exported_0_files"):
		return "exported_0_files"
	case containsAny(msg, "deadline exceeded", "context deadline", "timed out", "timeout"):
		return "timeout"
	case containsAny(msg, "context canceled", "operation was cancelled"):
		return "cancelled"
	default:
		return "osascript_failed"
	}
}

// containsAny returns true if s contains any of the substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
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
