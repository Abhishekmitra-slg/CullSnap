package hfclient

import (
	"fmt"
	"path/filepath"
	"strings"
)

// validateSiblingPath rejects HF tree paths that could escape the destination
// directory or contain illegal characters. Applied to every sibling path
// returned by the HF tree API and again before every file open.
func validateSiblingPath(p string) error {
	if p == "" {
		return fmt.Errorf("hfclient: bad path: empty")
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return fmt.Errorf("hfclient: bad path %q: leading separator", p)
	}
	if strings.ContainsAny(p, "\x00\\:") {
		return fmt.Errorf("hfclient: bad path %q: forbidden char", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("hfclient: bad path %q: parent traversal segment", p)
		}
	}
	if filepath.IsAbs(p) {
		return fmt.Errorf("hfclient: bad path %q: absolute", p)
	}
	if filepath.Clean(p) != p {
		return fmt.Errorf("hfclient: bad path %q: not in canonical form", p)
	}
	return nil
}
