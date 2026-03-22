package raw

import (
	"cullsnap/internal/logger"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

// RAF header constants.
const (
	rafMagic      = "FUJIFILMCCD-RAW "
	rafHeaderSize = 16 + 4 + 8 + 32 // magic + version + camera ID + camera string
	rafDirOffset  = rafHeaderSize   // offset directory starts after header
	rafDirMinSize = 4 + 20 + 4 + 4  // version + unknown + jpeg offset + jpeg length
)

// extractRAFPreview extracts the embedded JPEG preview from a Fujifilm RAF file.
//
// RAF layout:
//
//	Bytes 0-15:   Magic "FUJIFILMCCD-RAW " (16 bytes)
//	Bytes 16-19:  Format version (e.g. "0201")
//	Bytes 20-27:  Camera ID (8 bytes)
//	Bytes 28-59:  Camera string (32 bytes, null-terminated)
//	Bytes 60-63:  Offset directory version (4 bytes)
//	Bytes 64-83:  Unknown (20 bytes)
//	Bytes 84-87:  JPEG image offset (4 bytes, big-endian)
//	Bytes 88-91:  JPEG image length (4 bytes, big-endian)
func extractRAFPreview(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("raf: open: %w", err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("raf: stat: %w", err)
	}
	fileSize := info.Size()

	// Need at least the header + offset directory.
	minSize := int64(rafDirOffset + rafDirMinSize)
	if fileSize < minSize {
		return nil, fmt.Errorf("raf: file too small (%d bytes, need at least %d)", fileSize, minSize)
	}

	// Read the header + offset directory in one read.
	var hdr [rafDirOffset + rafDirMinSize]byte
	if _, err := f.ReadAt(hdr[:], 0); err != nil {
		return nil, fmt.Errorf("raf: read header: %w", err)
	}

	// Validate magic.
	if string(hdr[0:16]) != rafMagic {
		return nil, fmt.Errorf("raf: invalid magic: %q", string(hdr[0:16]))
	}

	logger.Log.Debug("raf: parsing", "path", path, "fileSize", fileSize,
		"version", string(hdr[16:20]), "camera", nullTermString(hdr[28:60]))

	// Read JPEG offset and length from the offset directory (big-endian).
	jpegOffset := binary.BigEndian.Uint32(hdr[84:88])
	jpegLength := binary.BigEndian.Uint32(hdr[88:92])

	logger.Log.Debug("raf: JPEG location", "offset", jpegOffset, "length", jpegLength)

	if jpegOffset == 0 || jpegLength == 0 {
		return nil, errors.New("raf: JPEG offset or length is zero")
	}

	if int64(jpegOffset)+int64(jpegLength) > fileSize {
		return nil, fmt.Errorf("raf: JPEG data (offset=%d, length=%d) exceeds file size %d",
			jpegOffset, jpegLength, fileSize)
	}

	// Read the JPEG data.
	buf := make([]byte, jpegLength)
	if _, err := f.ReadAt(buf, int64(jpegOffset)); err != nil {
		return nil, fmt.Errorf("raf: read JPEG: %w", err)
	}

	// Validate JPEG SOI marker.
	if len(buf) < 2 || buf[0] != 0xFF || buf[1] != 0xD8 {
		return nil, errors.New("raf: data at JPEG offset does not start with JPEG SOI marker")
	}

	logger.Log.Debug("raf: preview extracted", "path", path, "size", len(buf))
	return buf, nil
}

// nullTermString returns the null-terminated string from a byte slice.
func nullTermString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
