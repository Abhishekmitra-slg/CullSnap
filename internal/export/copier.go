package export

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cullsnap/internal/model"
)

// ExportSelections copies selected photos to the destination directory.
// It returns the number of successfully copied files and any error encountered.
func ExportSelections(photos []model.Photo, destDir string) (int, error) {
	// Create destination directory if it doesn't exist
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create destination directory: %w", err)
	}

	count := 0
	for _, p := range photos {
		srcFile, err := os.Open(p.Path)
		if err != nil {
			// Log error and continue? For now return error to stop.
			// Or maybe return a list of errors.
			return count, fmt.Errorf("failed to open source file %s: %w", p.Path, err)
		}
		defer srcFile.Close()

		filename := filepath.Base(p.Path)
		destPath := filepath.Join(destDir, filename)

		// Handle duplicates
		destPath = uniquePath(destPath)

		destFile, err := os.Create(destPath)
		if err != nil {
			srcFile.Close()
			return count, fmt.Errorf("failed to create destination file %s: %w", destPath, err)
		}

		_, err = io.Copy(destFile, srcFile)
		destFile.Close()
		srcFile.Close()

		if err != nil {
			return count, fmt.Errorf("failed to copy content to %s: %w", destPath, err)
		}
		count++
	}

	return count, nil
}

// uniquePath appends a counter to the filename if it already exists.
func uniquePath(path string) string {
	ext := filepath.Ext(path)
	name := strings.TrimSuffix(filepath.Base(path), ext)
	dir := filepath.Dir(path)

	newPath := path
	counter := 1
	for {
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			break
		}
		newPath = filepath.Join(dir, fmt.Sprintf("%s_%d%s", name, counter, ext))
		counter++
	}
	return newPath
}
