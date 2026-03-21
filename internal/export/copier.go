package export

import (
	"cullsnap/internal/model"
	"cullsnap/internal/video"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ExportSelections copies selected photos to the destination directory.
// It returns the number of successfully copied files and any error encountered.
func ExportSelections(photos []model.Photo, destDir string) (int, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return 0, fmt.Errorf("failed to create destination directory: %w", err)
	}

	count := 0
	for i := range photos {
		if err := exportSingleFile(photos[i], destDir); err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}

// exportSingleFile handles exporting a single photo/video so defer runs per-file.
func exportSingleFile(p model.Photo, destDir string) error {
	filename := filepath.Base(p.Path)
	destPath := uniquePath(filepath.Join(destDir, filename))

	isClippedVideo := p.IsVideo && (p.TrimStart > 0 || p.TrimEnd > 0)
	if isClippedVideo {
		if p.TrimEnd == 0 {
			p.TrimEnd = p.Duration
		}
		// If both start and end are 0, or end <= start, skip trimming and copy as-is.
		// This guards against Duration not being enriched yet.
		if p.TrimEnd > p.TrimStart {
			if err := video.TrimVideo(p.Path, destPath, p.TrimStart, p.TrimEnd); err != nil {
				return fmt.Errorf("failed to trim video %s: %w", p.Path, err)
			}
			return nil
		}
		// Fall through to plain copy if trim range is invalid
	}

	srcFile, err := os.Open(p.Path)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", p.Path, err)
	}
	defer func() { _ = srcFile.Close() }() // read-only; Close error is safe to ignore

	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %w", destPath, err)
	}

	if _, err := io.Copy(destFile, srcFile); err != nil {
		_ = destFile.Close() // best-effort close on copy failure
		return fmt.Errorf("failed to copy content to %s: %w", destPath, err)
	}
	if err := destFile.Close(); err != nil {
		return fmt.Errorf("failed to flush %s: %w", destPath, err)
	}
	return nil
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
