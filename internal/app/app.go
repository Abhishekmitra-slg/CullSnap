package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"sync"
	"time"

	"cullsnap/internal/dedupe"
	"cullsnap/internal/export"
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
}

// NewApp creates a new App application struct
func NewApp(store *storage.SQLiteStore) *App {
	return &App{store: store}
}

// Startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
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

// ExportPhotos copies specified photos to a destination directory inside a timestamped subfolder
func (a *App) ExportPhotos(photos []model.Photo, destDir string) (int, error) {
	timestamp := time.Now().Format("20060102_150405")
	sessionDir := filepath.Join(destDir, fmt.Sprintf("Session_%s", timestamp))

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

	groups, err := dedupe.FindDuplicates(appCtx, path, similarityThreshold, func(current, total int, message string) {
		runtime.EventsEmit(a.ctx, "dedupe-progress", map[string]interface{}{
			"current": current,
			"total":   total,
			"message": message,
		})
	})
	if err != nil {
		return nil, err
	}

	// 2. Select the best quality photo in each group to represent the unique
	err = dedupe.FindBestPhotos(appCtx, groups)
	if err != nil {
		return nil, err
	}

	// 3. Sort groups chronologically
	err = dedupe.SortGroupsByDate(appCtx, groups)
	if err != nil {
		return nil, err
	}

	// 4. Move duplicates physically
	errs := dedupe.RelocateGroupDuplicates(appCtx, groups)
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
