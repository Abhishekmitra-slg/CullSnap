package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// NewWelcomeScreen creates the "Empty State" view
func NewWelcomeScreen(onOpen func()) fyne.CanvasObject {
	icon := widget.NewIcon(theme.FolderOpenIcon())
	// Force large size via scale? Fyne icons scale with text usually.
	// We can use a button as an icon carrier or just layout.
	// Better: Use a custom layout or just a big label.
	// Actually theme icons are scalable.

	title := widget.NewLabel("No Folder Selected")
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	subtitle := widget.NewLabel("Open a folder to start culling.")
	subtitle.Alignment = fyne.TextAlignCenter

	btn := widget.NewButtonWithIcon("Open Folder", theme.FolderOpenIcon(), onOpen)
	btn.Importance = widget.HighImportance

	// Center everything
	content := container.NewVBox(
		// Spacer to push down
		layout.NewSpacer(),
		container.NewCenter(icon), // Icon might be small, acceptable for now
		title,
		subtitle,
		container.NewCenter(btn),
		layout.NewSpacer(),
	)

	return container.NewCenter(content)
}
