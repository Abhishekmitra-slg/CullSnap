package scanner

import (
	"os"
	"path/filepath"
	"strings"

	"cullsnap/internal/model"
	"cullsnap/internal/video"
)

var allowedExtensions = map[string]bool{
	// Photos
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	// Videos
	".mp4":  true,
	".mov":  true,
	".webm": true,
	".mkv":  true,
	".avi":  true,
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
			if d.Name() == "duplicates" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if allowedExtensions[ext] {
			info, err := d.Info()
			if err != nil {
				return nil // Skip if can't get info
			}
			
			isVideo := isVideoExt(ext)
			var duration float64
			if isVideo {
				// Using the previously built ffmpeg module to extract duration quickly
				dur, err := video.GetDuration(path)
				if err == nil {
					duration = dur
				}
			}

			p := model.Photo{
				Path:     path,
				Size:     info.Size(),
				TakenAt:  info.ModTime(), // Fallback to ModTime, ideally parse EXIF later
				IsVideo:  isVideo,
				Duration: duration,
			}

			photos = append(photos, p)
		}
		return nil
	})

	return photos, err
}

func isVideoExt(ext string) bool {
	switch ext {
	case ".mp4", ".mov", ".webm", ".mkv", ".avi":
		return true
	}
	return false
}
