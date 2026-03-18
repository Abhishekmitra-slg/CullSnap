package main

import (
	"context"
	"embed"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cullsnap/internal/app"
	"cullsnap/internal/logger"
	"cullsnap/internal/storage"
	"cullsnap/internal/video"


	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

type FileLoader struct {
	sem       chan struct{}    // limits concurrent file-serving goroutines
	serverCtx context.Context // cancelled on app shutdown
	cacheDir  string          // for Cache-Control header detection
}

func (h *FileLoader) ServeHTTP(res http.ResponseWriter, req *http.Request) {
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

		logger.Log.Debug("MediaServer serving media", "path", filePath)

		// Set Cache-Control based on content type:
		// - thumbnails (in cache dir): long-lived public cache
		// - videos: no-store (range requests go to server; browser cache unsuitable for large files)
		// - full-res photos: short private cache (avoids re-fetching 40MB images in the viewer)
		setCacheControl(res, filePath, h.cacheDir)

		http.ServeFile(res, req, filePath)
		return
	}

	logger.Log.Debug("MediaServer fallback", "path", requestedPath)

	requestedFilename, err := url.PathUnescape(requestedPath)
	if err != nil {
		requestedFilename = requestedPath
	}
	if !strings.HasPrefix(requestedFilename, "/") {
		requestedFilename = "/" + requestedFilename
	}
	setCacheControl(res, requestedFilename, h.cacheDir)
	http.ServeFile(res, req, requestedFilename)
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
	mime.AddExtensionType(".mov", "video/quicktime")
	mime.AddExtensionType(".mkv", "video/x-matroska")
	mime.AddExtensionType(".webm", "video/webm")
	mime.AddExtensionType(".mp4", "video/mp4")
	mime.AddExtensionType(".avi", "video/x-msvideo")

	// serverCtx is cancelled in OnShutdown so semaphore-blocked goroutines exit cleanly.
	serverCtx, serverCancel := context.WithCancel(context.Background())

	// Semaphore: limits concurrent file-serving goroutines.
	// Default 20 — overridden by AppConfig after config is loaded (see app.go).
	const defaultMaxConnections = 20
	sem := make(chan struct{}, defaultMaxConnections)

	// cacheDir is set from AppConfig after config loads; use OS default for now.
	// This value is updated once AppConfig is wired in (Chunk 3).
	userCacheDir, _ := os.UserCacheDir()
	cacheDir := filepath.Join(userCacheDir, "CullSnap", "thumbs")

	loader := &FileLoader{sem: sem, serverCtx: serverCtx, cacheDir: cacheDir}

	go func() {
		mediaServer := &http.Server{
			Addr:         ":34342",
			Handler:      panicRecoveryMiddleware(loader),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 0, // MUST be 0 — streaming large video files has no fixed deadline
			IdleTimeout:  30 * time.Second,
			BaseContext:  func(_ net.Listener) context.Context { return serverCtx },
		}
		// Shut down the HTTP server when the app context is cancelled (OnShutdown).
		// Without this, the goroutine keeps holding port 34342 and the next
		// hot-reload attempt fails with "address already in use".
		go func() {
			<-serverCtx.Done()
			mediaServer.Close()
		}()
		if err := mediaServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Failed to start media server: %v", err)
		}
	}()

	// Determine App Data Directory
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	appDir := filepath.Join(configDir, "CullSnap")
	if err := os.MkdirAll(appDir, 0755); err != nil {
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

	// Init Storage
	store, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		logger.Log.Error("CRITICAL: Failed to init storage", "error", err)
		log.Fatal(err)
	}
	defer store.Close()

	// Create an instance of the app structure
	application := app.NewApp(store)

	// Create application with options
	err = wails.Run(&options.App{
		Title:  "CullSnap",
		Width:  1200,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: loader,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 24, B: 27, A: 1}, // Matches zinc-900 (modern dark theme)
		OnStartup:        application.Startup,
		OnShutdown:       func(ctx context.Context) { serverCancel() },
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
