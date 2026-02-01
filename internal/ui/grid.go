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
	ExportedPhotos map[string]bool

	// Events
	OnPhotoSelected func(model.Photo)
}

func NewThumbnailGrid() *ThumbnailGrid {
	g := &ThumbnailGrid{
		Columns:        4,
		Photos:         []model.Photo{},
		SelectedPhotos: make(map[string]bool),
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
				// Each cell: Stack(Rectangle(Border), Image, TappableOverlay)
				bg := canvas.NewRectangle(theme.BackgroundColor())

				// Image (CanvasObject)
				img := canvas.NewImageFromResource(nil)
				img.FillMode = canvas.ImageFillContain
				img.SetMinSize(fyne.NewSize(100, 100))

				// Border
				border := canvas.NewRectangle(color.Transparent)
				border.StrokeColor = theme.PrimaryColor()
				border.StrokeWidth = 3
				border.Hide()

				// Exported Indicator (Top-Right Checkmark)
				exportedIcon := widget.NewIcon(theme.ConfirmIcon())
				// Green visual? Icon color defaults to text color.
				// We can't easily change icon color in Fyne v2 without custom theme or resource modification.
				// But ConfirmIcon is usually distinctive.
				// Wrapper for alignment
				exportedLayer := container.NewVBox(
					container.NewHBox(layout.NewSpacer(), exportedIcon),
					layout.NewSpacer(),
				)
				exportedLayer.Hide()

				// Tappable Overlay - Handler set in UpdateItem
				overlay := newTappableContent(nil)

				cell := container.NewStack(bg, img, border, exportedLayer, overlay)
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

					// Get widgets
					imgWidget := cell.Objects[1].(*canvas.Image)
					border := cell.Objects[2].(*canvas.Rectangle)
					exportedLayer := cell.Objects[3].(*fyne.Container)
					overlay := cell.Objects[4].(*tappableContent)

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

					go func(p string, w *canvas.Image) {
						thumb, err := imageUtils.GetThumbnail(p)
						if err == nil {
							fyne.CurrentApp().Driver().DoFromGoroutine(func() {
								w.Image = thumb
								w.Refresh()
							}, false)
						}
					}(photo.Path, imgWidget)

					// Selection
					if g.SelectedPhotos[photo.Path] {
						border.Show()
					} else {
						border.Hide()
					}

					// Exported
					if g.ExportedPhotos[photo.Path] {
						exportedLayer.Show()
					} else {
						exportedLayer.Hide()
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
