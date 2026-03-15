package export

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"cullsnap/internal/model"
	"cullsnap/internal/video"
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
		
		isClippedVideo := p.IsVideo && (p.TrimStart > 0 || p.TrimEnd > 0)
		if isClippedVideo {
			// Ensure file is closed because FFmpeg handles its own I/O
			destFile.Close()
			srcFile.Close()
			
			// Remove the empty placeholder file 
			os.Remove(destPath)

			// If TrimEnd isn't specified or is larger than duration somehow, FFmpeg handles it,
			// but best to pass the duration precisely.
			if p.TrimEnd == 0 {
				p.TrimEnd = p.Duration
			}
			
			err = video.TrimVideo(p.Path, destPath, p.TrimStart, p.TrimEnd)
			if err != nil {
				return count, fmt.Errorf("failed to trim video %s: %w", p.Path, err)
			}
		} else {
			_, err = io.Copy(destFile, srcFile)
			destFile.Close()
			srcFile.Close()
		}

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
