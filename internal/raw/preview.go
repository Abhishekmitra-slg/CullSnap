package raw

import (
	"bytes"
	"cullsnap/internal/logger"
	"errors"
	"fmt"
	"image/jpeg"
	"path/filepath"
	"strings"
)

// minPreviewWidth is the minimum acceptable width for an embedded preview.
// Previews smaller than this trigger a dcraw fallback attempt.
const minPreviewWidth = 400

// ExtractPreview extracts a JPEG preview from the given RAW file.
// It uses a two-path strategy:
//   - Path A: native Go extraction (TIFF parser for CR2/NEF/ARW/DNG/ORF/RW2/PEF/NRW/SRW,
//     BMFF parser for CR3, RAF header parser for RAF)
//   - Path B: dcraw fallback when Path A fails or preview is too small
//
// Returns the JPEG bytes or an error if no preview could be extracted.
func ExtractPreview(path string) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return nil, errors.New("preview: file has no extension")
	}
	if !IsRAWExt(ext) {
		return nil, fmt.Errorf("preview: unsupported extension %q", ext)
	}

	logger.Log.Debug("preview: extracting", "path", path, "format", FormatName(ext))

	var pathAData []byte
	var pathAErr error

	switch ext {
	case ".cr3":
		pathAData, pathAErr = extractCR3Preview(path)
		if pathAErr != nil {
			logger.Log.Debug("preview: CR3 extraction failed, trying dcraw", "path", path, "error", pathAErr)
		}
	case ".raf":
		pathAData, pathAErr = extractRAFPreview(path)
		if pathAErr != nil {
			logger.Log.Debug("preview: RAF extraction failed, trying dcraw", "path", path, "error", pathAErr)
		}
	case ".cr2", ".nef", ".arw", ".dng", ".orf", ".rw2", ".pef", ".nrw", ".srw":
		// Standard TIFF and TIFF-variant formats.
		// ORF (Olympus): TIFF with magic 0x4F52 or 0x5253 instead of 42.
		// RW2 (Panasonic): TIFF with magic 0x0055 instead of 42.
		// PEF (Pentax), NRW (Nikon), SRW (Samsung): standard TIFF (magic=42).
		pathAData, pathAErr = extractTIFFPreview(path)
		if pathAErr != nil {
			logger.Log.Debug("preview: TIFF extraction failed, trying dcraw", "path", path, "error", pathAErr)
		}
	}

	// If Path A succeeded, check dimensions.
	if pathAData != nil {
		if isPreviewLargeEnough(pathAData, minPreviewWidth) {
			logger.Log.Debug("preview: Path A preview accepted", "path", path, "size", len(pathAData))
			return pathAData, nil
		}
		logger.Log.Debug("preview: Path A preview too small, trying dcraw", "path", path)
	}

	// Path B: dcraw fallback.
	dcrawData, dcrawErr := ExtractPreviewDcraw(path)
	if dcrawErr == nil {
		logger.Log.Debug("preview: dcraw preview extracted", "path", path, "size", len(dcrawData))
		return dcrawData, nil
	}
	logger.Log.Debug("preview: dcraw fallback failed", "path", path, "error", dcrawErr)

	// If Path A returned a small-but-valid preview, use it despite being small.
	if pathAData != nil {
		logger.Log.Debug("preview: using small Path A preview as last resort", "path", path, "size", len(pathAData))
		return pathAData, nil
	}

	// Both paths failed.
	if pathAErr != nil {
		return nil, fmt.Errorf("preview: all extraction methods failed for %s: pathA=%w, dcraw=%v", ext, pathAErr, dcrawErr)
	}
	return nil, fmt.Errorf("preview: all extraction methods failed for %s: %v", ext, dcrawErr)
}

// isPreviewLargeEnough decodes the JPEG header and returns true if the
// image width is at least minWidth pixels.
func isPreviewLargeEnough(jpegData []byte, minWidth int) bool {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpegData))
	if err != nil {
		logger.Log.Debug("preview: cannot decode JPEG config for size check", "error", err)
		return false
	}
	logger.Log.Debug("preview: JPEG dimensions", "width", cfg.Width, "height", cfg.Height)
	return cfg.Width >= minWidth
}
