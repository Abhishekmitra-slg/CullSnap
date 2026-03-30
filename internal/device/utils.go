package device

import (
	"os"
	"path/filepath"
	"regexp"
)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// SanitizeSerial strips unsafe characters from a device serial number,
// returning a safe directory name component. Path traversal sequences
// like "../" are neutralized by replacing special chars and calling
// filepath.Base.
func SanitizeSerial(serial string) string {
	if serial == "" {
		return "_"
	}
	cleaned := sanitizeRe.ReplaceAllString(serial, "_")
	return filepath.Base(cleaned)
}

// countFiles returns the number of non-directory entries in dir.
// Returns 0 if the directory does not exist or cannot be read.
func countFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			count++
		}
	}
	return count
}
