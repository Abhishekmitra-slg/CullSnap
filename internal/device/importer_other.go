//go:build !darwin

package device

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// SanitizeSerial strips unsafe characters from a device serial number.
func SanitizeSerial(serial string) string {
	if serial == "" {
		return "_"
	}
	cleaned := sanitizeRe.ReplaceAllString(serial, "_")
	return filepath.Base(cleaned)
}

// ImportFromDevice is not available on non-macOS platforms.
func ImportFromDevice(_ context.Context, _, _ string) (string, int, error) {
	return "", 0, fmt.Errorf("device import is only available on macOS")
}
