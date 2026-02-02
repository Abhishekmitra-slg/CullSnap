package main

import (
	"cullsnap/internal/assets"
	"cullsnap/internal/logger"
	"cullsnap/internal/storage"
	"cullsnap/internal/ui"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func main() {
	// Determine App Data Directory
	configDir, err := os.UserConfigDir()
	if err != nil {
		panic(err)
	}
	appDir := filepath.Join(configDir, "CullSnap")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		panic(err)
	}

	logPath := filepath.Join(appDir, "cullsnap.log")
	dbPath := filepath.Join(appDir, "cullsnap.db")

	// Init Logger
	if err := logger.Init(logPath); err != nil {
		panic(err)
	}
	logger.Log.Info("Application starting", "dir", appDir)

	a := app.NewWithID("com.cullsnap.app")
	a.SetIcon(assets.AppIcon)
	a.Settings().SetTheme(&ui.CullSnapTheme{})
	w := a.NewWindow("CullSnap - Photo Culling")

	w.Resize(fyne.NewSize(1200, 800))

	// Init Storage
	store, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		logger.Log.Error("CRITICAL: Failed to init storage", "error", err)
		dialog.ShowError(err, w)
		w.SetContent(widget.NewLabel("Critical Error: Failed to initialize database.\nCheck logs for details."))
		w.ShowAndRun()
		return
	}
	defer store.Close()

	// Pass store to layout
	content := ui.SetupMainLayout(w, store)
	w.SetContent(content)

	w.ShowAndRun()
}
