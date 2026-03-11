package dedupe

import (
	"context"
	"os"
	"sort"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

// PhotoMeta holds enriched information about a photo, specifically EXIF data.
type PhotoMeta struct {
	Group      *DuplicateGroup
	DateTaken  time.Time
	HasDate    bool
	CameraMake string
}

// ExtractDateTaken efficiently extracts EXIF metadata without allocating full images into memory.
func ExtractDateTaken(path string) (time.Time, bool) {
	file, err := os.Open(path)
	if err != nil {
		return time.Time{}, false
	}
	defer file.Close()

	// Parse EXIF fast
	e, err := exif.Decode(file)
	if err != nil {
		return time.Time{}, false
	}

	date, err := e.DateTime()
	if err != nil || date.IsZero() {
		return time.Time{}, false
	}

	return date, true
}

// SortGroupsByDate extracts dates from the "Unique" representation inside a group
// and sorts the groups chronologically.
func SortGroupsByDate(ctx context.Context, groups []*DuplicateGroup) error {
	// 1. Assign Date Taken to each group based on the representative unique photo
	type GroupMeta struct {
		G     *DuplicateGroup
		Date  time.Time
		Valid bool
	}

	metaList := make([]*GroupMeta, len(groups))

	for i, g := range groups {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var representative string
		if len(g.Photos) > 0 {
			representative = g.Photos[0].Path // Fallback
			for _, p := range g.Photos {
				if p.IsUnique {
					representative = p.Path
					break
				}
			}
		}

		if representative != "" {
			date, valid := ExtractDateTaken(representative)
			metaList[i] = &GroupMeta{G: g, Date: date, Valid: valid}
		} else {
			metaList[i] = &GroupMeta{G: g, Valid: false}
		}
	}

	// 2. Sort the groups: Valid dates first (ascending), then invalid dates
	sort.Slice(metaList, func(i, j int) bool {
		m1 := metaList[i]
		m2 := metaList[j]

		if m1.Valid && m2.Valid {
			return m1.Date.Before(m2.Date)
		}
		if m1.Valid && !m2.Valid {
			return true // Put valid dates before invalid ones
		}
		if !m1.Valid && m2.Valid {
			return false
		}
		// If both invalid, sort by path string just to keep consistent order
		if len(m1.G.Photos) > 0 && len(m2.G.Photos) > 0 {
			return m1.G.Photos[0].Path < m2.G.Photos[0].Path
		}
		return false
	})

	// 3. Reconstruct into the original slice inline
	for i, m := range metaList {
		groups[i] = m.G
	}
	return nil
}
