package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// CullSnapTheme implements a custom dark theme inspired by Lightroom.
// Dark grays, high contrast white text, electric blue accents.
type CullSnapTheme struct{}

var (
	// Pallete
	colBackground      = color.RGBA{R: 30, G: 30, B: 30, A: 255}    // #1E1E1E
	colInputBackground = color.RGBA{R: 45, G: 45, B: 45, A: 255}    // #2D2D2D
	colPrimary         = color.RGBA{R: 51, G: 153, B: 255, A: 255}  // #3399FF (Electric Blue)
	colText            = color.RGBA{R: 240, G: 240, B: 240, A: 255} // #F0F0F0
	colSelection       = color.RGBA{R: 60, G: 80, B: 100, A: 255}   // Subtle Blue overlay
	colGreen           = color.RGBA{R: 0, G: 200, B: 83, A: 255}    // #00C853 (Success/Exported)
)

func (t *CullSnapTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return colBackground
	case theme.ColorNameInputBackground:
		return colInputBackground
	case theme.ColorNameButton:
		return colInputBackground
	case theme.ColorNamePrimary, theme.ColorNameHyperlink, theme.ColorNameFocus:
		return colPrimary
	case theme.ColorNameForeground, theme.ColorNamePlaceHolder:
		return colText
	case theme.ColorNameSelection:
		return colSelection
	case theme.ColorNameScrollBar:
		return color.RGBA{R: 80, G: 80, B: 80, A: 100}
	}
	// Fallback
	return theme.DefaultTheme().Color(name, variant)
}

func (t *CullSnapTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *CullSnapTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *CullSnapTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
