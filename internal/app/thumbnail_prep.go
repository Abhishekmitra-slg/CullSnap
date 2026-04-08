package app

import (
	"cullsnap/internal/model"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"time"
)

// thumbnailItem represents a file to be thumbnailed, with its mod time for cache keying.
type thumbnailItem struct {
	Path    string
	ModTime time.Time
}

// buildThumbnailItems converts a slice of Photos to thumbnailItems for GenerateBatch.
func buildThumbnailItems(photos []model.Photo) []thumbnailItem {
	items := make([]thumbnailItem, len(photos))
	for i := range photos {
		items[i] = thumbnailItem{Path: photos[i].Path, ModTime: photos[i].TakenAt}
	}
	return items
}

// detectHEICInfo counts HEIC/HEIF files and determines the decoder to use.
// Returns heicCount and heicDecoder ("sips", "ffmpeg", or "" if no HEIC files).
func detectHEICInfo(items []thumbnailItem, useNativeSips bool) (int, string) {
	heicCount := 0
	for _, item := range items {
		ext := strings.ToLower(filepath.Ext(item.Path))
		if ext == ".heic" || ext == ".heif" {
			heicCount++
		}
	}
	if heicCount == 0 {
		return 0, ""
	}
	if useNativeSips && stdruntime.GOOS == "darwin" {
		return heicCount, "sips"
	}
	return heicCount, "ffmpeg"
}

// applyThumbnailPaths populates ThumbnailPath on photos from a map of original→thumbnail paths.
func applyThumbnailPaths(photos []model.Photo, thumbnailMap map[string]string) {
	for i := range photos {
		if tp, ok := thumbnailMap[photos[i].Path]; ok {
			photos[i].ThumbnailPath = tp
		}
	}
}
