package dedupe

import (
	"bytes"
	"context"
	"cullsnap/internal/raw"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/tiff"
)

// PhotoMeta holds enriched information about a photo, specifically EXIF data.
type PhotoMeta struct {
	Group      *DuplicateGroup
	DateTaken  time.Time
	HasDate    bool
	CameraMake string
}

// isTIFFBasedRAW returns true for RAW formats whose file structure starts with
// a TIFF header, meaning exif.Decode can read EXIF directly from the file stream.
func isTIFFBasedRAW(ext string) bool {
	switch ext {
	case ".cr2", ".nef", ".arw", ".dng":
		return true
	}
	return false
}

// ExtractDateTaken efficiently extracts EXIF metadata without allocating full images into memory.
func ExtractDateTaken(path string) (time.Time, bool) {
	ext := strings.ToLower(filepath.Ext(path))

	// Non-TIFF RAW formats: extract EXIF from the embedded JPEG preview.
	if raw.IsRAWExt(ext) && !isTIFFBasedRAW(ext) {
		previewBytes, err := raw.ExtractPreview(path)
		if err != nil {
			return time.Time{}, false
		}
		e, err := exif.Decode(bytes.NewReader(previewBytes))
		if err != nil {
			return time.Time{}, false
		}
		date, err := e.DateTime()
		if err != nil || date.IsZero() {
			return time.Time{}, false
		}
		return date, true
	}

	// TIFF-based RAW and standard JPEG/PNG: exif.Decode works directly on the file.
	file, err := os.Open(path)
	if err != nil {
		return time.Time{}, false
	}
	defer func() { _ = file.Close() }()

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

// FullEXIF contains detailed EXIF metadata for display purposes.
type FullEXIF struct {
	Camera    string
	Lens      string
	ISO       string
	Aperture  string
	Shutter   string
	DateTaken string
}

// ExtractFullEXIF extracts comprehensive EXIF metadata from a photo.
func ExtractFullEXIF(path string) (_ *FullEXIF, err error) {
	ext := strings.ToLower(filepath.Ext(path))

	var e *exif.Exif

	// Non-TIFF RAW formats: extract EXIF from the embedded JPEG preview.
	if raw.IsRAWExt(ext) && !isTIFFBasedRAW(ext) {
		previewBytes, extractErr := raw.ExtractPreview(path)
		if extractErr != nil {
			return nil, fmt.Errorf("RAW preview extraction failed: %w", extractErr)
		}
		decoded, decErr := exif.Decode(bytes.NewReader(previewBytes))
		if decErr != nil {
			return nil, fmt.Errorf("no EXIF data in RAW preview: %w", decErr)
		}
		e = decoded
	} else {
		// TIFF-based RAW and standard JPEG/PNG: exif.Decode works directly on the file.
		file, openErr := os.Open(path)
		if openErr != nil {
			return nil, openErr
		}
		defer func() {
			if cerr := file.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}()

		decoded, decErr := exif.Decode(file)
		if decErr != nil {
			return nil, fmt.Errorf("no EXIF data: %w", decErr)
		}
		e = decoded
	}

	info := &FullEXIF{}

	// Camera make + model
	if make, err := e.Get(exif.Make); err == nil {
		info.Camera = tagString(make)
	}
	if model, err := e.Get(exif.Model); err == nil {
		if info.Camera != "" {
			info.Camera += " "
		}
		info.Camera += tagString(model)
	}

	// Lens
	if lens, err := e.Get(exif.LensModel); err == nil {
		info.Lens = tagString(lens)
	}

	// ISO
	if iso, err := e.Get(exif.ISOSpeedRatings); err == nil {
		info.ISO = tagString(iso)
	}

	// Aperture (FNumber)
	if fnum, err := e.Get(exif.FNumber); err == nil {
		n, d, _ := fnum.Rat2(0)
		if d != 0 {
			info.Aperture = fmt.Sprintf("f/%.1f", float64(n)/float64(d))
		}
	}

	// Shutter speed (ExposureTime)
	if exp, err := e.Get(exif.ExposureTime); err == nil {
		n, d, _ := exp.Rat2(0)
		if d != 0 {
			if n == 1 {
				info.Shutter = fmt.Sprintf("1/%ds", d)
			} else {
				info.Shutter = fmt.Sprintf("%.1fs", float64(n)/float64(d))
			}
		}
	}

	// Date taken
	if date, err := e.DateTime(); err == nil && !date.IsZero() {
		info.DateTaken = date.Format("Jan 2, 2006 3:04 PM")
	}

	return info, nil
}

// tagString safely extracts a string from a TIFF tag.
func tagString(t *tiff.Tag) string {
	s, err := t.StringVal()
	if err != nil {
		return t.String()
	}
	return s
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
