package dedupe

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/corona10/goimagehash"
	"github.com/disintegration/imaging"
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
func hashImage(path string) (*goimagehash.ImageHash, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open image %s: %w", path, err)
	}
	defer file.Close()

	// Decoding the full image might take huge memory.
	// Since hashes only need a tiny downscaled matrix, we use decodeConfig to check dimensions
	// but we must fully decode it ultimately. disintegration/imaging supports resizing.
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image %s: %w", path, err)
	}

	// Downscale heavily before hashing to save matrix overhead inside hashing func
	// a simple 256x256 is plenty for perceptual hashes.
	thumb := imaging.Resize(img, 256, 0, imaging.NearestNeighbor)

	hash, err := goimagehash.DifferenceHash(thumb)
	if err != nil {
		return nil, fmt.Errorf("failed to compute dHash for %s: %w", path, err)
	}

	return hash, nil
}

// FindDuplicates scans a directory concurrently, hashes them, and groups by similarity.
// similarityThreshold controls the hamming distance allowed (e.g., 5-10 for dHash)
func FindDuplicates(ctx context.Context, dirPath string, similarityThreshold int, progressCallback func(current, total int, message string)) ([]*DuplicateGroup, error) {
	var paths []string

	extensions := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, // RAW decoding could be added later
	}

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
