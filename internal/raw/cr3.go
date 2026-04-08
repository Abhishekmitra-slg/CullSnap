package raw

import (
	"bytes"
	"cullsnap/internal/logger"
	"encoding/binary"
	"fmt"
	"os"
)

// prvwUUID is the UUID of the BMFF box containing the PRVW preview in CR3 files.
var prvwUUID = [16]byte{
	0xea, 0xf4, 0x2b, 0x5e, 0x1c, 0x98, 0x4b, 0x88,
	0xb9, 0xfb, 0xb7, 0xdc, 0x40, 0x6e, 0x4d, 0x16,
}

// extractCR3Preview extracts the embedded JPEG preview from a CR3 (Canon BMFF) file.
// Returns the JPEG bytes of the ~1620x1080 preview image.
// Pass 1: looks for PRVW UUID box (standard Canon CR3 preview).
// Pass 2: looks for THMB box inside container boxes (Canon R-series fallback).
func extractCR3Preview(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cr3: open: %w", err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("cr3: stat: %w", err)
	}
	fileSize := info.Size()

	logger.Log.Debug("cr3: parsing BMFF boxes", "path", path, "fileSize", fileSize)

	// Pass 1: Look for PRVW UUID box (standard Canon CR3 preview).
	data, err := findPRVWInFile(f, fileSize)
	if err == nil {
		return data, nil
	}
	logger.Log.Debug("cr3: PRVW not found, trying THMB fallback", "path", path)

	// Pass 2: Look for THMB box (some Canon R-series cameras).
	data, err = findTHMBInFile(f, fileSize)
	if err == nil {
		return data, nil
	}
	logger.Log.Debug("cr3: THMB not found either", "path", path)

	return nil, fmt.Errorf("cr3: no preview found in %s (tried PRVW and THMB)", path)
}

// findPRVWInFile walks the root-level BMFF boxes looking for the PRVW UUID box.
func findPRVWInFile(f *os.File, fileSize int64) ([]byte, error) {
	var offset int64
	for offset < fileSize {
		// Read box header: 4 bytes size + 4 bytes type
		var header [8]byte
		if _, err := f.ReadAt(header[:], offset); err != nil {
			break
		}

		boxSize := int64(binary.BigEndian.Uint32(header[0:4]))
		boxType := string(header[4:8])

		// Handle extended size (size == 1 means 64-bit size follows)
		if boxSize == 1 {
			var extSize [8]byte
			if _, err := f.ReadAt(extSize[:], offset+8); err != nil {
				break
			}
			boxSize = int64(binary.BigEndian.Uint64(extSize[:]))
		}

		// Sanity check
		if boxSize < 8 || offset+boxSize > fileSize {
			break
		}

		logger.Log.Debug("cr3: box", "type", boxType, "offset", offset, "size", boxSize)

		if boxType == "uuid" && boxSize >= 24 {
			// Read UUID (16 bytes after the 8-byte header)
			var uuid [16]byte
			if _, err := f.ReadAt(uuid[:], offset+8); err != nil {
				offset += boxSize
				continue
			}

			if uuid == prvwUUID {
				logger.Log.Debug("cr3: found PRVW UUID box", "offset", offset)

				// Read the content of this UUID box (after header + UUID = offset+24)
				// Inside, there should be a PRVW sub-box
				contentStart := offset + 24
				contentEnd := offset + boxSize

				return findPRVWJPEG(f, contentStart, contentEnd)
			}
		}

		offset += boxSize
	}

	return nil, fmt.Errorf("cr3: PRVW box not found")
}

// findPRVWJPEG searches within the UUID box content for the PRVW sub-box
// and extracts the JPEG data from it.
func findPRVWJPEG(f *os.File, start, end int64) ([]byte, error) {
	offset := start

	for offset < end-8 {
		var header [8]byte
		if _, err := f.ReadAt(header[:], offset); err != nil {
			break
		}

		boxSize := int64(binary.BigEndian.Uint32(header[0:4]))
		boxType := string(header[4:8])

		if boxSize < 8 || offset+boxSize > end {
			break
		}

		if boxType == "PRVW" {
			logger.Log.Debug("cr3: found PRVW box", "offset", offset, "size", boxSize)

			// Read the PRVW payload (after 8-byte box header)
			payloadStart := offset + 8
			payloadSize := boxSize - 8

			if payloadSize < 14 {
				return nil, fmt.Errorf("cr3: PRVW payload too small: %d", payloadSize)
			}

			// Read the full payload
			payload := make([]byte, payloadSize)
			if _, err := f.ReadAt(payload, payloadStart); err != nil {
				return nil, fmt.Errorf("cr3: read PRVW payload: %w", err)
			}

			// Find JPEG SOI marker (0xFFD8) in the payload.
			// The header before JPEG is typically 14-18 bytes.
			soiIdx := bytes.Index(payload, []byte{0xFF, 0xD8})
			if soiIdx < 0 {
				return nil, fmt.Errorf("cr3: no JPEG SOI marker in PRVW data")
			}

			jpegData := payload[soiIdx:]
			logger.Log.Debug("cr3: extracted JPEG preview", "jpegSize", len(jpegData), "headerSkipped", soiIdx)

			return jpegData, nil
		}

		offset += boxSize
	}

	// Fallback: scan the entire UUID box content for JPEG SOI marker
	contentSize := end - start
	if contentSize > 0 && contentSize < 50*1024*1024 { // sanity: max 50MB
		content := make([]byte, contentSize)
		if _, err := f.ReadAt(content, start); err == nil {
			soiIdx := bytes.Index(content, []byte{0xFF, 0xD8})
			if soiIdx >= 0 {
				logger.Log.Debug("cr3: found JPEG via SOI scan fallback", "offset", soiIdx)
				return content[soiIdx:], nil
			}
		}
	}

	return nil, fmt.Errorf("cr3: PRVW JPEG not found")
}

// findTHMBInFile searches the file recursively for THMB boxes and returns the largest JPEG found.
func findTHMBInFile(f *os.File, fileSize int64) ([]byte, error) {
	return findTHMBRecursive(f, 0, fileSize)
}

// findTHMBRecursive walks BMFF boxes in the range [start, end), recursing into container boxes.
// It returns the largest JPEG found across all THMB boxes.
func findTHMBRecursive(f *os.File, start, end int64) ([]byte, error) {
	offset := start
	var bestJPEG []byte

	for offset < end-8 {
		// Read 8-byte box header
		var header [8]byte
		if _, err := f.ReadAt(header[:], offset); err != nil {
			break
		}
		boxSize := int64(binary.BigEndian.Uint32(header[0:4]))
		boxType := string(header[4:8])

		// Handle extended size
		if boxSize == 1 {
			var extSize [8]byte
			if _, err := f.ReadAt(extSize[:], offset+8); err != nil {
				break
			}
			boxSize = int64(binary.BigEndian.Uint64(extSize[:]))
		}
		if boxSize < 8 || offset+boxSize > end {
			break
		}

		logger.Log.Debug("cr3: thmb scan box", "type", boxType, "offset", offset, "size", boxSize)

		if boxType == "THMB" {
			// THMB payload: variable Canon header then JPEG data. Scan for SOI marker.
			payloadStart := offset + 8
			payloadSize := boxSize - 8
			if payloadSize > 0 && payloadSize < 10*1024*1024 {
				payload := make([]byte, payloadSize)
				if _, err := f.ReadAt(payload, payloadStart); err != nil {
					logger.Log.Warn("cr3: failed to read THMB payload", "offset", payloadStart, "size", payloadSize, "error", err)
					offset += boxSize
					continue
				}
				soiIdx := bytes.Index(payload, []byte{0xFF, 0xD8})
				if soiIdx >= 0 {
					jpegData := payload[soiIdx:]
					logger.Log.Debug("cr3: found THMB JPEG", "offset", offset, "jpegSize", len(jpegData))
					if len(jpegData) > len(bestJPEG) {
						bestJPEG = jpegData
					}
				}
			}
		}

		// Recurse into container boxes
		if isContainerBox(boxType) {
			childData, err := findTHMBRecursive(f, offset+8, offset+boxSize)
			if err == nil && len(childData) > len(bestJPEG) {
				bestJPEG = childData
			}
		}

		offset += boxSize
	}

	if bestJPEG != nil {
		return bestJPEG, nil
	}
	return nil, fmt.Errorf("cr3: no THMB box found")
}

// isContainerBox reports whether the box type is a known BMFF container.
func isContainerBox(boxType string) bool {
	switch boxType {
	case "moov", "trak", "mdia", "minf", "stbl", "dinf", "edts",
		"CRAW", "CMT1", "CMT2", "CMT3", "CMT4":
		return true
	}
	return false
}
