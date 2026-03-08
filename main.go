package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
	requestedFilename := strings.TrimPrefix(req.URL.Path, "/")

	// Ensure we only serve local absolute paths
	if strings.HasPrefix(req.URL.Path, "/") {
		requestedFilename = req.URL.Path
	} else {
		requestedFilename = "/" + requestedFilename
	}

	fileData, err := os.ReadFile(requestedFilename)
	if err != nil {
		res.WriteHeader(http.StatusNotFound)
		res.Write([]byte(fmt.Sprintf("Could not load file %s", requestedFilename)))
		return
	}
	res.Write(fileData)
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
	app := NewApp(store)

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
		OnStartup:        app.Startup,
		Bind: []interface{}{
			app,
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
