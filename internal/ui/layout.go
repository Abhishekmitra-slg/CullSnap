package ui

import (
	"cullsnap/internal/export"
	imageUtils "cullsnap/internal/image"
	"cullsnap/internal/logger"
	"cullsnap/internal/model"
	"cullsnap/internal/scanner"
	"cullsnap/internal/storage"
	"fmt"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// SetupMainLayout constructs the primary UI structure.
func SetupMainLayout(w fyne.Window, store *storage.SQLiteStore) fyne.CanvasObject {
	// State
	var currentPhotos []model.Photo
	// Track current directory for session/storage context
	var currentPath string
	var activePhotoIndex int = -1

	// Components
	grid := NewThumbnailGrid()
	loadingBar := widget.NewProgressBarInfinite()
	loadingBar.Hide()
	pathLabel := widget.NewLabelWithStyle("No Folder Opened", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	statusLabel := widget.NewLabel("Session: None | Photos: 0 | Selected: 0")
	statusLabel.TextStyle.Monospace = true

	// Helper to update status bar
	updateStatus := func() {
		sessionName := "None"
		if currentPath != "" {
			sessionName = filepath.Base(currentPath)
		}
		selCount := len(grid.SelectedPhotos)
		totalCount := len(currentPhotos)
		statusLabel.SetText(fmt.Sprintf("Session: %s | Photos: %d | Selected: %d", sessionName, totalCount, selCount))
	}

	// Right: Viewer
	viewerRect := canvas.NewRectangle(theme.InputBackgroundColor())
	viewerLabel := widget.NewLabel("No Image Selected")
	viewerImg := canvas.NewImageFromResource(nil)
	viewerImg.FillMode = canvas.ImageFillContain
	viewer := container.NewStack(viewerRect, viewerImg, container.NewCenter(viewerLabel))

	grid.OnPhotoSelected = func(photo model.Photo) {
		// Load full image for viewer
		viewerLabel.SetText("Loading " + photo.Path + "...")
		viewerLabel.Hidden = false
		viewerImg.Image = nil // Clear previous image
		viewerImg.Refresh()

		go func() {
			img, err := imageUtils.GetFullImage(photo.Path)
			// Ensure UI updates are on main thread
			fyne.CurrentApp().Driver().DoFromGoroutine(func() {
				if err == nil {
					viewerImg.Image = img
					viewerImg.Refresh()
					viewerLabel.Hidden = true
					logger.Log.Info("Loaded full image", "path", photo.Path)
				} else {
					viewerLabel.SetText("Error: " + err.Error())
					viewerLabel.Hidden = false
					logger.Log.Error("Failed to load image", "path", photo.Path, "error", err)
				}
			}, false)
		}()

		// Update active index
		for i, p := range currentPhotos {
			if p.Path == photo.Path {
				activePhotoIndex = i
				break
			}
		}
	}

	// Helper to load directory
	loadDirectory := func(path string) {
		loadingBar.Show()
		go func() {
			// Add to recents
			if err := store.AddRecent(path); err != nil {
				logger.Log.Error("Failed to add recent", "path", path, "error", err)
			}

			// 1. Scan Photos
			photos, err := scanner.ScanDirectory(path)

			// 2. Load Persistence Data
			var selections map[string]bool
			var exported map[string]bool

			if err == nil {
				// Use directory path as Session ID
				selections, _ = store.GetSelections(path)
				if selections == nil {
					selections = make(map[string]bool)
				}

				exported, _ = store.GetExportedInDirectory(path)
				if exported == nil {
					exported = make(map[string]bool)
				}
			}

			fyne.CurrentApp().Driver().DoFromGoroutine(func() {
				loadingBar.Hide()
				if err != nil {
					dialog.ShowError(err, w)
					logger.Log.Error("Scan failed", "path", path, "error", err)
					return
				}

				currentPath = path
				pathLabel.SetText(path)
				currentPhotos = photos
				grid.SetPhotos(photos)

				// Restore state
				grid.SelectedPhotos = selections
				grid.ExportedPhotos = exported

				activePhotoIndex = -1
				grid.List.Refresh()
				updateStatus() // Update Status
				logger.Log.Info("Directory loaded", "path", path, "count", len(photos))
			}, false)
		}()
	}

	// Keyboard Shortcuts: Shift+S
	w.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if ev.Name == fyne.KeyS {
			if activePhotoIndex != -1 && activePhotoIndex < len(currentPhotos) {
				path := currentPhotos[activePhotoIndex].Path

				// Toggle selection using Grid's map
				isSelected := !grid.SelectedPhotos[path]

				if isSelected {
					grid.SelectedPhotos[path] = true
				} else {
					delete(grid.SelectedPhotos, path)
				}
				grid.List.Refresh()
				updateStatus() // Update Status

				// Persist
				go func(p, dir string, sel bool) {
					if err := store.SaveSelection(p, dir, sel); err != nil {
						logger.Log.Error("Failed to save selection", "path", p, "error", err)
					}
				}(path, currentPath, isSelected)
			}
		}
	})

	// Top Toolbar
	toolbar := widget.NewToolbar(
		widget.NewToolbarAction(theme.FolderOpenIcon(), func() {
			dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
				if err != nil || uri == nil {
					return
				}
				loadDirectory(uri.Path())
			}, w)
		}),
		widget.NewToolbarAction(theme.ComputerIcon(), func() {
			entry := widget.NewEntry()
			entry.SetPlaceHolder("Enter full path (e.g. /Volumes/Share/Photos)")
			dialog.ShowCustomConfirm("Enter Path", "Load", "Cancel", entry, func(confirm bool) {
				if confirm && entry.Text != "" {
					loadDirectory(entry.Text)
				}
			}, w)
		}),
		widget.NewToolbarAction(theme.HistoryIcon(), func() {
			// Recent Folders
			go func() {
				recents, err := store.GetRecents()
				fyne.CurrentApp().Driver().DoFromGoroutine(func() {
					if err != nil {
						dialog.ShowError(err, w)
						return
					}
					if len(recents) == 0 {
						dialog.ShowInformation("Recents", "No recent folders.", w)
						return
					}

					// List widget for recents
					list := widget.NewList(
						func() int { return len(recents) },
						func() fyne.CanvasObject { return widget.NewButton("Template Path", nil) },
						func(id widget.ListItemID, obj fyne.CanvasObject) {
							path := recents[id]
							btn := obj.(*widget.Button)
							btn.SetText(path)
						},
					)

					// Fix: Use `dialog.NewCustom` to get handle.
					var d dialog.Dialog

					// Re-define list update to use `d`
					list.UpdateItem = func(id widget.ListItemID, obj fyne.CanvasObject) {
						path := recents[id]
						btn := obj.(*widget.Button)
						btn.SetText(path)
						btn.OnTapped = func() {
							d.Hide()
							loadDirectory(path)
						}
					}

					// Limit height
					scroll := container.NewVScroll(list)
					scroll.SetMinSize(fyne.NewSize(400, 300))

					d = dialog.NewCustom("Recent Folders", "Close", scroll, w)
					d.Show()
				}, false)
			}()
		}),
		widget.NewToolbarSpacer(),

		widget.NewToolbarAction(theme.DocumentPrintIcon(), func() {
			// Export Action
			var photosToExport []model.Photo
			for _, photo := range currentPhotos {
				if grid.SelectedPhotos[photo.Path] {
					photosToExport = append(photosToExport, photo)
				}
			}

			if len(photosToExport) == 0 {
				dialog.ShowInformation("Export", "No photos selected for export.", w)
				return
			}

			// Helper to execute export
			doExport := func(dest string) {
				timestamp := time.Now().Format("20060102_150405")
				finalDest := filepath.Join(dest, "Session_"+timestamp)

				go func() {
					count, err := export.ExportSelections(photosToExport, finalDest)

					// Mark exported in DB
					if err == nil {
						for _, p := range photosToExport {
							store.MarkExported(p.Path)
						}
					}

					fyne.CurrentApp().Driver().DoFromGoroutine(func() {
						if err != nil {
							dialog.ShowError(err, w)
							logger.Log.Error("Export failed", "error", err)
						} else {
							// Update Interface State
							for _, p := range photosToExport {
								grid.ExportedPhotos[p.Path] = true
							}
							grid.List.Refresh()
							updateStatus() // Update status

							dialog.ShowInformation("Export Complete", fmt.Sprintf("Exported %d photos to %s", count, finalDest), w)
							logger.Log.Info("Export success", "count", count, "dest", finalDest)
						}
					}, false)
				}()
			}

			// Export Dialog
			var exportDialog dialog.Dialog
			browseBtn := widget.NewButtonWithIcon("Browse Folder", theme.FolderOpenIcon(), func() {
				exportDialog.Hide()
				dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
					if err == nil && uri != nil {
						doExport(uri.Path())
					}
				}, w)
			})
			manualBtn := widget.NewButtonWithIcon("Enter Path", theme.ComputerIcon(), func() {
				exportDialog.Hide()
				entry := widget.NewEntry()
				entry.SetPlaceHolder("/full/path/to/destination")
				dialog.ShowCustomConfirm("Export Path", "Export", "Cancel", entry, func(ok bool) {
					if ok && entry.Text != "" {
						doExport(entry.Text)
					}
				}, w)
			})

			content := container.NewVBox(
				widget.NewLabel(fmt.Sprintf("Exporting %d photos.", len(photosToExport))),
				widget.NewLabel("Choose destination method:"),
				browseBtn,
				manualBtn,
			)
			exportDialog = dialog.NewCustom("Export Selection", "Cancel", content, w)
			exportDialog.Show()
		}),
		widget.NewToolbarAction(theme.FileTextIcon(), func() {
			if err := logger.OpenLogFile(); err != nil {
				dialog.ShowError(err, w)
			}
		}),
		widget.NewToolbarAction(theme.HelpIcon(), func() {
			helpText := `
## Controls
- **Folder Icon**: Open directory to cull.
- **Computer Icon**: Enter directory path manually.
- **History Icon**: Open list of recent folders.
- **Left Panel**: Thumbnails (Blue = Selected, Green Badge = Selected).
- **Right Panel**: High-res viewer.

## Shortcuts
- **S** or **Shift+S**: Toggle Selection of current photo.

## Export
- Click **Print Icon** to export selected photos.
- App adds a date-stamped folder automatically.
			`
			dialog.ShowCustom("Help", "Close", widget.NewRichTextFromMarkdown(helpText), w)
		}),
	)

	// Center: SplitContainer
	split := container.NewHSplit(grid.List, viewer)
	split.SetOffset(0.3) // Give 30% to thumbnails

	// Main Layout: BorderLayout
	topContainer := container.NewVBox(toolbar, pathLabel)
	bottomContainer := container.NewVBox(loadingBar, statusLabel) // Status bar at bottom

	content := container.New(layout.NewBorderLayout(topContainer, bottomContainer, nil, nil),
		topContainer,
		bottomContainer,
		split,
	)

	return content
}
