package dedupe

import (
	"bytes"
	"context"
	"cullsnap/internal/raw"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/corona10/goimagehash"
	"golang.org/x/sync/errgroup"
)

// PhotoInfo holds deduplication info for a single photo.
type PhotoInfo struct {
	Path     string
	Hash     *goimagehash.ImageHash
	IsUnique bool
	GroupID  string // used to bind duplicate photos together
}

// Group similar images based on perceptual hash.
type DuplicateGroup struct {
	Photos []*PhotoInfo
}

// hashImage concurrently loads a downscaled image and computes its difference hash safely.
func hashImage(path string) (result *goimagehash.ImageHash, err error) {
	ext := strings.ToLower(filepath.Ext(path))
	var img image.Image

	if raw.IsRAWExt(ext) {
		previewBytes, extractErr := raw.ExtractPreview(path)
		if extractErr != nil {
			return nil, fmt.Errorf("RAW preview extraction failed: %w", extractErr)
		}
		decoded, decErr := jpeg.Decode(bytes.NewReader(previewBytes))
		if decErr != nil {
			return nil, fmt.Errorf("failed to decode RAW preview: %w", decErr)
		}
		img = decoded
	} else {
		file, openErr := os.Open(path)
		if openErr != nil {
			return nil, fmt.Errorf("failed to open image %s: %w", path, openErr)
		}
		defer func() {
			if cerr := file.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}()

		decoded, _, decErr := image.Decode(file)
		if decErr != nil {
			return nil, fmt.Errorf("failed to decode image %s: %w", path, decErr)
		}
		img = decoded
	}

	hash, err := goimagehash.DifferenceHash(img)
	if err != nil {
		return nil, fmt.Errorf("failed to compute dHash for %s: %w", path, err)
	}

	return hash, nil
}

// FindDuplicates scans a directory concurrently, hashes them, and groups by similarity.
// similarityThreshold controls the hamming distance allowed (e.g., 5-10 for dHash)
func FindDuplicates(ctx context.Context, dirPath string, similarityThreshold int, progressCallback func(current, total int, message string)) ([]*DuplicateGroup, error) {
	var paths []string

	extensions := raw.ImageExtensions()

	// 1. Discover all images
	err := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors in walk gracefully
		}
		if !d.IsDir() && extensions[strings.ToLower(filepath.Ext(path))] {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 2. Hash images concurrently
	photos := make([]*PhotoInfo, len(paths))
	g := new(errgroup.Group)
	g.SetLimit(runtime.NumCPU()) // Limit concurrency to avoid memory exhaustion

	var processedCount int32
	totalCount := len(paths)
	if progressCallback != nil {
		progressCallback(0, totalCount, "Hashing images...")
	}

	for i, path := range paths {
		i, path := i, path
		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			hash, err := hashImage(path)
			if err != nil {
				// Skipping corrupted files gracefully instead of failing entire batch
				photos[i] = &PhotoInfo{Path: path}

				count := atomic.AddInt32(&processedCount, 1)
				if progressCallback != nil {
					progressCallback(int(count), totalCount, "Hashing images...")
				}
				return nil
			}
			photos[i] = &PhotoInfo{
				Path: path,
				Hash: hash,
			}

			count := atomic.AddInt32(&processedCount, 1)
			if progressCallback != nil {
				progressCallback(int(count), totalCount, "Hashing images...")
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	if progressCallback != nil {
		progressCallback(totalCount, totalCount, "Grouping duplicates...")
	}

	// 3. Group by perceptual similarity
	var groups []*DuplicateGroup
	for _, p := range photos {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if p.Hash == nil {
			continue // Skip un-hashable images
		}

		matched := false
		for _, g := range groups {
			if len(g.Photos) == 0 {
				continue
			}

			// Compare distance with the first image in the group
			distance, err := p.Hash.Distance(g.Photos[0].Hash)
			if err == nil && distance <= similarityThreshold {
				matched = true
				p.GroupID = g.Photos[0].Path // use first photo path as ID
				g.Photos = append(g.Photos, p)
				break
			}
		}

		if !matched {
			// New group
			newGroup := &DuplicateGroup{Photos: []*PhotoInfo{p}}
			p.GroupID = p.Path
			groups = append(groups, newGroup)
		}
	}

	// Mark Uniques
	var finalGroups []*DuplicateGroup
	for _, g := range groups {
		if len(g.Photos) == 1 {
			g.Photos[0].IsUnique = true
		} else {
			for _, p := range g.Photos {
				p.IsUnique = false
			}
		}
		finalGroups = append(finalGroups, g)
	}

	return finalGroups, nil
}
