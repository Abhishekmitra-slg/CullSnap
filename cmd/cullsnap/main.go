package main

import (
	"cullsnap/internal/logger"
	"cullsnap/internal/storage"
	"cullsnap/internal/ui"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

func main() {
	// Init Logger
	if err := logger.Init("cullsnap.log"); err != nil {
		panic(err)
	}
	logger.Log.Info("Application starting")

	a := app.NewWithID("com.cullsnap.app")
	w := a.NewWindow("CullSnap - Photo Culling")

	w.Resize(fyne.NewSize(1200, 800))

	// Init Storage
	store, err := storage.NewSQLiteStore("cullsnap.db")
	if err != nil {
		logger.Log.Error("Failed to init storage", "error", err)
	} else {
		defer store.Close()
	}

	// Pass store to layout
	content := ui.SetupMainLayout(w, store)
	w.SetContent(content)

	w.ShowAndRun()
}
