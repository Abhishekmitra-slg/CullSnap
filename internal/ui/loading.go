package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// NewLoadingScreen creates the overlay shown during long operations.
func NewLoadingScreen() fyne.CanvasObject {
	progressBar := widget.NewProgressBarInfinite()
	progressBar.Start()

	label := widget.NewLabel("Loading photos...")
	label.TextStyle = fyne.TextStyle{Bold: true}
	label.Alignment = fyne.TextAlignCenter

	subtext := widget.NewLabel("Scanning for RAW files and thumbnails...")
	subtext.Alignment = fyne.TextAlignCenter

	content := container.NewVBox(
		layout.NewSpacer(),
		label,
		progressBar,
		subtext,
		layout.NewSpacer(),
	)

	// Wrap in a Center container to ensure it floats in the middle
	return container.NewCenter(content)
}
