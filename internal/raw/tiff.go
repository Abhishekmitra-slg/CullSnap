package raw

import (
	"cullsnap/internal/logger"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// TIFF tag IDs.
const (
	tagNewSubFileType           = 0x00FE
	tagCompression              = 0x0103
	tagStripOffsets             = 0x0111
	tagStripByteCounts          = 0x0117
	tagJPEGInterchangeFormat    = 0x0201
	tagJPEGInterchangeFormatLen = 0x0202
	tagSubIFDs                  = 0x014A
)

// Compression values.
const (
	compressionOJPEG = 6
	compressionJPEG  = 7
)

// Safety limits.
const (
	maxIFDs   = 20
	jpegSOI   = 0xFFD8
	tiffMagic = 42
	bigTIFF   = 43

	// Variant TIFF magic numbers used by camera manufacturers.
	orfMagicOR = 0x4F52 // Olympus ORF: "OR" in big-endian (IIRO / MMOR)
	orfMagicRS = 0x5253 // Olympus ORF: "RS" (IIRS)
	rw2Magic   = 0x0055 // Panasonic RW2
)

// TIFF data type sizes (indexed by type ID 1..5).
var tiffTypeSize = map[uint16]uint32{
	1: 1, // BYTE
	2: 1, // ASCII
	3: 2, // SHORT
	4: 4, // LONG
	5: 8, // RATIONAL
}

// jpegPreview holds information about a JPEG preview found in an IFD.
type jpegPreview struct {
	offset uint32
	size   uint32
}

// extractTIFFPreview parses a TIFF-based RAW file and returns the largest
// embedded JPEG preview found across all IFDs.
func extractTIFFPreview(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("tiff: open: %w", err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("tiff: stat: %w", err)
	}
	fileSize := info.Size()

	if fileSize < 8 {
		return nil, errors.New("tiff: file too small for TIFF header")
	}

	// Read 8-byte TIFF header.
	var hdr [8]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return nil, fmt.Errorf("tiff: read header: %w", err)
	}

	var bo binary.ByteOrder
	switch string(hdr[0:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return nil, fmt.Errorf("tiff: invalid byte order mark: %x%x", hdr[0], hdr[1])
	}

	magic := bo.Uint16(hdr[2:4])
	ifd0Offset := bo.Uint32(hdr[4:8])

	orderName := "little-endian"
	if bo == binary.BigEndian {
		orderName = "big-endian"
	}
	logger.Log.Debug("tiff: parsing header", "path", path, "byteOrder", orderName, "magic", magic, "ifd0Offset", ifd0Offset)

	if magic == bigTIFF {
		return nil, errors.New("tiff: BigTIFF (magic=43) is not supported")
	}
	if !isAcceptedTIFFMagic(magic) {
		return nil, fmt.Errorf("tiff: unexpected magic number %d (0x%04X)", magic, magic)
	}

	visited := make(map[uint32]bool)
	ifdCount := 0
	var previews []jpegPreview

	// walkIFDChain follows the linked list of IFDs.
	var walkIFDChain func(offset uint32) error
	walkIFDChain = func(offset uint32) error {
		for offset != 0 {
			if ifdCount >= maxIFDs {
				return nil
			}
			if visited[offset] {
				return nil // circular chain
			}
			visited[offset] = true

			p, nextOffset, subIFDOffsets, err := parseIFD(f, bo, offset, fileSize, ifdCount)
			if err != nil {
				return err
			}
			ifdCount++

			if p != nil {
				previews = append(previews, *p)
			}

			// Recurse into SubIFDs.
			for _, sub := range subIFDOffsets {
				if err := walkIFDChain(sub); err != nil {
					return err
				}
			}

			offset = nextOffset
		}
		return nil
	}

	if err := walkIFDChain(ifd0Offset); err != nil {
		return nil, err
	}

	if len(previews) == 0 {
		return nil, errors.New("tiff: no JPEG preview found")
	}

	// Select the largest preview.
	best := previews[0]
	for _, p := range previews[1:] {
		if p.size > best.size {
			best = p
		}
	}
	logger.Log.Debug("tiff: selected largest preview", "offset", best.offset, "size", best.size)

	// Read the JPEG data.
	if int64(best.offset)+int64(best.size) > fileSize {
		return nil, fmt.Errorf("tiff: preview at offset %d size %d exceeds file size %d", best.offset, best.size, fileSize)
	}

	buf := make([]byte, best.size)
	if _, err := f.ReadAt(buf, int64(best.offset)); err != nil {
		return nil, fmt.Errorf("tiff: read preview: %w", err)
	}

	if len(buf) < 2 || binary.BigEndian.Uint16(buf[0:2]) != jpegSOI {
		return nil, errors.New("tiff: preview data does not start with JPEG SOI marker")
	}

	return buf, nil
}

// parseIFD reads a single IFD at the given offset. Returns a jpegPreview (if
// this IFD contains one), the next-IFD offset, any SubIFD offsets, and error.
func parseIFD(f *os.File, bo binary.ByteOrder, offset uint32, fileSize int64, index int) (*jpegPreview, uint32, []uint32, error) {
	if int64(offset)+2 > fileSize {
		return nil, 0, nil, fmt.Errorf("tiff: IFD offset %d beyond EOF", offset)
	}

	var countBuf [2]byte
	if _, err := f.ReadAt(countBuf[:], int64(offset)); err != nil {
		return nil, 0, nil, fmt.Errorf("tiff: read IFD entry count: %w", err)
	}
	entryCount := bo.Uint16(countBuf[:])

	logger.Log.Debug("tiff: visiting IFD", "index", index, "offset", offset, "entryCount", entryCount)

	ifdEnd := int64(offset) + 2 + int64(entryCount)*12 + 4
	if ifdEnd > fileSize {
		return nil, 0, nil, fmt.Errorf("tiff: IFD at %d with %d entries extends beyond EOF", offset, entryCount)
	}

	// Read all entries at once.
	entriesSize := int(entryCount) * 12
	entriesBuf := make([]byte, entriesSize+4) // +4 for next IFD pointer
	if _, err := f.ReadAt(entriesBuf, int64(offset)+2); err != nil {
		return nil, 0, nil, fmt.Errorf("tiff: read IFD entries: %w", err)
	}

	var compression uint32
	var stripOffsets, stripByteCounts []uint32
	var jpegOffset, jpegLength uint32
	var subIFDOffsets []uint32
	hasCompression := false

	for i := 0; i < int(entryCount); i++ {
		entry := entriesBuf[i*12 : i*12+12]
		tag := bo.Uint16(entry[0:2])
		typ := bo.Uint16(entry[2:4])
		count := bo.Uint32(entry[4:8])
		valueOrOffset := entry[8:12]

		switch tag {
		case tagCompression:
			compression = readTagValue(bo, typ, count, valueOrOffset)
			hasCompression = true
		case tagStripOffsets:
			stripOffsets = readTagValues(f, bo, typ, count, valueOrOffset, fileSize)
		case tagStripByteCounts:
			stripByteCounts = readTagValues(f, bo, typ, count, valueOrOffset, fileSize)
		case tagJPEGInterchangeFormat:
			jpegOffset = readTagValue(bo, typ, count, valueOrOffset)
		case tagJPEGInterchangeFormatLen:
			jpegLength = readTagValue(bo, typ, count, valueOrOffset)
		case tagSubIFDs:
			subIFDOffsets = readTagValues(f, bo, typ, count, valueOrOffset, fileSize)
		case tagNewSubFileType:
			// Read but we don't filter on it; we collect all JPEG previews and pick the largest.
			_ = readTagValue(bo, typ, count, valueOrOffset)
		}
	}

	nextIFDOffset := bo.Uint32(entriesBuf[entriesSize : entriesSize+4])

	var preview *jpegPreview

	isJPEGCompression := hasCompression && (compression == compressionOJPEG || compression == compressionJPEG)

	// Path 1: StripOffsets + StripByteCounts with JPEG compression.
	if isJPEGCompression && len(stripOffsets) > 0 && len(stripByteCounts) > 0 && len(stripOffsets) == len(stripByteCounts) {
		var totalSize uint32
		for _, bc := range stripByteCounts {
			totalSize += bc
		}
		if totalSize > 0 && len(stripOffsets) > 0 {
			preview = &jpegPreview{offset: stripOffsets[0], size: totalSize}
			logger.Log.Debug("tiff: found JPEG preview", "ifdIndex", index, "offset", stripOffsets[0], "size", totalSize)
		}
	}

	// Path 2: JPEGInterchangeFormat + JPEGInterchangeFormatLength.
	if jpegOffset > 0 && jpegLength > 0 {
		candidate := jpegPreview{offset: jpegOffset, size: jpegLength}
		if preview == nil || candidate.size > preview.size {
			preview = &candidate
			logger.Log.Debug("tiff: found JPEG preview", "ifdIndex", index, "offset", jpegOffset, "size", jpegLength)
		}
	}

	return preview, nextIFDOffset, subIFDOffsets, nil
}

// readTagValue reads a single scalar value from a tag entry.
func readTagValue(bo binary.ByteOrder, typ uint16, count uint32, valueOrOffset []byte) uint32 {
	if count == 0 {
		return 0
	}
	switch typ {
	case 3: // SHORT
		return uint32(bo.Uint16(valueOrOffset[0:2]))
	case 4: // LONG
		return bo.Uint32(valueOrOffset[0:4])
	case 1: // BYTE
		return uint32(valueOrOffset[0])
	default:
		return bo.Uint32(valueOrOffset[0:4])
	}
}

// readTagValues reads an array of values from a tag. If count*typeSize <= 4,
// the values are inline in the offset field; otherwise, the offset field points
// to the data location in the file.
func readTagValues(f *os.File, bo binary.ByteOrder, typ uint16, count uint32, valueOrOffset []byte, fileSize int64) []uint32 {
	if count == 0 {
		return nil
	}

	ts, ok := tiffTypeSize[typ]
	if !ok {
		ts = 4 // default to LONG size for unknown types
	}
	totalBytes := count * ts

	var data []byte
	if totalBytes <= 4 {
		// Value stored inline in the offset field.
		data = valueOrOffset[:totalBytes]
	} else {
		// Value stored at the offset pointed to by the field.
		off := bo.Uint32(valueOrOffset[0:4])
		if int64(off)+int64(totalBytes) > fileSize {
			return nil
		}
		data = make([]byte, totalBytes)
		if _, err := f.ReadAt(data, int64(off)); err != nil {
			return nil
		}
	}

	result := make([]uint32, count)
	for i := uint32(0); i < count; i++ {
		switch typ {
		case 3: // SHORT
			result[i] = uint32(bo.Uint16(data[i*2 : i*2+2]))
		case 4: // LONG
			result[i] = bo.Uint32(data[i*4 : i*4+4])
		case 1: // BYTE
			result[i] = uint32(data[i])
		default:
			result[i] = bo.Uint32(data[i*4 : i*4+4])
		}
	}
	return result
}

// isAcceptedTIFFMagic returns true if the magic number is standard TIFF (42)
// or a known camera-vendor variant (Olympus ORF, Panasonic RW2).
func isAcceptedTIFFMagic(magic uint16) bool {
	switch magic {
	case tiffMagic, orfMagicOR, orfMagicRS, rw2Magic:
		return true
	default:
		return false
	}
}
