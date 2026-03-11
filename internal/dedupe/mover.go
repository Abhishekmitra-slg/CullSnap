package dedupe

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// MoveDuplicate moves a duplicate photo into a "duplicates/" subfolder within
// its original directory. It prefers renaming, but falls back to copying if across drives.
func MoveDuplicate(originalPath string) (string, error) {
	dir := filepath.Dir(originalPath)
	filename := filepath.Base(originalPath)

	dupeDir := filepath.Join(dir, "duplicates")

	// Ensure duplicates directory exists
	err := os.MkdirAll(dupeDir, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to create duplicates dir: %w", err)
	}

	newPath := filepath.Join(dupeDir, filename)

	// Attempt standard rename first (fast, atomic within same FS)
	err = os.Rename(originalPath, newPath)
	if err == nil {
		return newPath, nil
	}

	// Fallback to Move (copy + delete) if cross-filesystem or locked rename failed
	err = copyFile(originalPath, newPath)
	if err != nil {
		return "", fmt.Errorf("failed fallback copy: %w", err)
	}

	// Remove original after successful copy
	err = os.Remove(originalPath)
	if err != nil {
		// If removal fails, we might have weird state. Attempt to clean up.
		_ = os.Remove(newPath)
		return "", fmt.Errorf("failed to remove original post-copy: %w", err)
	}

	return newPath, nil
}

// RelocateGroupDuplicates physically moves non-unique photos of the groups.
// Updates the Path of the PhotoInfo struct to the new location.
func RelocateGroupDuplicates(ctx context.Context, groups []*DuplicateGroup) []error {
	var errs []error

	for _, g := range groups {
		select {
		case <-ctx.Done():
			return []error{ctx.Err()}
		default:
		}

		for _, p := range g.Photos {
			if !p.IsUnique {
				newPath, err := MoveDuplicate(p.Path)
				if err != nil {
					errs = append(errs, fmt.Errorf("failed to move %s: %w", p.Path, err))
				} else {
					p.Path = newPath // Update local reference
				}
			}
		}
	}
	return errs
}

// copyFile is a utility for cross-device fallback.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}

	err = out.Sync()
	return err
}
