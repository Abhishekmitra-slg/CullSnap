package picker

import (
	"os"
	"path/filepath"
	"runtime"

	"fyne.io/fyne/v2/theme"
)

// GetSidebarItems discovers system paths and returns categorized sidebar items.
func GetSidebarItems() []SidebarItem {
	var items []SidebarItem

	// 1. FAVORITES
	items = append(items, SidebarItem{Label: "FAVORITES", IsHeader: true, Section: SectionFavorites})
	homeDir, err := os.UserHomeDir()
	if err == nil {
		items = append(items, SidebarItem{
			Label:   "Home",
			Icon:    theme.HomeIcon(),
			Path:    homeDir,
			Section: SectionFavorites,
		})
		items = append(items, SidebarItem{
			Label:   "Desktop",
			Icon:    theme.ComputerIcon(), // Fallback icon
			Path:    filepath.Join(homeDir, "Desktop"),
			Section: SectionFavorites,
		})
		items = append(items, SidebarItem{
			Label:   "Documents",
			Icon:    theme.DocumentIcon(),
			Path:    filepath.Join(homeDir, "Documents"),
			Section: SectionFavorites,
		})
		items = append(items, SidebarItem{
			Label:   "Pictures",
			Icon:    theme.MediaPhotoIcon(), // Better suited for photos app
			Path:    filepath.Join(homeDir, "Pictures"),
			Section: SectionFavorites,
		})
		items = append(items, SidebarItem{
			Label:   "Downloads",
			Icon:    theme.DownloadIcon(),
			Path:    filepath.Join(homeDir, "Downloads"),
			Section: SectionFavorites,
		})
	}

	// 2. LOCATIONS (Drives)
	items = append(items, SidebarItem{Label: "LOCATIONS", IsHeader: true, Section: SectionLocations})
	if runtime.GOOS == "windows" {
		// Simple drive letter scan
		for _, drive := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
			path := string(drive) + ":\\"
			if _, err := os.Stat(path); err == nil {
				items = append(items, SidebarItem{
					Label:   string(drive) + ":",
					Icon:    theme.StorageIcon(),
					Path:    path,
					Section: SectionLocations,
				})
			}
		}
	} else if runtime.GOOS == "darwin" {
		// Mac /Volumes
		items = append(items, SidebarItem{
			Label:   "Macintosh HD", // Simplification: Root
			Icon:    theme.StorageIcon(),
			Path:    "/",
			Section: SectionLocations,
		})

		entries, err := os.ReadDir("/Volumes")
		if err == nil {
			for _, e := range entries {
				// Skip hidden or symlinks if needed
				items = append(items, SidebarItem{
					Label:   e.Name(),
					Icon:    theme.StorageIcon(),
					Path:    filepath.Join("/Volumes", e.Name()),
					Section: SectionLocations,
				})
			}
		}
	} else {
		// Linux /mnt or /media
		items = append(items, SidebarItem{
			Label:   "Root",
			Icon:    theme.StorageIcon(),
			Path:    "/",
			Section: SectionLocations,
		})
		// Simple check for /mnt
		if entries, err := os.ReadDir("/mnt"); err == nil {
			for _, e := range entries {
				items = append(items, SidebarItem{
					Label:   e.Name(),
					Icon:    theme.StorageIcon(),
					Path:    filepath.Join("/mnt", e.Name()),
					Section: SectionLocations,
				})
			}
		}
	}

	// 3. NETWORK
	items = append(items, SidebarItem{Label: "NETWORK", IsHeader: true, Section: SectionNetwork})
	items = append(items, SidebarItem{
		Label:    "Connect with manual path...",
		Icon:     theme.ComputerIcon(), // Network Globe fallback
		Path:     "",
		Section:  SectionNetwork,
		IsAction: true,
	})

	return items
}
