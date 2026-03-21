package raw

import "strings"

// Extensions is the single source of truth for all supported RAW file extensions.
var Extensions = map[string]bool{
	".cr2": true, ".cr3": true, ".arw": true, ".nef": true, ".dng": true,
	".raf": true, ".rw2": true, ".orf": true, ".nrw": true, ".pef": true, ".srw": true,
}

// IsRAWExt returns true if the given extension (with leading dot) is a supported RAW format.
func IsRAWExt(ext string) bool {
	return Extensions[strings.ToLower(ext)]
}

// FormatName returns the uppercase format name for display (e.g., ".cr3" -> "CR3").
func FormatName(ext string) string {
	return strings.ToUpper(strings.TrimPrefix(strings.ToLower(ext), "."))
}

// ImageExtensions returns all image extensions (JPEG + PNG + RAW) for use in file filters.
func ImageExtensions() map[string]bool {
	exts := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true,
	}
	for ext := range Extensions {
		exts[ext] = true
	}
	return exts
}
