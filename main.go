package main

import (
	"embed"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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

type FileLoader struct{}

func (h *FileLoader) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	// Add CORS headers for the secondary port access
	res.Header().Set("Access-Control-Allow-Origin", "*")
	res.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	res.Header().Set("Access-Control-Allow-Headers", "Range")
	res.Header().Set("Access-Control-Expose-Headers", "Content-Range, Content-Length, Accept-Ranges")

	if req.Method == "OPTIONS" {
		res.WriteHeader(http.StatusOK)
		return
	}

	requestedPath := req.URL.Path
	
	// If the request is specifically for our media endpoint, extract the parameter
	if strings.HasPrefix(requestedPath, "/wails-media") {
		filePath := req.URL.Query().Get("path")
		if filePath == "" {
			http.Error(res, "Missing path parameter", http.StatusBadRequest)
			return
		}
		
		logger.Log.Debug("MediaServer serving media", "path", filePath)
		
		// Use http.ServeFile to handle range requests, content types, etc.
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
	
	http.ServeFile(res, req, requestedFilename)
}

func main() {
	// Start a dedicated media server on a fixed port to bypass Wails/Vite dev mode routing issues
	go func() {
		// Use a local server instance to ensure it's distinct from Wails internal asset server
		loader := &FileLoader{}
		err := http.ListenAndServe(":34342", loader)
		if err != nil {
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
			Handler: &FileLoader{},
		},
		BackgroundColour: &options.RGBA{R: 24, G: 24, B: 27, A: 1}, // Matches zinc-900 (modern dark theme)
		OnStartup:        application.Startup,
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
