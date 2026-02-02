package picker

import (
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ShowFinder opens the custom finder dialog
func ShowFinder(parent fyne.Window, title string, onOpen func(path string)) {
	// State
	var currentPath string
	home, _ := os.UserHomeDir()
	currentPath = home

	// Components
	fileGrid := NewFileGrid()

	// Top Bar
	pathInput := widget.NewEntry()
	pathInput.SetText(currentPath)

	// Navigation State
	var history []string
	isNavigating := false

	// Define loadDir variable first to use in closure
	var loadDir func(path string)

	loadDir = func(path string) {
		// 1. Push to history if not navigating back
		if !isNavigating && currentPath != "" && currentPath != path {
			history = append(history, currentPath)
		}

		// 2. Read Directory
		files, err := os.ReadDir(path)
		if err != nil {
			dialog.ShowError(err, parent)
			return
		}

		currentPath = path

		// Filter hidden/system files
		var filtered []fs.DirEntry
		for _, f := range files {
			name := f.Name()
			if len(name) > 0 {
				if name[0] == '.' || name[0] == '$' {
					continue
				}
				if name == "Desktop.ini" || name == "Thumbs.db" || name == "__MACOSX" {
					continue
				}
			}
			filtered = append(filtered, f)
		}

		// Sort: Folders first, then files
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].IsDir() && !filtered[j].IsDir()
		})

		fileGrid.SetItems(filtered, path)
		pathInput.SetText(path)
	}

	// Top Bar Actions
	refreshBtn := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), func() {
		loadDir(currentPath)
	})

	// BACK BUTTON (History Pop)
	upBtn := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() {
		if len(history) > 0 {
			// Pop
			last := history[len(history)-1]
			history = history[:len(history)-1]

			isNavigating = true
			loadDir(last)
			isNavigating = false
		} else {
			// Fallback: Go to Parent if no history?
			// User requested "True Back". If stack empty, maybe disable or do nothing.
			// Let's implement Parent logic on a separate button or just keep it strict.
			// Ideally, we might want to go up if history is empty, but "Back" implies "Where I was".
			// Let's just do nothing if history is empty.
			// OR: The user mentioned "Add a separate 'Up' button". I'll stick to history strictness for "Back".
		}
	})
	// Optional: Add Up Button
	parentBtn := widget.NewButtonWithIcon("", theme.MoveUpIcon(), func() {
		parentDir := filepath.Dir(currentPath)
		loadDir(parentDir)
	})

	newFolderBtn := widget.NewButtonWithIcon("", theme.FolderNewIcon(), func() {
		entry := widget.NewEntry()
		entry.SetPlaceHolder("New Folder Name")
		dialog.ShowCustomConfirm("New Folder", "Create", "Cancel", entry, func(ok bool) {
			if ok && entry.Text != "" {
				targetDir := fileGrid.CurrentDir
				newPath := filepath.Join(targetDir, entry.Text)
				err := os.Mkdir(newPath, 0755)
				if err != nil {
					dialog.ShowError(err, parent)
				} else {
					loadDir(targetDir)
				}
			}
		}, parent)
	})

	topBar := container.NewBorder(nil, nil,
		container.NewHBox(upBtn, parentBtn, refreshBtn, newFolderBtn),
		nil,
		container.NewPadded(pathInput), // Padded address bar
	)

	// Bottom Bar
	openBtn := widget.NewButton("Open", func() {
		onOpen(currentPath)
	})
	openBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton("Cancel", func() {
		// We'll close the dialog via the popup handle logic below
	})

	statusLabel := widget.NewLabel("")
	statusLabel.TextStyle = fyne.TextStyle{Italic: true}
	statusLabel.Truncation = fyne.TextTruncateEllipsis

	bottomBar := container.NewHBox(statusLabel, layout.NewSpacer(), cancelBtn, openBtn)

	// Sidebar
	sidebarList := widget.NewList(
		func() int { return len(GetSidebarItems()) },
		func() fyne.CanvasObject {
			icon := widget.NewIcon(theme.FolderIcon())
			label := widget.NewLabel("Label")
			label.TextStyle = fyne.TextStyle{}

			headerLabel := canvas.NewText("HEADER", color.RGBA{136, 136, 136, 255})
			headerLabel.TextStyle = fyne.TextStyle{Bold: true}
			headerLabel.TextSize = 10

			return container.NewStack(
				container.NewPadded(container.NewHBox(icon, label)),
				container.NewPadded(headerLabel),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			item := GetSidebarItems()[id]
			stack := obj.(*fyne.Container)
			itemCont := stack.Objects[0].(*fyne.Container)
			hbox := itemCont.Objects[0].(*fyne.Container)
			headerCont := stack.Objects[1].(*fyne.Container)
			icon := hbox.Objects[0].(*widget.Icon)
			label := hbox.Objects[1].(*widget.Label)
			headerLabel := headerCont.Objects[0].(*canvas.Text)

			if item.IsHeader {
				itemCont.Hide()
				headerCont.Show()
				headerLabel.Text = item.Label
				headerLabel.Refresh()
			} else {
				headerCont.Hide()
				itemCont.Show()
				label.SetText(item.Label)
				icon.SetResource(item.Icon)
			}
		},
	)

	var d *widget.PopUp

	sidebarList.OnSelected = func(id widget.ListItemID) {
		item := GetSidebarItems()[id]
		sidebarList.Unselect(id)

		if item.IsHeader {
			return
		}

		if item.IsAction {
			entry := widget.NewEntry()
			entry.SetPlaceHolder("smb://... or /Path/To/Share")
			dialog.ShowCustomConfirm("Connect to Server", "Connect", "Cancel", entry, func(ok bool) {
				if ok && entry.Text != "" {
					loadDir(entry.Text)
				}
			}, parent)
		} else {
			loadDir(item.Path)
		}
	}

	// Layout Construction
	sidebarBg := canvas.NewRectangle(color.RGBA{42, 42, 42, 255})
	sidebarContent := container.NewStack(sidebarBg, sidebarList)
	sidebarContainer := container.NewHSplit(sidebarContent, container.NewStack(
		canvas.NewRectangle(color.RGBA{30, 30, 30, 255}),
		container.NewBorder(topBar, nil, nil, nil, fileGrid.Table),
	))
	sidebarContainer.SetOffset(0.25)

	mainContent := container.NewBorder(nil, container.NewPadded(bottomBar), nil, nil, sidebarContainer)
	mainContentWithSize := container.NewStack(
		canvas.NewRectangle(color.Transparent),
		mainContent,
	)
	mainContentWithSize.Resize(fyne.NewSize(800, 600))

	d = widget.NewModalPopUp(mainContentWithSize, parent.Canvas())

	cancelBtn.OnTapped = func() {
		d.Hide()
	}
	openBtn.OnTapped = func() {
		d.Hide()
		onOpen(currentPath)
	}

	fileGrid.OnNavigate = func(path string) {
		loadDir(path)
	}

	fileGrid.OnHover = func(name string) {
		statusLabel.SetText(name)
	}

	// Initial Load
	loadDir(currentPath)

	d.Show()
	d.Resize(fyne.NewSize(900, 600))
}
