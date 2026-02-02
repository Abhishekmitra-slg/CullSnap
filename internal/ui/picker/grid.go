package picker

import (
	"image/color"
	"io/fs"
	"math"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// FileGrid displays folders and files in a virtual grid using widget.Table
type FileGrid struct {
	Table      *widget.Table
	Columns    int
	Items      []fs.DirEntry
	CurrentDir string

	// Events
	OnNavigate func(path string)
	OnSelect   func(path string) // Single selection
	OnHover    func(name string) // For Status Bar
	Selected   string
}

// FileCard represents a single file/folder cell in the grid
type FileCard struct {
	widget.BaseWidget

	// Data
	FullPath string
	Name     string
	IsDir    bool

	// UI
	bg    *canvas.Rectangle
	icon  *canvas.Image // Using Image for potential thumbnails, or standard icons
	label *widget.Label

	// Interactions
	OnTapped func(path string, isDir bool)
	OnHover  func(name string)
}

func NewFileCard() *FileCard {
	c := &FileCard{
		bg:    canvas.NewRectangle(color.Transparent),
		icon:  canvas.NewImageFromResource(theme.FolderIcon()), // Default
		label: widget.NewLabel(""),
	}
	c.bg.CornerRadius = 6

	c.icon.FillMode = canvas.ImageFillContain
	c.icon.SetMinSize(fyne.NewSize(50, 50)) // Fixed Icon Size 50px

	c.label.Alignment = fyne.TextAlignCenter
	c.label.Truncation = fyne.TextTruncateEllipsis
	// Note: widget.Label TextSize is standard, hard to force 11 without custom theme or extending.
	// We'll stick to standard size which is readable.

	c.ExtendBaseWidget(c)
	return c
}

func (c *FileCard) CreateRenderer() fyne.WidgetRenderer {
	// Layout: Icon Top, Label Bottom
	content := container.NewVBox(
		container.NewCenter(c.icon),
		layout.NewSpacer(),
		c.label,
	)

	// Stack: BG (Bottom), Content (Top)
	stack := container.NewStack(c.bg, container.NewPadded(content))
	return widget.NewSimpleRenderer(stack)
}

func (c *FileCard) SetData(name, fullPath string, isDir bool, isSelected bool) {
	c.FullPath = fullPath
	c.Name = name
	c.IsDir = isDir

	// TRUNCATE TEXT - Handled by widget.Label
	c.label.SetText(name)

	// ICON
	// For now using Standard Theme Icons converted to Resource
	if isDir {
		c.icon.Resource = theme.FolderIcon()
	} else {
		c.icon.Resource = theme.FileIcon()
	}

	// SELECTION STATE
	if isSelected {
		c.bg.FillColor = theme.PrimaryColor()
		// widget.Label handles color automatically usually, or we can't easily force it white without custom renderer/theme
		// For now, let it be default or implied.
	} else {
		c.bg.FillColor = color.Transparent
	}

	c.bg.Refresh()
	c.icon.Refresh() // Important when changing resource
	c.label.Refresh()
	c.Show()
}

// MouseIn - Hover Effect
func (c *FileCard) MouseIn(*desktop.MouseEvent) {
	if c.bg.FillColor == theme.PrimaryColor() {
		return // Don't override selection
	}
	// Rounded White Overlay (25 value)
	c.bg.FillColor = color.RGBA{255, 255, 255, 25}
	c.bg.Refresh()

	if c.OnHover != nil {
		c.OnHover(c.Name)
	}
}

// MouseOut - Remove Hover
func (c *FileCard) MouseOut() {
	if c.bg.FillColor == theme.PrimaryColor() {
		return
	}
	c.bg.FillColor = color.Transparent
	c.bg.Refresh()
}

// Tapped
func (c *FileCard) Tapped(_ *fyne.PointEvent) {
	if c.OnTapped != nil {
		c.OnTapped(c.FullPath, c.IsDir)
	}
}

func NewFileGrid() *FileGrid {
	f := &FileGrid{
		Columns: 5, // Fixed column count
		Items:   []fs.DirEntry{},
	}

	f.Table = widget.NewTable(
		// Length
		func() (int, int) {
			rows := int(math.Ceil(float64(len(f.Items)) / float64(f.Columns)))
			return rows, f.Columns
		},
		// Create
		func() fyne.CanvasObject {
			// Container wrapper to give padding/spacing between cells
			card := NewFileCard()
			return container.NewPadded(card)
		},
		// Update
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			padded := obj.(*fyne.Container)
			card := padded.Objects[0].(*FileCard)

			index := (id.Row * f.Columns) + id.Col

			if index >= len(f.Items) {
				card.Hide() // Out of bounds
				return
			}

			entry := f.Items[index]
			name := entry.Name()
			fullPath := f.CurrentDir + string(os.PathSeparator) + name
			isSelected := f.Selected == fullPath

			card.SetData(name, fullPath, entry.IsDir(), isSelected)

			// Interaction
			card.OnTapped = func(path string, isDir bool) {
				if isDir {
					if f.OnNavigate != nil {
						f.OnNavigate(path)
					}
				} else {
					f.Selected = path
					f.Table.Refresh() // Redraw to update selection visually
					if f.OnSelect != nil {
						f.OnSelect(path)
					}
				}
			}
			card.OnHover = func(name string) {
				if f.OnHover != nil {
					f.OnHover(name)
				}
			}
		},
	)

	// Table Styling
	f.Table.ShowHeaderRow = false
	f.Table.ShowHeaderColumn = false

	// WIDER COLUMNS: Set explicit width to allow more text visibility
	for i := 0; i < f.Columns; i++ {
		f.Table.SetColumnWidth(i, 110)
	}

	return f
}

func (f *FileGrid) SetItems(items []fs.DirEntry, dir string) {
	f.Items = items
	f.CurrentDir = dir
	f.Selected = ""
	f.Table.Refresh()
	f.Table.ScrollToTop()
}
