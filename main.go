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

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

type FileLoader struct{}

func (h *FileLoader) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	// URL-decode the path to handle special characters (spaces, &, etc.)
	requestedFilename, err := url.PathUnescape(req.URL.Path)
	if err != nil {
		requestedFilename = req.URL.Path
	}

	// Ensure absolute path
	if !strings.HasPrefix(requestedFilename, "/") {
		requestedFilename = "/" + requestedFilename
	}

	// Use http.ServeFile — it handles:
	// - Content-Type detection from file extension
	// - Range requests for efficient partial reads
	// - Proper caching headers
	// - Streaming (doesn't load full file into memory)
	http.ServeFile(res, req, requestedFilename)
}

func main() {
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
