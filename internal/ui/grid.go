package ui

import (
	"image/color"

	imageUtils "cullsnap/internal/image"
	"cullsnap/internal/model"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ThumbnailGrid manages the grid of photo thumbnails.
// We use a widget.List for virtualization, where each row contains N thumbnails.
type ThumbnailGrid struct {
	List *widget.List

	// Config
	Columns int

	// Data
	Photos         []model.Photo
	SelectedPhotos map[string]bool
	RejectedPhotos map[string]bool
	ExportedPhotos map[string]bool

	// Events
	OnPhotoSelected func(model.Photo)
}

func NewThumbnailGrid() *ThumbnailGrid {
	g := &ThumbnailGrid{
		Columns:        4,
		Photos:         []model.Photo{},
		SelectedPhotos: make(map[string]bool),
		RejectedPhotos: make(map[string]bool),
		ExportedPhotos: make(map[string]bool),
	}

	g.List = widget.NewList(
		func() int {
			// Calculate number of rows needed
			return (len(g.Photos) + g.Columns - 1) / g.Columns
		},
		func() fyne.CanvasObject {
			row := container.NewGridWithColumns(g.Columns)
			for i := 0; i < g.Columns; i++ {
				// Each cell: Stack(Bg, Image, SelectionBorder, SelectionBadge, ExportedBadge, NoPreview, TappableOverlay)

				// 1. Background (Card-like)
				bg := canvas.NewRectangle(theme.InputBackgroundColor())

				// 2. Image (Centered with Padding)
				img := canvas.NewImageFromResource(nil)
				img.FillMode = canvas.ImageFillContain
				img.SetMinSize(fyne.NewSize(120, 120))

				// 3. Selection Border (Thick Green)
				border := canvas.NewRectangle(color.Transparent)
				border.StrokeColor = color.RGBA{76, 175, 80, 255} // Green #4CAF50
				border.StrokeWidth = 4
				border.Hide()

				// 4. Selection Badge (Green Checkmark Overlay Top-Right)
				// Use color icon via themed resource manipulation or just custom tint?
				// Simple approach: Green Icon
				// Fyne icons are white/black by default. Let's assume theme is dark, so white icon is fine.
				// But we want Green circle context.
				// Let's stick to Checkmark for now, maybe tint background.
				// Simplification: Just the Icon, we rely on the Border for "Green".
				selectIcon := widget.NewIcon(theme.ConfirmIcon())
				selectBadge := container.NewVBox(
					container.NewHBox(layout.NewSpacer(), selectIcon),
					layout.NewSpacer(),
				)
				selectBadge.Hide()

				// 5. Exported Indicator (Text Badge Top-Left)
				exportedLabel := canvas.NewText("EXPORTED", color.RGBA{0, 200, 0, 255})
				exportedLabel.TextStyle.Bold = true
				exportedLabel.TextSize = 10
				exportedBadge := container.NewVBox(
					container.NewHBox(exportedLabel, layout.NewSpacer()),
					layout.NewSpacer(),
				)
				exportedBadge.Hide()

				// 6. No Preview Placeholder (Hidden by default)
				npText := canvas.NewText("No Preview", color.RGBA{150, 150, 150, 255})
				npText.Alignment = fyne.TextAlignCenter
				noPreview := container.NewCenter(npText)
				noPreview.Hide()

				// 7. Reject Overlay (Red Tint)
				rejectOverlay := canvas.NewRectangle(color.RGBA{255, 0, 0, 50})
				rejectOverlay.Hide()

				// 8. Tappable Overlay
				overlay := newTappableContent(nil)

				// Stack em up
				// Note: Use container.NewPadded(img) to give image some breathing room from edges
				cell := container.NewStack(bg, container.NewPadded(img), rejectOverlay, border, selectBadge, exportedBadge, noPreview, overlay)
				row.Add(cell)
			}
			return row
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			row := obj.(*fyne.Container)
			startIdx := id * g.Columns

			for i := 0; i < g.Columns; i++ {
				idx := startIdx + i
				cell := row.Objects[i].(*fyne.Container)

				if idx < len(g.Photos) {
					photo := g.Photos[idx]

					// Get widgets: [bg, padded, rejectOverlay, border, selectBadge, exportedBadge, noPreview, overlay]
					paddedCont := cell.Objects[1].(*fyne.Container)
					imgWidget := paddedCont.Objects[0].(*canvas.Image)

					rejectOverlay := cell.Objects[2].(*canvas.Rectangle)
					border := cell.Objects[3].(*canvas.Rectangle)
					selectBadge := cell.Objects[4].(*fyne.Container)
					exportedBadge := cell.Objects[5].(*fyne.Container)
					noPreview := cell.Objects[6].(*fyne.Container)
					overlay := cell.Objects[7].(*tappableContent)

					cell.Show()

					// Update Tapped Handler
					overlay.OnTapped = func() {
						if g.OnPhotoSelected != nil {
							g.OnPhotoSelected(photo)
						}
					}

					// Async Thumbnail Loading
					imgWidget.File = ""
					imgWidget.Image = nil
					imgWidget.Refresh()
					noPreview.Hide()

					go func(p string, w *canvas.Image, np *fyne.Container) {
						thumb, err := imageUtils.GetThumbnail(p)
						fyne.CurrentApp().Driver().DoFromGoroutine(func() {
							if err == nil {
								w.Image = thumb
								w.Refresh()
								w.Show()
								np.Hide()
							} else {
								w.Hide()
								np.Show()
							}
						}, false)
					}(photo.Path, imgWidget, noPreview)

					// Selection State (KEEP)
					if g.SelectedPhotos[photo.Path] {
						border.Show()
						selectBadge.Show()
						// Reset Reject visual just in case
						rejectOverlay.Hide()
						imgWidget.Translucency = 0
					} else if g.RejectedPhotos[photo.Path] {
						// REJECT State
						border.Hide()
						selectBadge.Hide()
						rejectOverlay.Show()
						imgWidget.Translucency = 0.5 // Dim it
					} else {
						// NEUTRAL
						border.Hide()
						selectBadge.Hide()
						rejectOverlay.Hide()
						imgWidget.Translucency = 0
					}

					// Exported State
					if g.ExportedPhotos[photo.Path] {
						exportedBadge.Show()
					} else {
						exportedBadge.Hide()
					}

				} else {
					cell.Hide()
				}
			}
		},
	)

	return g
}

func (g *ThumbnailGrid) SetPhotos(photos []model.Photo) {
	g.Photos = photos
	g.List.Refresh()
}

// Tappable wrapper
type tappableContent struct {
	widget.BaseWidget
	OnTapped func()
}

func newTappableContent(onTapped func()) *tappableContent {
	t := &tappableContent{OnTapped: onTapped}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tappableContent) CreateRenderer() fyne.WidgetRenderer {
	// Invisible
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

func (t *tappableContent) Tapped(_ *fyne.PointEvent) {
	if t.OnTapped != nil {
		t.OnTapped()
	}
}
