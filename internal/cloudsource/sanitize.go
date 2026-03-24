package cloudsource

import (
	"path/filepath"
	"regexp"
)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// SanitizeID makes a cloud-provided ID safe for use as a filesystem name.
// Strips non-alphanumeric characters and prevents directory traversal.
func SanitizeID(id string) string {
	if id == "" {
		return "_"
	}
	sanitized := sanitizeRe.ReplaceAllString(id, "_")
	return filepath.Base(sanitized)
}
