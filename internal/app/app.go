package app

import (
	"context"
	"cullsnap/internal/dedupe"
	"cullsnap/internal/export"
	"cullsnap/internal/logger"
	"cullsnap/internal/model"
	"cullsnap/internal/raw"
	"cullsnap/internal/scanner"
	"cullsnap/internal/storage"
	"cullsnap/internal/updater"
	"cullsnap/internal/video"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cullImage "cullsnap/internal/image"

	"golang.org/x/sync/errgroup"

	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Contributor represents a project contributor parsed from CONTRIBUTORS.yml.
type Contributor struct {
	Name   string `json:"name"`
	GitHub string `json:"github"`
	Role   string `json:"role"`
	Bio    string `json:"bio"`
	Avatar string `json:"avatar"`
}

// AboutInfo contains app metadata returned by GetAboutInfo.
type AboutInfo struct {
	Version       string        `json:"version"`
	GoVersion     string        `json:"goVersion"`
	WailsVersion  string        `json:"wailsVersion"`
	SQLiteVersion string        `json:"sqliteVersion"`
	FFmpegVersion string        `json:"ffmpegVersion"`
	License       string        `json:"license"`
	Repo          string        `json:"repo"`
	Contributors  []Contributor `json:"contributors"`
}

// App struct
type App struct {
	ctx             context.Context
	store           *storage.SQLiteStore
	dedupeMutex     sync.Mutex
	dedupeCancel    context.CancelFunc
	thumbCache      *cullImage.ThumbCache
	cfg             *AppConfig
	enrichMu        sync.Mutex
	enrichCancel    context.CancelFunc
	OnAllowDir      func(dir string) // called to register a directory with the media server allowlist
	Version         string           // set from main.version (build-time ldflags)
	ContributorsRaw string           // raw CONTRIBUTORS.yml content embedded at build time
	UpdatePublicKey []byte           // ECDSA public key for update signature verification
	updater         *updater.Updater // manages self-update checks
}

// NewApp creates a new App application struct
func NewApp(store *storage.SQLiteStore) *App {
	return &App{store: store}
}

// Startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	home, _ := os.UserHomeDir()
	ffmpegPath := filepath.Join(home, ".cullsnap", "bin", "ffmpeg")
	if stdruntime.GOOS == "windows" {
		ffmpegPath += ".exe"
	}
	a.cfg = a.loadOrInitConfig(ffmpegPath)

	tc, err := cullImage.NewThumbCache(a.cfg.CacheDir, true)
	if err != nil {
		logger.Log.Error("Failed to initialize thumbnail cache", "error", err)
	} else {
		a.thumbCache = tc
	}

	// Start background goroutine to push system metrics every second
	go a.emitSystemMetrics()

	// Start auto-update checker
	a.updater = updater.NewUpdater(ctx, a.Version, a.UpdatePublicKey, a.cfg.AutoUpdate)
	a.updater.Start()

	// Initialize RAW module (dcraw provisioning)
	if err := raw.Init(); err != nil {
		logger.Log.Error("Failed to initialize RAW module", "error", err)
	}
	logger.Log.Info("app: RAW module initialized")
}

func (a *App) loadOrInitConfig(ffmpegPath string) *AppConfig {
	maxConn, _ := a.store.GetConfig("maxConnections")
	if maxConn == "" {
		probe := RunSystemProbe(ffmpegPath)
		cfg := DeriveDefaults(probe)
		a.persistConfig(&cfg)
		return &cfg
	}

	cfg := &AppConfig{}
	cfg.MaxConnections, _ = strconv.Atoi(maxConn)
	val, _ := a.store.GetConfig("thumbnailWorkers")
	cfg.ThumbnailWorkers, _ = strconv.Atoi(val)
	val, _ = a.store.GetConfig("scannerWorkers")
	cfg.ScannerWorkers, _ = strconv.Atoi(val)
	val, _ = a.store.GetConfig("serverIdleTimeoutSec")
	cfg.ServerIdleTimeoutSec, _ = strconv.Atoi(val)
	cfg.CacheDir, _ = a.store.GetConfig("cacheDir")
	cfg.AutoUpdate, _ = a.store.GetConfig("autoUpdate")
	if cfg.AutoUpdate == "" {
		cfg.AutoUpdate = "notify"
	}

	// Always re-run the probe on startup so hardware info stays current.
	cfg.Probe = RunSystemProbe(ffmpegPath)

	if cfg.MaxConnections < 10 {
		cfg.MaxConnections = 10
	}
	if cfg.ThumbnailWorkers < 2 {
		cfg.ThumbnailWorkers = 2
	}
	if cfg.ScannerWorkers < 1 {
		cfg.ScannerWorkers = 1
	}
	if cfg.ServerIdleTimeoutSec < 1 {
		cfg.ServerIdleTimeoutSec = 30
	}
	if cfg.CacheDir == "" {
		cacheBase, _ := os.UserCacheDir()
		cfg.CacheDir = filepath.Join(cacheBase, "CullSnap", "thumbs")
	}
	return cfg
}

func (a *App) persistConfig(cfg *AppConfig) {
	if err := a.store.SetConfig("maxConnections", strconv.Itoa(cfg.MaxConnections)); err != nil {
		runtime.LogWarningf(a.ctx, "persistConfig: failed to save maxConnections: %v", err)
	}
	if err := a.store.SetConfig("thumbnailWorkers", strconv.Itoa(cfg.ThumbnailWorkers)); err != nil {
		runtime.LogWarningf(a.ctx, "persistConfig: failed to save thumbnailWorkers: %v", err)
	}
	if err := a.store.SetConfig("scannerWorkers", strconv.Itoa(cfg.ScannerWorkers)); err != nil {
		runtime.LogWarningf(a.ctx, "persistConfig: failed to save scannerWorkers: %v", err)
	}
	if err := a.store.SetConfig("serverIdleTimeoutSec", strconv.Itoa(cfg.ServerIdleTimeoutSec)); err != nil {
		runtime.LogWarningf(a.ctx, "persistConfig: failed to save serverIdleTimeoutSec: %v", err)
	}
	if err := a.store.SetConfig("cacheDir", cfg.CacheDir); err != nil {
		runtime.LogWarningf(a.ctx, "persistConfig: failed to save cacheDir: %v", err)
	}
	if err := a.store.SetConfig("autoUpdate", cfg.AutoUpdate); err != nil {
		runtime.LogWarningf(a.ctx, "persistConfig: failed to save autoUpdate: %v", err)
	}
	if probeJSON, err := json.Marshal(cfg.Probe); err == nil {
		if err := a.store.SetConfig("probe", string(probeJSON)); err != nil {
			runtime.LogWarningf(a.ctx, "persistConfig: failed to save probe: %v", err)
		}
	}
}

// GetAppConfig returns the current configuration including last system probe data.
func (a *App) GetAppConfig() (*AppConfig, error) {
	if a.cfg == nil {
		return nil, fmt.Errorf("config not initialised")
	}
	return a.cfg, nil
}

// SaveAppConfig persists user-overridden values.
func (a *App) SaveAppConfig(cfg AppConfig) error {
	cfg.Probe = a.cfg.Probe
	a.cfg = &cfg
	a.persistConfig(&cfg)
	return nil
}

// GetAboutInfo returns app metadata, tech stack versions, and contributors.
func (a *App) GetAboutInfo() *AboutInfo {
	info := &AboutInfo{
		Version:       a.Version,
		GoVersion:     stdruntime.Version(),
		WailsVersion:  "v2.11.0",
		SQLiteVersion: a.getSQLiteVersion(),
		FFmpegVersion: video.GetFFmpegVersion(),
		License:       "AGPL-3.0",
		Repo:          "https://github.com/Abhishekmitra-slg/CullSnap",
		Contributors:  parseContributors(a.ContributorsRaw),
	}
	return info
}

func (a *App) getSQLiteVersion() string {
	ver, err := a.store.GetSQLiteVersion()
	if err != nil {
		return "unknown"
	}
	return ver
}

// parseContributors parses the simple YAML list format from CONTRIBUTORS.yml.
func parseContributors(raw string) []Contributor {
	var contributors []Contributor
	var current *Contributor

	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "---" {
			continue
		}
		if strings.HasPrefix(trimmed, "- name:") {
			if current != nil {
				current.Avatar = fmt.Sprintf("https://github.com/%s.png", current.GitHub)
				contributors = append(contributors, *current)
			}
			current = &Contributor{Name: strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:"))}
		} else if current != nil {
			switch {
			case strings.HasPrefix(trimmed, "github:"):
				current.GitHub = strings.TrimSpace(strings.TrimPrefix(trimmed, "github:"))
			case strings.HasPrefix(trimmed, "role:"):
				current.Role = strings.TrimSpace(strings.TrimPrefix(trimmed, "role:"))
			case strings.HasPrefix(trimmed, "bio:"):
				current.Bio = strings.TrimSpace(strings.TrimPrefix(trimmed, "bio:"))
			}
		}
	}
	if current != nil {
		current.Avatar = fmt.Sprintf("https://github.com/%s.png", current.GitHub)
		contributors = append(contributors, *current)
	}
	return contributors
}

// ResetAppConfig re-runs the system probe and resets config to derived defaults.
func (a *App) ResetAppConfig() (*AppConfig, error) {
	home, _ := os.UserHomeDir()
	ffmpegPath := filepath.Join(home, ".cullsnap", "bin", "ffmpeg")
	if stdruntime.GOOS == "windows" {
		ffmpegPath += ".exe"
	}
	probe := RunSystemProbe(ffmpegPath)
	cfg := DeriveDefaults(probe)
	a.cfg = &cfg
	if err := a.store.DeleteAllConfig(); err != nil {
		return nil, err
	}
	a.persistConfig(&cfg)
	return &cfg, nil
}

// CheckForUpdate triggers an immediate update check.
func (a *App) CheckForUpdate() error {
	if a.updater == nil {
		return fmt.Errorf("updater not initialized")
	}
	return a.updater.CheckNow()
}

// DownloadUpdate downloads the pending update.
func (a *App) DownloadUpdate() error {
	if a.updater == nil {
		return fmt.Errorf("updater not initialized")
	}
	return a.updater.DownloadUpdate()
}

// RestartForUpdate applies the downloaded update and restarts the app.
func (a *App) RestartForUpdate() error {
	if a.updater == nil {
		return fmt.Errorf("updater not initialized")
	}
	return a.updater.RestartForUpdate()
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
		_ = a.store.AddRecent(dir)
		if a.OnAllowDir != nil {
			a.OnAllowDir(dir)
		}
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

// startEnrichment cancels any running enrichment, then starts a new one.
// Safe to call multiple times (e.g. when user switches folders).
func (a *App) startEnrichment(videoPaths []string) {
	a.enrichMu.Lock()
	if a.enrichCancel != nil {
		a.enrichCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.enrichCancel = cancel
	a.enrichMu.Unlock()

	go a.enrichVideoDurations(ctx, videoPaths)
}

// enrichVideoDurations runs a worker pool of cfg.ScannerWorkers goroutines.
// Each worker calls ffprobe on one video at a time.
// Results are pushed to the frontend via "video-duration-ready" events.
// Emits "video-duration-complete" when all workers finish.
func (a *App) enrichVideoDurations(ctx context.Context, videoPaths []string) {
	if len(videoPaths) == 0 {
		runtime.EventsEmit(a.ctx, "video-duration-complete")
		return
	}

	workers := 2
	if a.cfg != nil {
		workers = a.cfg.ScannerWorkers
	}

	work := make(chan string, len(videoPaths))
	for _, p := range videoPaths {
		work <- p
	}
	close(work)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range work {
				select {
				case <-ctx.Done():
					return
				default:
				}
				dur, err := video.GetDuration(path)
				if err != nil {
					logger.Log.Warn("Duration enrichment failed", "file", filepath.Base(path), "error", err)
					continue
				}
				runtime.EventsEmit(a.ctx, "video-duration-ready", map[string]interface{}{
					"path":     path,
					"duration": dur,
				})
			}
		}()
	}

	wg.Wait()
	runtime.EventsEmit(a.ctx, "video-duration-complete")
}

// ScanDirectory returns all photos in the directory
func (a *App) ScanDirectory(path string) ([]model.Photo, error) {
	logger.Log.Info("Scanning directory", "path", path)
	_ = a.store.AddRecent(path)
	if a.OnAllowDir != nil {
		a.OnAllowDir(path)
	}
	photos, err := scanner.ScanDirectory(path)
	if err != nil {
		return nil, err
	}
	var videoPaths []string
	for i := range photos {
		if photos[i].IsVideo {
			videoPaths = append(videoPaths, photos[i].Path)
		}
	}
	if len(videoPaths) > 0 {
		a.startEnrichment(videoPaths)
	}
	return photos, nil
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
// Uses parallel goroutines (cfg.ThumbnailWorkers) for fast generation.
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
	items := make([]struct {
		Path    string
		ModTime time.Time
	}, len(photos))

	for i := range photos {
		items[i] = struct {
			Path    string
			ModTime time.Time
		}{Path: photos[i].Path, ModTime: photos[i].TakenAt}
	}

	// Parallel thumbnail generation with progress — use config value
	numWorkers := 4
	if a.cfg != nil {
		numWorkers = a.cfg.ThumbnailWorkers
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

	var videoPaths []string
	for i := range photos {
		if photos[i].IsVideo {
			videoPaths = append(videoPaths, photos[i].Path)
		}
	}
	if len(videoPaths) > 0 {
		a.startEnrichment(videoPaths)
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
		// Mark exported and clear selections in DB
		srcDir := ""
		if len(photos) > 0 {
			srcDir = filepath.Dir(photos[0].Path)
		}
		for i := range photos {
			_ = a.store.MarkExported(photos[i].Path)
			if srcDir != "" {
				_ = a.store.SaveSelection(photos[i].Path, srcDir, false)
			}
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
		_ = exec.Command("open", logPath).Start()
	case "windows":
		_ = exec.Command("cmd", "/c", "start", logPath).Start()
	default:
		_ = exec.Command("xdg-open", logPath).Start()
	}
}

// OpenFolderInFinder opens a folder in the native file manager
func (a *App) OpenFolderInFinder(path string) {
	switch stdruntime.GOOS {
	case "darwin":
		_ = exec.Command("open", path).Start()
	case "windows":
		_ = exec.Command("explorer", path).Start()
	default:
		_ = exec.Command("xdg-open", path).Start()
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

// emitSystemMetrics runs in a background goroutine, pushing metrics to the
// frontend via Wails events every second. Stops when the app context is cancelled.
func (a *App) emitSystemMetrics() {
	var (
		lastDiskRead  uint64
		lastDiskWrite uint64
		lastNetSent   uint64
		lastNetRecv   uint64
		lastUpdate    time.Time
		proc          *process.Process
	)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
		}

		var metrics SystemResources

		if proc == nil {
			p, err := process.NewProcess(int32(os.Getpid()))
			if err == nil {
				proc = p
			}
		}

		if proc != nil {
			cpuPercent, _ := proc.CPUPercent()
			metrics.CPU = cpuPercent

			ioCounters, _ := proc.IOCounters()
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

		var m stdruntime.MemStats
		stdruntime.ReadMemStats(&m)
		metrics.RAM = float64(m.Alloc) / 1024 / 1024

		netStats, _ := net.IOCounters(false)
		if len(netStats) > 0 {
			stat := netStats[0]
			if lastNetSent > 0 && lastNetRecv > 0 {
				metrics.NetSent = float64(stat.BytesSent-lastNetSent) / 1024
				metrics.NetRecv = float64(stat.BytesRecv-lastNetRecv) / 1024
			}
			lastNetSent = stat.BytesSent
			lastNetRecv = stat.BytesRecv
		}

		runtime.EventsEmit(a.ctx, "sys-metrics", metrics)
	}
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
	HasDuplicates  bool          `json:"hasDuplicates"`
	DuplicateCount int           `json:"duplicateCount"`
	Duplicates     []model.Photo `json:"duplicates"`
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
		isImage := ext == ".jpg" || ext == ".jpeg" || ext == ".png" || raw.IsRAWExt(ext)
		if isImage {
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
	_ = a.store.AddRecent(path)

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

	// Pre-scan for RAW+JPEG pairing
	emitProgress(0, 0, "Scanning directory...")
	scannedPhotos, scanErr := scanner.ScanDirectory(path)
	if scanErr != nil {
		logger.Log.Warn("raw: pre-scan failed, proceeding without pairing", "error", scanErr)
		scannedPhotos = nil
	}
	if scannedPhotos != nil {
		scannedPhotos = raw.PairRAWJPEG(scannedPhotos)
	}

	// Build RAW metadata lookup map
	rawMeta := make(map[string]model.Photo)
	for i := range scannedPhotos {
		if scannedPhotos[i].IsRAW || scannedPhotos[i].IsRAWCompanion {
			rawMeta[scannedPhotos[i].Path] = scannedPhotos[i]
		}
	}

	// Pre-extract EXIF dates in parallel to avoid repeated file reads later
	emitProgress(0, len(scannedPhotos), "Extracting photo dates...")
	var dateMu sync.Mutex
	dateCache := make(map[string]time.Time)

	var dateCount int32
	eg := new(errgroup.Group)
	eg.SetLimit(stdruntime.NumCPU())
	for i := range scannedPhotos {
		photoPath := scannedPhotos[i].Path
		eg.Go(func() error {
			date, valid := dedupe.ExtractDateTaken(photoPath)
			if valid {
				dateMu.Lock()
				dateCache[photoPath] = date
				dateMu.Unlock()
			}
			count := atomic.AddInt32(&dateCount, 1)
			if int(count)%10 == 0 || int(count) == len(scannedPhotos) {
				emitProgress(int(count), len(scannedPhotos), "Extracting photo dates...")
			}
			return nil
		})
	}
	_ = eg.Wait()

	logger.Log.Info("dedup: date extraction complete", "total", len(scannedPhotos), "withDates", len(dateCache))

	thumbnailDir := ""
	if a.thumbCache != nil {
		thumbnailDir = a.thumbCache.CacheDir()
	}

	groups, err := dedupe.FindDuplicates(appCtx, path, similarityThreshold, thumbnailDir, emitProgress)
	if err != nil {
		return nil, err
	}

	// 2. Select the best quality photo in each group to represent the unique
	err = dedupe.FindBestPhotos(appCtx, groups, a.thumbCache.CacheDir(), emitProgress)
	if err != nil {
		return nil, err
	}

	// 3. Sort groups chronologically
	emitProgress(0, len(groups), "Sorting by date...")
	err = dedupe.SortGroupsByDate(appCtx, groups, dateCache)
	if err != nil {
		return nil, err
	}

	// 4. Move duplicates physically
	errs := dedupe.RelocateGroupDuplicates(appCtx, groups, emitProgress)
	if len(errs) > 0 && errs[0] == context.Canceled {
		return nil, context.Canceled
	}

	// 5. Structure data for the frontend
	emitProgress(0, 0, "Building results...")
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

			date, hasCachedDate := dateCache[p.Path]
			if !hasCachedDate {
				date = info.ModTime() // fallback
			}

			photoModel := model.Photo{
				Path:    p.Path,
				Size:    info.Size(),
				TakenAt: date,
			}

			// Propagate RAW metadata from pre-scan pairing
			if meta, ok := rawMeta[p.Path]; ok {
				photoModel.IsRAW = meta.IsRAW
				photoModel.RAWFormat = meta.RAWFormat
				photoModel.CompanionPath = meta.CompanionPath
				photoModel.IsRAWCompanion = meta.IsRAWCompanion
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
