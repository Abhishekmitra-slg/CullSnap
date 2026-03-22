package raw

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// buildValidJPEG creates a valid JPEG image with the given dimensions.
func buildValidJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 50}); err != nil {
		t.Fatalf("failed to encode test JPEG: %v", err)
	}
	return buf.Bytes()
}

// buildTIFFWithJPEG creates a minimal TIFF file containing a real JPEG preview.
func buildTIFFWithJPEG(t *testing.T, jpegData []byte) []byte {
	t.Helper()
	bo := binary.LittleEndian

	ifdOffset := uint32(8)
	numEntries := uint16(3)
	ifdSize := 2 + int(numEntries)*12 + 4
	jpegDataOffset := uint32(8 + ifdSize)

	totalSize := int(jpegDataOffset) + len(jpegData)
	buf := make([]byte, totalSize)

	// Header.
	writeTIFFHeader(buf, bo, tiffMagic, ifdOffset)

	// IFD.
	pos := int(ifdOffset)
	bo.PutUint16(buf[pos:pos+2], numEntries)
	pos += 2

	writeIFDEntryShortInline(buf[pos:pos+12], bo, tagCompression, compressionJPEG)
	pos += 12

	writeIFDEntry(buf[pos:pos+12], bo, tagStripOffsets, 4, 1, jpegDataOffset)
	pos += 12

	writeIFDEntry(buf[pos:pos+12], bo, tagStripByteCounts, 4, 1, uint32(len(jpegData)))
	pos += 12

	bo.PutUint32(buf[pos:pos+4], 0)

	copy(buf[jpegDataOffset:], jpegData)
	return buf
}

func TestExtractPreview_NonRAWFile(t *testing.T) {
	_, err := ExtractPreview("/some/photo.jpg")
	if err == nil {
		t.Fatal("expected error for non-RAW extension")
	}
}

func TestExtractPreview_EmptyExtension(t *testing.T) {
	_, err := ExtractPreview("/some/file")
	if err == nil {
		t.Fatal("expected error for empty extension")
	}
}

func TestExtractPreview_TIFF(t *testing.T) {
	// Disable dcraw so the test only exercises the TIFF path.
	origAvailable := dcrawAvailable
	dcrawAvailable = false
	defer func() { dcrawAvailable = origAvailable }()

	jpegData := buildValidJPEG(t, 800, 600)
	tiffData := buildTIFFWithJPEG(t, jpegData)

	dir := t.TempDir()
	path := filepath.Join(dir, "test.cr2")
	if err := os.WriteFile(path, tiffData, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ExtractPreview(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty preview data")
	}
	if result[0] != 0xFF || result[1] != 0xD8 {
		t.Fatal("result does not start with JPEG SOI")
	}
}

func TestExtractPreview_TIFFVariantFormats(t *testing.T) {
	// Test that ORF, RW2, PEF, NRW, SRW extensions route through the TIFF parser.
	origAvailable := dcrawAvailable
	dcrawAvailable = false
	defer func() { dcrawAvailable = origAvailable }()

	jpegData := buildValidJPEG(t, 800, 600)

	// Standard TIFF formats (PEF, NRW, SRW use magic=42).
	standardExts := []string{".pef", ".nrw", ".srw"}
	for _, ext := range standardExts {
		t.Run(ext, func(t *testing.T) {
			tiffData := buildTIFFWithJPEG(t, jpegData)
			dir := t.TempDir()
			path := filepath.Join(dir, "test"+ext)
			if err := os.WriteFile(path, tiffData, 0o644); err != nil {
				t.Fatal(err)
			}

			result, err := ExtractPreview(path)
			if err != nil {
				t.Fatalf("unexpected error for %s: %v", ext, err)
			}
			if result[0] != 0xFF || result[1] != 0xD8 {
				t.Fatalf("result for %s does not start with JPEG SOI", ext)
			}
		})
	}
}

func TestExtractPreview_ORF(t *testing.T) {
	// ORF uses non-standard TIFF magic 0x4F52.
	origAvailable := dcrawAvailable
	dcrawAvailable = false
	defer func() { dcrawAvailable = origAvailable }()

	jpegData := buildValidJPEG(t, 800, 600)

	// Build a TIFF with ORF magic.
	bo := binary.LittleEndian
	ifdOffset := uint32(8)
	numEntries := uint16(3)
	ifdSize := 2 + int(numEntries)*12 + 4
	jpegDataOffset := uint32(8 + ifdSize)
	totalSize := int(jpegDataOffset) + len(jpegData)
	buf := make([]byte, totalSize)

	// Write header with ORF magic.
	buf[0], buf[1] = 'I', 'I'
	bo.PutUint16(buf[2:4], orfMagicOR)
	bo.PutUint32(buf[4:8], ifdOffset)

	pos := int(ifdOffset)
	bo.PutUint16(buf[pos:pos+2], numEntries)
	pos += 2
	writeIFDEntryShortInline(buf[pos:pos+12], bo, tagCompression, compressionJPEG)
	pos += 12
	writeIFDEntry(buf[pos:pos+12], bo, tagStripOffsets, 4, 1, jpegDataOffset)
	pos += 12
	writeIFDEntry(buf[pos:pos+12], bo, tagStripByteCounts, 4, 1, uint32(len(jpegData)))
	pos += 12
	bo.PutUint32(buf[pos:pos+4], 0)
	copy(buf[jpegDataOffset:], jpegData)

	dir := t.TempDir()
	path := filepath.Join(dir, "test.orf")
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ExtractPreview(path)
	if err != nil {
		t.Fatalf("unexpected error for ORF: %v", err)
	}
	if result[0] != 0xFF || result[1] != 0xD8 {
		t.Fatal("result does not start with JPEG SOI")
	}
}

func TestExtractPreview_RAF(t *testing.T) {
	origAvailable := dcrawAvailable
	dcrawAvailable = false
	defer func() { dcrawAvailable = origAvailable }()

	jpegData := buildValidJPEG(t, 800, 600)
	rafData := buildMinimalRAF(jpegData)

	dir := t.TempDir()
	path := filepath.Join(dir, "test.raf")
	if err := os.WriteFile(path, rafData, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ExtractPreview(path)
	if err != nil {
		t.Fatalf("unexpected error for RAF: %v", err)
	}
	if result[0] != 0xFF || result[1] != 0xD8 {
		t.Fatal("result does not start with JPEG SOI")
	}
}

func TestIsPreviewLargeEnough_Large(t *testing.T) {
	data := buildValidJPEG(t, 800, 600)
	if !isPreviewLargeEnough(data, 400) {
		t.Fatal("800px wide image should be large enough")
	}
}

func TestIsPreviewLargeEnough_Small(t *testing.T) {
	data := buildValidJPEG(t, 160, 120)
	if isPreviewLargeEnough(data, 400) {
		t.Fatal("160px wide image should not be large enough")
	}
}

func TestIsPreviewLargeEnough_InvalidJPEG(t *testing.T) {
	// Invalid data should return false (not panic).
	if isPreviewLargeEnough([]byte{0xFF, 0xD8, 0x00}, 400) {
		t.Fatal("invalid JPEG should return false")
	}
}

func TestExtractPreview_RealFiles(t *testing.T) {
	tests := []struct {
		file   string
		format string
	}{
		{"sample.cr2", "CR2"},
		{"sample.nef", "NEF"},
		{"sample.arw", "ARW"},
		{"sample.dng", "DNG"},
		{"sample.raf", "RAF"},
		{"sample.rw2", "RW2"},
		{"sample.orf", "ORF"},
		{"sample.pef", "PEF"},
		{"sample.nrw", "NRW"},
		{"sample.srw", "SRW"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			path := filepath.Join("testdata", tt.file)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Skipf("test fixture %s not found — run testdata/download.sh first", tt.file)
			}

			data, err := ExtractPreview(path)
			if err != nil {
				t.Fatalf("ExtractPreview(%s) error: %v", tt.format, err)
			}

			if len(data) < 2 {
				t.Fatal("preview too short")
			}
			if data[0] != 0xFF || data[1] != 0xD8 {
				t.Fatal("preview does not start with JPEG SOI marker")
			}

			cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("failed to decode preview config: %v", err)
			}
			if cfg.Width < 200 {
				t.Errorf("preview too small: %dx%d", cfg.Width, cfg.Height)
			}

			t.Logf("%s preview: %dx%d, %d bytes", tt.format, cfg.Width, cfg.Height, len(data))
		})
	}
}
