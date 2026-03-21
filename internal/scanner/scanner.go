package scanner

import (
	"cullsnap/internal/model"
	"cullsnap/internal/raw"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	for ext := range raw.Extensions {
		allowedExtensions[ext] = true
	}
}

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

// ScanDirectory walks root and returns Photo structs for supported files.
// This is intentionally fast — it only reads file metadata (no thumbnails, no ffprobe).
// Thumbnail generation and video duration enrichment happen in app.go after scan completes.
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

			p := model.Photo{
				Path:    path,
				Size:    info.Size(),
				TakenAt: info.ModTime(),
				IsVideo: isVideo,
				// Duration intentionally 0 — enriched asynchronously by app.go
			}

			if raw.IsRAWExt(ext) {
				p.IsRAW = true
				p.RAWFormat = raw.FormatName(ext)
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
