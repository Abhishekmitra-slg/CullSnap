package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"sync"
	"time"

	"cullsnap/internal/dedupe"
	"cullsnap/internal/export"
	cullImage "cullsnap/internal/image"
	"cullsnap/internal/logger"
	"cullsnap/internal/model"
	"cullsnap/internal/scanner"
	"cullsnap/internal/storage"

	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx          context.Context
	store        *storage.SQLiteStore
	dedupeMutex  sync.Mutex
	dedupeCancel context.CancelFunc
	thumbCache   *cullImage.ThumbCache
}

// NewApp creates a new App application struct
func NewApp(store *storage.SQLiteStore) *App {
	return &App{store: store}
}

// Startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	// Initialize thumbnail cache
	tc, err := cullImage.NewThumbCache()
	if err != nil {
		logger.Log.Error("Failed to initialize thumbnail cache", "error", err)
	} else {
		a.thumbCache = tc
	}
}

// SelectDirectory opens a native OS dialog to select a folder
func (a *App) SelectDirectory() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Folder to Cull",
	})
	if err != nil {
		return "", err
	}
	if dir != "" {
		a.store.AddRecent(dir)
	}
	return dir, nil
}

// SelectExportDirectory opens a native OS dialog to select an export folder
func (a *App) SelectExportDirectory() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Export Destination",
	})
	return dir, err
}

// ScanDirectory returns all photos in the directory
func (a *App) ScanDirectory(path string) ([]model.Photo, error) {
	logger.Log.Info("Scanning directory", "path", path)
	a.store.AddRecent(path)
	return scanner.ScanDirectory(path)
}

// GetSelections returns map of selections for a session
func (a *App) GetSelections(sessionID string) (map[string]bool, error) {
	return a.store.GetSelections(sessionID)
}

// ToggleSelection sets the selection status of a photo
func (a *App) ToggleSelection(path string, sessionID string, selected bool) error {
	return a.store.SaveSelection(path, sessionID, selected)
}

// GetExportedStatus returns map of exported paths under a directory
func (a *App) GetExportedStatus(dirPath string) (map[string]bool, error) {
	return a.store.GetExportedInDirectory(dirPath)
}

// PreloadThumbnails generates cached thumbnails for all images in a directory.
// Uses parallel goroutines (runtime.NumCPU()) for fast generation.
// Emits "thumb-progress" events for the UI loading animation.
// Returns photos with ThumbnailPath populated.
func (a *App) PreloadThumbnails(dirPath string) ([]model.Photo, error) {
	logger.Log.Info("PreloadThumbnails starting", "path", dirPath)
	if a.thumbCache == nil {
		// No cache available — return photos without thumbnail paths
		return scanner.ScanDirectory(dirPath)
	}

	photos, err := scanner.ScanDirectory(dirPath)
	if err != nil {
		return nil, err
	}

	// Build item list with mod times
	type thumbItem struct {
		Path    string
		ModTime time.Time
	}
	items := make([]struct {
		Path    string
		ModTime time.Time
	}, len(photos))

	for i, p := range photos {
		items[i] = struct {
			Path    string
			ModTime time.Time
		}{Path: p.Path, ModTime: p.TakenAt}
	}

	// Parallel thumbnail generation with progress
	numWorkers := stdruntime.NumCPU()
	if numWorkers < 2 {
		numWorkers = 2
	}
	if numWorkers > 8 {
		numWorkers = 8
	}

	thumbnailMap := a.thumbCache.GenerateBatch(items, numWorkers, func(completed, total int) {
		runtime.EventsEmit(a.ctx, "thumb-progress", map[string]interface{}{
			"current": completed,
			"total":   total,
		})
	})

	// Populate ThumbnailPath on photos
	for i := range photos {
		if tp, ok := thumbnailMap[photos[i].Path]; ok {
			photos[i].ThumbnailPath = tp
		}
	}

	return photos, nil
}

// ExportPhotos copies specified photos/videos to a destination directory inside a specific subfolder
func (a *App) ExportPhotos(photos []model.Photo, destDir string, folderName string) (int, error) {
	if folderName == "" {
		timestamp := time.Now().Format("20060102_150405")
		folderName = fmt.Sprintf("Session_%s", timestamp)
	}
	sessionDir := filepath.Join(destDir, folderName)

	count, err := export.ExportSelections(photos, sessionDir)
	if err == nil && count > 0 {
		// Mark exported in DB
		for _, p := range photos {
			a.store.MarkExported(p.Path)
		}
	}
	return count, err
}

// GetRecentFolders returns previously accessed folders
func (a *App) GetRecentFolders() ([]string, error) {
	return a.store.GetRecents()
}

// OpenLog opens the log file
func (a *App) OpenLog() {
	configDir, _ := os.UserConfigDir()
	logPath := filepath.Join(configDir, "CullSnap", "cullsnap.log")
	// Fallback to exec.Command as BrowserOpenURL can struggle with file:// on mac
	switch stdruntime.GOOS {
	case "darwin":
		exec.Command("open", logPath).Start()
	case "windows":
		exec.Command("cmd", "/c", "start", logPath).Start()
	default:
		exec.Command("xdg-open", logPath).Start()
	}
}

// OpenFolderInFinder opens a folder in the native file manager
func (a *App) OpenFolderInFinder(path string) {
	switch stdruntime.GOOS {
	case "darwin":
		exec.Command("open", path).Start()
	case "windows":
		exec.Command("explorer", path).Start()
	default:
		exec.Command("xdg-open", path).Start()
	}
}



// SystemResources represents the current system resource usage
type SystemResources struct {
	CPU       float64 `json:"cpu"`
	RAM       float64 `json:"ram"`
	DiskRead  float64 `json:"diskRead"`
	DiskWrite float64 `json:"diskWrite"`
	NetSent   float64 `json:"netSent"`
	NetRecv   float64 `json:"netRecv"`
}

var lastDiskRead uint64
var lastDiskWrite uint64
var lastNetSent uint64
var lastNetRecv uint64
var lastUpdate time.Time
var currentProcess *process.Process

// GetSystemResources fetches the system resources utilized by the system
func (a *App) GetSystemResources() SystemResources {
	var metrics SystemResources

	// 1. Get process CPU using gopsutil
	if currentProcess == nil {
		p, err := process.NewProcess(int32(os.Getpid()))
		if err == nil {
			currentProcess = p
		}
	}

	if currentProcess != nil {
		cpuPercent, _ := currentProcess.CPUPercent()
		metrics.CPU = cpuPercent

		// Storage/Disk IO
		ioCounters, _ := currentProcess.IOCounters()
		if ioCounters != nil {
			now := time.Now()
			if !lastUpdate.IsZero() {
				elapsed := now.Sub(lastUpdate).Seconds()
				if elapsed > 0 {
					metrics.DiskRead = float64(ioCounters.ReadBytes-lastDiskRead) / 1024 / 1024 / elapsed
					metrics.DiskWrite = float64(ioCounters.WriteBytes-lastDiskWrite) / 1024 / 1024 / elapsed
				}
			}
			lastDiskRead = ioCounters.ReadBytes
			lastDiskWrite = ioCounters.WriteBytes
			lastUpdate = now
		}
	}

	// 2. Get accurate Go Backend memory usage (in MB)
	// gopsutil RSS counts the entire MacOS WKWebview reserved memory allocations (which can be huge but lazy loaded).
	// We want to show the app's actual backend heap consumption.
	var m stdruntime.MemStats
	stdruntime.ReadMemStats(&m)
	metrics.RAM = float64(m.Alloc) / 1024 / 1024

	// For network, just get total system network and calculate diff
	// Process-specific network is hard to get cross-platform cleanly with gopsutil,
	// so we get system-wide, or we can just leave it 0 if we only want app specific.
	// But let's try network counters.
	netStats, _ := net.IOCounters(false)
	if len(netStats) > 0 {
		stat := netStats[0]
		// To keep it simple, let's just do a rough diff.
		if lastNetSent > 0 && lastNetRecv > 0 {
			metrics.NetSent = float64(stat.BytesSent-lastNetSent) / 1024 // KB
			metrics.NetRecv = float64(stat.BytesRecv-lastNetRecv) / 1024 // KB
		}
		lastNetSent = stat.BytesSent
		lastNetRecv = stat.BytesRecv
	}

	return metrics
}

// DedupeResult is the payload for the frontend
type DedupeResult struct {
	UniquePhotos    []model.Photo   `json:"uniquePhotos"`
	DuplicateGroups [][]model.Photo `json:"duplicateGroups"`
}

// CancelDeduplicate allows the frontend to abort an ongoing deduplication process.
func (a *App) CancelDeduplicate() {
	a.dedupeMutex.Lock()
	defer a.dedupeMutex.Unlock()
	if a.dedupeCancel != nil {
		a.dedupeCancel()
		a.dedupeCancel = nil
	}
}

// PhotoEXIF contains EXIF metadata for display in the viewer panel.
type PhotoEXIF struct {
	Camera    string `json:"camera"`
	Lens      string `json:"lens"`
	ISO       string `json:"iso"`
	Aperture  string `json:"aperture"`
	Shutter   string `json:"shutter"`
	DateTaken string `json:"dateTaken"`
}

// GetPhotoEXIF extracts EXIF metadata from a photo.
func (a *App) GetPhotoEXIF(path string) (*PhotoEXIF, error) {
	info, err := dedupe.ExtractFullEXIF(path)
	if err != nil {
		return nil, err
	}
	return &PhotoEXIF{
		Camera:    info.Camera,
		Lens:      info.Lens,
		ISO:       info.ISO,
		Aperture:  info.Aperture,
		Shutter:   info.Shutter,
		DateTaken: info.DateTaken,
	}, nil
}

// SetPhotoRating persists a star rating (0-5) for a photo.
func (a *App) SetPhotoRating(path string, rating int) error {
	if rating < 0 || rating > 5 {
		return fmt.Errorf("rating must be between 0 and 5")
	}
	return a.store.SaveRating(path, rating)
}

// GetRatingsForDirectory retrieves all ratings for photos in a directory.
func (a *App) GetRatingsForDirectory(dirPath string) (map[string]int, error) {
	return a.store.GetRatingsInDirectory(dirPath)
}

// DedupStatus holds information about existing dedup results for a directory.
type DedupStatus struct {
	HasDuplicates  bool            `json:"hasDuplicates"`
	DuplicateCount int             `json:"duplicateCount"`
	Duplicates     []model.Photo   `json:"duplicates"`
}

// CheckDedupStatus checks if a directory has an existing "duplicates" subfolder
// and returns its contents. This allows the frontend to auto-detect previous
// dedup results without re-running the process.
func (a *App) CheckDedupStatus(dirPath string) (*DedupStatus, error) {
	dupeDir := filepath.Join(dirPath, "duplicates")
	info, err := os.Stat(dupeDir)
	if err != nil || !info.IsDir() {
		return &DedupStatus{HasDuplicates: false}, nil
	}

	// Scan the duplicates directory for photos
	var duplicates []model.Photo
	entries, err := os.ReadDir(dupeDir)
	if err != nil {
		return &DedupStatus{HasDuplicates: false}, nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".cr2" || ext == ".cr3" || ext == ".arw" || ext == ".nef" || ext == ".dng" {
			fInfo, err := entry.Info()
			if err != nil {
				continue
			}
			p := model.Photo{
				Path:    filepath.Join(dupeDir, entry.Name()),
				Size:    fInfo.Size(),
				TakenAt: fInfo.ModTime(),
			}
			// Try to get actual date taken from EXIF
			if date, valid := dedupe.ExtractDateTaken(p.Path); valid {
				p.TakenAt = date
			}
			duplicates = append(duplicates, p)
		}
	}

	return &DedupStatus{
		HasDuplicates:  len(duplicates) > 0,
		DuplicateCount: len(duplicates),
		Duplicates:     duplicates,
	}, nil
}

// ScanAndDeduplicate runs perceptual hashing, quality scoring, sorting, and relocation.
func (a *App) ScanAndDeduplicate(path string, similarityThreshold int) (*DedupeResult, error) {
	logger.Log.Info("Scanning and deduplicating directory", "path", path)
	a.store.AddRecent(path)

	// 1. Find explicit duplicates
	// 8 is a good default threshold for dHash.
	if similarityThreshold <= 0 {
		similarityThreshold = 8
	}

	appCtx, cancel := context.WithCancel(a.ctx)
	a.dedupeMutex.Lock()
	a.dedupeCancel = cancel
	a.dedupeMutex.Unlock()

	defer func() {
		a.dedupeMutex.Lock()
		a.dedupeCancel = nil
		a.dedupeMutex.Unlock()
		cancel()
	}()

	// Shared progress emitter
	emitProgress := func(current, total int, message string) {
		runtime.EventsEmit(a.ctx, "dedupe-progress", map[string]interface{}{
			"current": current,
			"total":   total,
			"message": message,
		})
	}

	groups, err := dedupe.FindDuplicates(appCtx, path, similarityThreshold, emitProgress)
	if err != nil {
		return nil, err
	}

	// 2. Select the best quality photo in each group to represent the unique
	err = dedupe.FindBestPhotos(appCtx, groups, emitProgress)
	if err != nil {
		return nil, err
	}

	// 3. Sort groups chronologically
	emitProgress(0, len(groups), "Sorting by date...")
	err = dedupe.SortGroupsByDate(appCtx, groups)
	if err != nil {
		return nil, err
	}

	// 4. Move duplicates physically
	errs := dedupe.RelocateGroupDuplicates(appCtx, groups, emitProgress)
	if len(errs) > 0 && errs[0] == context.Canceled {
		return nil, context.Canceled
	}

	// 5. Structure data for the frontend
	res := &DedupeResult{
		UniquePhotos:    make([]model.Photo, 0, len(groups)),
		DuplicateGroups: make([][]model.Photo, 0),
	}

	for _, g := range groups {
		var duplicates []model.Photo
		for _, p := range g.Photos {
			// Get basic file info
			info, err := os.Stat(p.Path)
			if err != nil {
				continue // Skip gracefully
			}

			date, valid := dedupe.ExtractDateTaken(p.Path)
			if !valid {
				date = info.ModTime() // fallback
			}

			photoModel := model.Photo{
				Path:    p.Path,
				Size:    info.Size(),
				TakenAt: date,
			}

			if p.IsUnique {
				res.UniquePhotos = append(res.UniquePhotos, photoModel)
			} else {
				duplicates = append(duplicates, photoModel)
			}
		}
		if len(duplicates) > 0 {
			res.DuplicateGroups = append(res.DuplicateGroups, duplicates)
		}
	}

	return res, nil
}
