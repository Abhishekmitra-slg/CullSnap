package device

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var allowedBinDirs = []string{
	"/usr/bin/",
	"/usr/local/bin/",
	"/usr/sbin/",
	"/bin/",
	"/sbin/",
	"/opt/homebrew/bin/",
}

func resolveSecureBinary(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found: %w", name, err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("cannot resolve absolute path for %s: %w", name, err)
	}

	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("cannot resolve symlinks for %s: %w", name, err)
	}

	for _, prefix := range allowedBinDirs {
		if strings.HasPrefix(realPath, prefix) {
			return realPath, nil
		}
	}

	return "", fmt.Errorf("%s found at %s which is not in a trusted directory", name, realPath)
}

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

// validateDestDir ensures the destination directory is under the cache directory.
// Prevents path traversal attacks from malicious serial numbers or crafted paths.
func validateDestDir(destDir, cacheDir string) error {
	absDir, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("invalid destination path: %w", err)
	}
	absCacheDir, err := filepath.Abs(cacheDir)
	if err != nil {
		return fmt.Errorf("invalid cache path: %w", err)
	}
	// Must be strictly under cacheDir (not equal to it).
	if !strings.HasPrefix(absDir, absCacheDir+string(filepath.Separator)) {
		return fmt.Errorf("destination must be under cache directory")
	}
	return nil
}

// verifyNoPathTraversal walks the import directory and removes any files
// whose resolved path escapes the expected root. Returns count of removed files.
func verifyNoPathTraversal(rootDir string) int {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return 0
	}
	// Resolve symlinks in the root itself so comparisons work on systems
	// where temp dirs are symlinked (e.g., macOS /var -> /private/var).
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return 0
	}
	removed := 0
	_ = filepath.Walk(absRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == absRoot {
			return nil
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			_ = os.Remove(path) //nolint:gosec // G122: intentional security cleanup — removing unresolvable files from import dir
			removed++
			return nil
		}
		absResolved, err := filepath.Abs(resolved)
		if err != nil {
			_ = os.Remove(path) //nolint:gosec // G122: intentional security cleanup — removing unresolvable files from import dir
			removed++
			return nil
		}
		if !strings.HasPrefix(absResolved, resolvedRoot+string(filepath.Separator)) && absResolved != resolvedRoot {
			_ = os.Remove(path) //nolint:gosec // G122: intentional security cleanup — removing escaping files from import dir
			removed++
		}
		return nil
	})
	return removed
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
