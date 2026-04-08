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
	".heic": true,
	".heif": true,
	// Videos
	".mp4":  true,
	".mov":  true,
	".webm": true,
	".mkv":  true,
	".avi":  true,
}

// ScanDirectoryStream walks root and calls batchFn with batches of photos.
// batchFn is called every batchSize files with done=false, and once more at the end
// with done=true (which may carry a non-empty final partial batch, or an empty batch
// for empty directories). Each batch is a fresh copy — safe to store without cloning.
func ScanDirectoryStream(root string, batchSize int, batchFn func(batch []model.Photo, done bool)) error {
	if batchSize <= 0 {
		batchSize = 50
	}

	var batch []model.Photo

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
		if !allowedExtensions[ext] {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil // Skip if can't get info
		}

		p := model.Photo{
			Path:    path,
			Size:    info.Size(),
			TakenAt: info.ModTime(),
			IsVideo: isVideoExt(ext),
		}

		if raw.IsRAWExt(ext) {
			p.IsRAW = true
			p.RAWFormat = raw.FormatName(ext)
		}

		batch = append(batch, p)

		if len(batch) >= batchSize {
			out := make([]model.Photo, len(batch))
			copy(out, batch)
			batchFn(out, false)
			batch = batch[:0]
		}

		return nil
	})

	// Final batch (may be empty for empty dirs) — copy for safety
	out := make([]model.Photo, len(batch))
	copy(out, batch)
	batchFn(out, true)

	return err
}

// ScanDirectory walks root and returns Photo structs for supported files.
// This is intentionally fast — it only reads file metadata (no thumbnails, no ffprobe).
// Thumbnail generation and video duration enrichment happen in app.go after scan completes.
func ScanDirectory(root string) ([]model.Photo, error) {
	var photos []model.Photo
	err := ScanDirectoryStream(root, 10000, func(batch []model.Photo, done bool) {
		photos = append(photos, batch...)
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
