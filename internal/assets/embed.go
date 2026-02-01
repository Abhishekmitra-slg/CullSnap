package assets

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed icon.png
var iconBytes []byte

// AppIcon is the globally accessible resource for the application icon.
var AppIcon = fyne.NewStaticResource("icon.png", iconBytes)
