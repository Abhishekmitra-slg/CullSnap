package scanner

import (
	"os"
	"path/filepath"
	"strings"

	"cullsnap/internal/model"
)

var allowedExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	// Add RAW formats later if needed, starting with basic support
}

// ScanDirectory worker pool implementation could go here,
// but for the initial list population, we just need to get the file paths fast.
// The expensive operation is thumbnail generation, which happens in the UI layer (virtualized).
// So this scanner just finds files quickly.
func ScanDirectory(root string) ([]model.Photo, error) {
	var photos []model.Photo

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if allowedExtensions[ext] {
			info, err := d.Info()
			if err != nil {
				return nil // Skip if can't get info
			}

			p := model.Photo{
				Path:    path,
				Size:    info.Size(),
				TakenAt: info.ModTime(), // Fallback to ModTime, ideally parse EXIF later
			}

			photos = append(photos, p)
		}
		return nil
	})

	return photos, err
}
