package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"cullsnap/internal/app"
	"cullsnap/internal/heic"
	"cullsnap/internal/logger"
	"cullsnap/internal/raw"
	"cullsnap/internal/storage"
	"cullsnap/internal/video"
	"embed"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed CONTRIBUTORS.yml
var contributorsYML string

//go:embed keys/update_signing.pub
var updatePublicKey []byte

//go:embed CHANGELOG.md
var changelogMD string

// version and Google Drive OAuth credentials are set at build time via ldflags:
//
//	-X main.version=vX.Y.Z
//	-X main.googleDriveClientID=<id>
//	-X main.googleDriveClientSecret=<secret>
//
// For local development, set GOOGLE_DRIVE_CLIENT_ID and GOOGLE_DRIVE_CLIENT_SECRET env vars.
var (
	version                 = "dev"
	googleDriveClientID     = "" // injected via ldflags in CI
	googleDriveClientSecret = "" // injected via ldflags in CI
)

type FileLoader struct {
	sem           chan struct{}      // limits concurrent file-serving goroutines
	serverCtx     context.Context    // cancelled on app shutdown
	cacheDir      string             // for Cache-Control header detection
	allowedMu     sync.RWMutex       // protects allowedDirs
	allowedDirs   []string           // directories the user has explicitly opened
	heicGroup     singleflight.Group // dedup concurrent HEIC conversions
	useNativeSips bool               // controls sips vs FFmpeg for HEIC conversion
}

// AllowDirectory adds a directory to the allowlist of paths the media server may serve.
func (h *FileLoader) AllowDirectory(dir string) {
	cleaned := filepath.Clean(dir)
	h.allowedMu.Lock()
	defer h.allowedMu.Unlock()
	for _, d := range h.allowedDirs {
		if d == cleaned {
			return
		}
	}
	h.allowedDirs = append(h.allowedDirs, cleaned)
}

// isPathAllowed checks whether filePath falls inside any allowed directory or the cache dir.
func (h *FileLoader) isPathAllowed(filePath string) bool {
	cleaned := filepath.Clean(filePath)
	// Always allow serving from the thumbnail cache
	if strings.HasPrefix(cleaned, h.cacheDir) {
		return true
	}
	h.allowedMu.RLock()
	defer h.allowedMu.RUnlock()
	for _, dir := range h.allowedDirs {
		if strings.HasPrefix(cleaned, dir) {
			return true
		}
	}
	return false
}

func (h *FileLoader) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	// Allow any origin — the media server is bound to 127.0.0.1 and validates
	// paths against an explicit allowlist, so CORS adds no meaningful protection.
	// The Wails webview origin varies by platform and mode.
	res.Header().Set("Access-Control-Allow-Origin", "*")
	res.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	res.Header().Set("Access-Control-Allow-Headers", "Range")
	res.Header().Set("Access-Control-Expose-Headers", "Content-Range, Content-Length, Accept-Ranges")

	if req.Method == "OPTIONS" {
		res.WriteHeader(http.StatusOK)
		return
	}

	// Acquire semaphore slot — blocks if maxConnections are already active.
	// context.Done() escape ensures we don't leak goroutines on app shutdown.
	select {
	case h.sem <- struct{}{}:
		defer func() { <-h.sem }()
	case <-h.serverCtx.Done():
		http.Error(res, "server shutting down", http.StatusServiceUnavailable)
		return
	}

	requestedPath := req.URL.Path

	if strings.HasPrefix(requestedPath, "/wails-media") {
		filePath := req.URL.Query().Get("path")
		if filePath == "" {
			http.Error(res, "Missing path parameter", http.StatusBadRequest)
			return
		}

		// Path traversal protection: only serve files within user-opened directories or cache
		if !h.isPathAllowed(filePath) {
			logger.Log.Warn("MediaServer blocked request for path outside allowed directories", "path", filePath)
			http.Error(res, "Forbidden", http.StatusForbidden)
			return
		}

		logger.Log.Debug("MediaServer serving media", "path", filePath)

		// RAW files: extract and serve JPEG preview instead of raw binary
		ext := strings.ToLower(filepath.Ext(filePath))
		if raw.IsRAWExt(ext) {
			logger.Log.Debug("media: serving RAW preview", "path", filePath)

			// Try cache first
			if cached, err := raw.GetCachedPreview(filePath); err == nil {
				res.Header().Set("Content-Type", "image/jpeg")
				res.Header().Set("Cache-Control", "private, max-age=3600")
				io.Copy(res, bytes.NewReader(cached)) //nolint:errcheck,gosec // best-effort binary JPEG write
				return
			}

			// Extract and cache
			start := time.Now()
			previewBytes, err := raw.ExtractPreview(filePath)
			if err != nil {
				logger.Log.Error("media: RAW preview extraction failed", "path", filePath, "error", err)
				http.Error(res, "failed to extract RAW preview", http.StatusInternalServerError)
				return
			}

			_ = raw.CachePreview(filePath, previewBytes)
			logger.Log.Debug("media: RAW preview served", "path", filePath, "bytes", len(previewBytes), "duration", time.Since(start))

			res.Header().Set("Content-Type", "image/jpeg")
			res.Header().Set("Cache-Control", "private, max-age=3600")
			io.Copy(res, bytes.NewReader(previewBytes)) //nolint:errcheck,gosec // best-effort binary JPEG write
			return
		}

		// HEIC/HEIF: convert to JPEG, cache, and serve
		if ext == ".heic" || ext == ".heif" {
			heicCacheDir := filepath.Join(h.cacheDir, "heic")
			os.MkdirAll(heicCacheDir, 0o700) //nolint:errcheck,gosec // best-effort cache dir creation
			cacheKey := fmt.Sprintf("%x", sha256.Sum256([]byte(filePath)))
			cachedPath := filepath.Join(heicCacheDir, cacheKey+".jpg")

			// Check cache first
			if _, err := os.Stat(cachedPath); err == nil {
				logger.Log.Debug("media: serving cached HEIC conversion", "path", filePath)
				res.Header().Set("Content-Type", "image/jpeg")
				res.Header().Set("Cache-Control", "public, max-age=86400")
				http.ServeFile(res, req, cachedPath)
				return
			}

			// Convert with singleflight to dedup concurrent requests for same file
			logger.Log.Debug("media: converting HEIC to JPEG", "path", filePath, "useSips", h.useNativeSips)
			_, err, _ := h.heicGroup.Do(cacheKey, func() (interface{}, error) {
				return nil, heic.ConvertToJPEG(filePath, cachedPath, h.useNativeSips)
			})
			if err != nil {
				logger.Log.Error("HEIC conversion failed", "path", filePath, "error", err)
				http.Error(res, "HEIC conversion failed", http.StatusInternalServerError)
				return
			}

			logger.Log.Debug("media: HEIC conversion complete", "path", filePath, "cached", cachedPath)
			res.Header().Set("Content-Type", "image/jpeg")
			res.Header().Set("Cache-Control", "public, max-age=86400")
			http.ServeFile(res, req, cachedPath)
			return
		}

		setCacheControl(res, filePath, h.cacheDir)
		http.ServeFile(res, req, filePath)
		return
	}

	// Reject fallback path — only /wails-media with validated paths is allowed
	http.Error(res, "Not found", http.StatusNotFound)
}

var videoExtensions = map[string]bool{
	".mp4": true, ".mov": true, ".mkv": true, ".webm": true, ".avi": true,
}

// setCacheControl sets the Cache-Control header based on the served file type.
func setCacheControl(w http.ResponseWriter, filePath, cacheDir string) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch {
	case strings.HasPrefix(filePath, cacheDir):
		// Thumbnails: long-lived public cache
		w.Header().Set("Cache-Control", "public, max-age=86400")
	case videoExtensions[ext]:
		// Videos: never cache — browser cache unsuitable for large range-request files
		w.Header().Set("Cache-Control", "no-store")
	default:
		// Full-res photos: short private cache (1 hour) avoids re-fetching large originals
		w.Header().Set("Cache-Control", "private, max-age=3600")
	}
}

// trackingWriter wraps http.ResponseWriter and records whether WriteHeader has been called.
// Used by panicRecoveryMiddleware to avoid a "superfluous WriteHeader" warning when
// http.ServeFile has already started writing before the panic occurred.
type trackingWriter struct {
	http.ResponseWriter
	headerWritten bool
}

func (tw *trackingWriter) WriteHeader(status int) {
	tw.headerWritten = true
	tw.ResponseWriter.WriteHeader(status)
}

func (tw *trackingWriter) Write(b []byte) (int, error) {
	tw.headerWritten = true
	return tw.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter, enabling http.ResponseController
// to traverse wrapper chains (e.g. for Flush or SetWriteDeadline).
func (tw *trackingWriter) Unwrap() http.ResponseWriter {
	return tw.ResponseWriter
}

// panicRecoveryMiddleware wraps any HTTP handler with a deferred recover.
// If the handler panics (e.g. broken pipe when client disconnects mid-stream),
// the panic is logged and a 500 is returned rather than crashing the process.
func panicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tw := &trackingWriter{ResponseWriter: w}
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("MediaServer: recovered from panic: %v", rec)
				if !tw.headerWritten {
					tw.WriteHeader(http.StatusInternalServerError)
				}
			}
		}()
		next.ServeHTTP(tw, r)
	})
}

func main() {
	// Register video MIME types explicitly.
	// mime.AddExtensionType overrides whatever the OS registry/database has,
	// ensuring correct Content-Type on all platforms (Windows registry is missing
	// mkv/webm; macOS may not have all).
	_ = mime.AddExtensionType(".mov", "video/quicktime")
	_ = mime.AddExtensionType(".mkv", "video/x-matroska")
	_ = mime.AddExtensionType(".webm", "video/webm")
	_ = mime.AddExtensionType(".mp4", "video/mp4")
	_ = mime.AddExtensionType(".avi", "video/x-msvideo")

	// Determine App Data Directory
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	appDir := filepath.Join(configDir, "CullSnap")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		log.Fatal(err)
	}

	logPath := filepath.Join(appDir, "cullsnap.log")
	dbPath := filepath.Join(appDir, "cullsnap.db")

	// Init Logger
	if err := logger.Init(logPath); err != nil {
		log.Fatal(err)
	}
	logger.Log.Info("Application starting", "dir", appDir)

	// Init Video (FFmpeg)
	if err := video.Init(); err != nil {
		logger.Log.Error("Failed to init video support (FFmpeg): ", "error", err)
		// We don't fatal here, as the user can still use CullSnap for photos
	}

	// Init RAW preview cache
	if err := raw.InitPreviewCache(); err != nil {
		logger.Log.Warn("Failed to initialize preview cache", "error", err)
	}

	// Init Storage
	store, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		logger.Log.Error("CRITICAL: Failed to init storage", "error", err)
		log.Fatal(err)
	}
	defer store.Close() //nolint:errcheck // best-effort cleanup on exit

	// serverCtx is cancelled in OnShutdown so semaphore-blocked goroutines exit cleanly.
	serverCtx, serverCancel := context.WithCancel(context.Background())

	// Read MaxConnections from persisted config; fall back to 20 if not set.
	maxConn := 20
	if val, _ := store.GetConfig("maxConnections"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n >= 10 && n <= 50 {
			maxConn = n
		}
	}
	sem := make(chan struct{}, maxConn)

	// Read cacheDir from persisted config; fall back to OS default.
	cacheDir := ""
	if val, _ := store.GetConfig("cacheDir"); val != "" {
		cacheDir = val
	}
	if cacheDir == "" {
		userCacheDir, _ := os.UserCacheDir()
		cacheDir = filepath.Join(userCacheDir, "CullSnap", "thumbs")
	}

	// Read useNativeSips from persisted config; fall back to platform default (true on macOS).
	useNativeSips := true
	if val, _ := store.GetConfig("useNativeSips"); val == "false" {
		useNativeSips = false
	}

	loader := &FileLoader{sem: sem, serverCtx: serverCtx, cacheDir: cacheDir, useNativeSips: useNativeSips}

	go func() {
		mediaServer := &http.Server{
			Addr:         "127.0.0.1:34342",
			Handler:      panicRecoveryMiddleware(loader),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 0, // MUST be 0 — streaming large video files has no fixed deadline
			IdleTimeout:  30 * time.Second,
			BaseContext:  func(_ net.Listener) context.Context { return serverCtx },
		}
		go func() {
			<-serverCtx.Done()
			mediaServer.Close() //nolint:errcheck,gosec // best-effort shutdown
		}()
		if err := mediaServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Failed to start media server: %v", err)
		}
	}()

	// Create an instance of the app structure
	application := app.NewApp(store)
	application.OnAllowDir = loader.AllowDirectory
	application.Version = version
	application.ContributorsRaw = contributorsYML
	application.ChangelogRaw = changelogMD
	application.UpdatePublicKey = updatePublicKey

	// Google Drive OAuth credentials: ldflags (CI) → env vars (local dev)
	gdID := googleDriveClientID
	gdSecret := googleDriveClientSecret
	if gdID == "" {
		gdID = os.Getenv("GOOGLE_DRIVE_CLIENT_ID")
	}
	if gdSecret == "" {
		gdSecret = os.Getenv("GOOGLE_DRIVE_CLIENT_SECRET")
	}
	application.GoogleDriveClientID = gdID
	application.GoogleDriveClientSecret = gdSecret

	// Build application menu (macOS menu bar / Windows menu)
	appMenu := buildAppMenu()

	// Create application with options
	var appCtx context.Context

	// Wire menu item callbacks to emit events after context is available.
	appMenu.Items[1].SubMenu.Items[0].Click = func(_ *menu.CallbackData) { wailsRuntime.EventsEmit(appCtx, "menu:open-folder") }
	appMenu.Items[2].SubMenu.Items[0].Click = func(_ *menu.CallbackData) { wailsRuntime.EventsEmit(appCtx, "menu:settings") }
	appMenu.Items[2].SubMenu.Items[2].Click = func(_ *menu.CallbackData) { wailsRuntime.EventsEmit(appCtx, "menu:ai-panel") }

	err = wails.Run(&options.App{
		Menu:             appMenu,
		Title:            "CullSnap",
		Width:            1200,
		Height:           800,
		MinWidth:         1100,
		MinHeight:        700,
		WindowStartState: options.Maximised,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: loader,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 24, B: 27, A: 1}, // Matches zinc-900 (modern dark theme)
		OnStartup: func(ctx context.Context) {
			appCtx = ctx
			application.Startup(ctx)
		},
		OnShutdown: func(ctx context.Context) {
			serverCancel()
			application.Shutdown()
		},
		Bind: []interface{}{
			application,
		},
		Mac: &mac.Options{
			TitleBar: &mac.TitleBar{
				TitlebarAppearsTransparent: true,
				HideTitle:                  true,
				HideTitleBar:               false,
				FullSizeContent:            false,
				UseToolbar:                 false,
				HideToolbarSeparator:       true,
			},
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
		},
	})
	if err != nil {
		logger.Log.Error("Error starting Wails", "error", err)
	}
}

// buildAppMenu creates the macOS/Windows application menu bar.
// Menu item Click callbacks are wired after construction (they need appCtx).
func buildAppMenu() *menu.Menu {
	appMenu := menu.NewMenu()

	// App menu (macOS only — auto-mapped by Wails)
	appMenu.Append(menu.AppMenu())

	// File menu
	fileMenu := appMenu.AddSubmenu("File")
	fileMenu.AddText("Open Folder...", keys.CmdOrCtrl("o"), nil) // [0] — wired later
	fileMenu.AddSeparator()                                      // [1]
	fileMenu.AddText("Close Window", keys.CmdOrCtrl("w"), func(_ *menu.CallbackData) {})

	// View menu
	viewMenu := appMenu.AddSubmenu("View")
	viewMenu.AddText("Settings", keys.CmdOrCtrl(","), nil)        // [0] — wired later
	viewMenu.AddSeparator()                                       // [1]
	viewMenu.AddText("Toggle AI Panel", keys.CmdOrCtrl("i"), nil) // [2] — wired later

	// Edit menu
	appMenu.Append(menu.EditMenu())

	// Window menu
	windowMenu := appMenu.AddSubmenu("Window")
	windowMenu.AddText("Minimize", keys.CmdOrCtrl("m"), func(_ *menu.CallbackData) {})
	windowMenu.AddText("Zoom", nil, func(_ *menu.CallbackData) {})

	// Help menu
	helpMenu := appMenu.AddSubmenu("Help")
	helpMenu.AddText("CullSnap Help", nil, nil)

	return appMenu
}
