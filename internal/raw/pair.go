package raw

import (
	"cullsnap/internal/logger"
	"cullsnap/internal/model"
	"path/filepath"
	"strings"
)

var jpegExtensions = map[string]bool{
	".jpg": true, ".jpeg": true,
}

// PairRAWJPEG identifies RAW+JPEG companion pairs in a photo list.
// Same base filename + same directory = pair. Case-insensitive.
// RAW is primary, JPEG is marked as companion.
func PairRAWJPEG(photos []model.Photo) []model.Photo {
	type fileKey struct {
		dir  string
		base string // lowercase, no extension
	}

	rawFiles := make(map[fileKey]int)
	jpegFiles := make(map[fileKey]int)

	for i := range photos {
		ext := strings.ToLower(filepath.Ext(photos[i].Path))
		dir := filepath.Dir(photos[i].Path)
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(photos[i].Path), filepath.Ext(photos[i].Path)))
		key := fileKey{dir: dir, base: base}

		if IsRAWExt(ext) {
			rawFiles[key] = i
		} else if jpegExtensions[ext] {
			jpegFiles[key] = i
		}
	}

	pairCount := 0
	for key, rawIdx := range rawFiles {
		jpegIdx, hasCompanion := jpegFiles[key]
		if !hasCompanion {
			continue
		}

		photos[rawIdx].CompanionPath = photos[jpegIdx].Path
		photos[jpegIdx].CompanionPath = photos[rawIdx].Path
		photos[jpegIdx].IsRAWCompanion = true

		pairCount++
		logger.Log.Debug("raw: paired RAW+JPEG", "raw", photos[rawIdx].Path, "jpeg", photos[jpegIdx].Path)
	}

	unpaired := len(rawFiles) - pairCount
	logger.Log.Info("raw: pairing complete",
		"totalFiles", len(photos),
		"pairs", pairCount,
		"unpairedRAW", unpaired,
		"unpairedJPEG", len(jpegFiles)-pairCount,
	)

	return photos
}
