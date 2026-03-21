package raw

import (
	"cullsnap/internal/logger"
	"encoding/binary"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func init() {
	// Initialize logger for tests so Debug calls don't panic.
	if logger.Log == nil {
		logger.Log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
}

// --- Test helper functions ---

// writeTIFFHeader writes an 8-byte TIFF header into buf at position 0.
func writeTIFFHeader(buf []byte, bo binary.ByteOrder, magic uint16, ifd0Offset uint32) {
	if bo == binary.LittleEndian {
		buf[0], buf[1] = 'I', 'I'
	} else {
		buf[0], buf[1] = 'M', 'M'
	}
	bo.PutUint16(buf[2:4], magic)
	bo.PutUint32(buf[4:8], ifd0Offset)
}

// writeIFDEntry writes a single 12-byte IFD entry into buf.
func writeIFDEntry(buf []byte, bo binary.ByteOrder, tag, typ uint16, count, value uint32) {
	bo.PutUint16(buf[0:2], tag)
	bo.PutUint16(buf[2:4], typ)
	bo.PutUint32(buf[4:8], count)
	bo.PutUint32(buf[8:12], value)
}

// writeIFDEntryShortInline writes an IFD entry with a SHORT value inline.
func writeIFDEntryShortInline(buf []byte, bo binary.ByteOrder, tag uint16, value uint16) {
	bo.PutUint16(buf[0:2], tag)
	bo.PutUint16(buf[2:4], 3) // SHORT
	bo.PutUint32(buf[4:8], 1) // count=1
	// SHORT value in first 2 bytes of value field, rest zero.
	bo.PutUint16(buf[8:10], value)
	buf[10] = 0
	buf[11] = 0
}

// fakeJPEG returns fake JPEG data starting with SOI marker.
func fakeJPEG(size int) []byte {
	data := make([]byte, size)
	data[0] = 0xFF
	data[1] = 0xD8
	// Fill the rest with non-zero data.
	for i := 2; i < size; i++ {
		data[i] = byte(i % 251)
	}
	return data
}

// buildMinimalTIFF creates a minimal TIFF with one IFD containing a JPEG preview.
// Returns the file bytes. The IFD has Compression=7, StripOffsets, StripByteCounts.
func buildMinimalTIFF(bo binary.ByteOrder, jpegSize int) []byte {
	jpeg := fakeJPEG(jpegSize)

	// Layout: header(8) + IFD(2 + 3*12 + 4) + jpeg data
	ifdOffset := uint32(8)
	numEntries := uint16(3)
	ifdSize := 2 + int(numEntries)*12 + 4
	jpegDataOffset := uint32(8 + ifdSize)

	totalSize := int(jpegDataOffset) + jpegSize
	buf := make([]byte, totalSize)

	// Header.
	writeTIFFHeader(buf, bo, tiffMagic, ifdOffset)

	// IFD at offset 8.
	pos := int(ifdOffset)
	bo.PutUint16(buf[pos:pos+2], numEntries)
	pos += 2

	// Entry 0: Compression = 7 (JPEG), SHORT.
	writeIFDEntryShortInline(buf[pos:pos+12], bo, tagCompression, compressionJPEG)
	pos += 12

	// Entry 1: StripOffsets = jpegDataOffset, LONG.
	writeIFDEntry(buf[pos:pos+12], bo, tagStripOffsets, 4, 1, jpegDataOffset)
	pos += 12

	// Entry 2: StripByteCounts = jpegSize, LONG.
	writeIFDEntry(buf[pos:pos+12], bo, tagStripByteCounts, 4, 1, uint32(jpegSize))
	pos += 12

	// Next IFD = 0 (end of chain).
	bo.PutUint32(buf[pos:pos+4], 0)

	// JPEG data.
	copy(buf[jpegDataOffset:], jpeg)

	return buf
}

func writeTempFile(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "test.tiff")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// --- Tests ---

func TestParseTIFF_LittleEndian(t *testing.T) {
	data := buildMinimalTIFF(binary.LittleEndian, 100)
	path := writeTempFile(t, data)

	result, err := extractTIFFPreview(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 100 {
		t.Fatalf("expected 100 bytes, got %d", len(result))
	}
	if result[0] != 0xFF || result[1] != 0xD8 {
		t.Fatal("missing JPEG SOI marker")
	}
}

func TestParseTIFF_BigEndian(t *testing.T) {
	data := buildMinimalTIFF(binary.BigEndian, 200)
	path := writeTempFile(t, data)

	result, err := extractTIFFPreview(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 200 {
		t.Fatalf("expected 200 bytes, got %d", len(result))
	}
	if result[0] != 0xFF || result[1] != 0xD8 {
		t.Fatal("missing JPEG SOI marker")
	}
}

func TestParseTIFF_BigTIFF_Rejected(t *testing.T) {
	buf := make([]byte, 16)
	writeTIFFHeader(buf, binary.LittleEndian, bigTIFF, 8)
	path := writeTempFile(t, buf)

	_, err := extractTIFFPreview(path)
	if err == nil {
		t.Fatal("expected error for BigTIFF")
	}
	if got := err.Error(); got != "tiff: BigTIFF (magic=43) is not supported" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestParseTIFF_CircularChain(t *testing.T) {
	// Two IFDs that point to each other.
	bo := binary.LittleEndian
	jpeg := fakeJPEG(50)

	// Layout: header(8) + IFD0(2+3*12+4=42) at 8 + IFD1(2+3*12+4=42) at 50 + jpeg at 92
	ifd0 := uint32(8)
	ifd1 := uint32(50)
	jpegOff := uint32(92)

	totalSize := int(jpegOff) + 50
	buf := make([]byte, totalSize)
	writeTIFFHeader(buf, bo, tiffMagic, ifd0)

	// IFD0: 3 entries, next -> IFD1.
	pos := int(ifd0)
	bo.PutUint16(buf[pos:], 3)
	pos += 2
	writeIFDEntryShortInline(buf[pos:], bo, tagCompression, compressionJPEG)
	pos += 12
	writeIFDEntry(buf[pos:], bo, tagStripOffsets, 4, 1, jpegOff)
	pos += 12
	writeIFDEntry(buf[pos:], bo, tagStripByteCounts, 4, 1, 50)
	pos += 12
	bo.PutUint32(buf[pos:], ifd1) // next -> IFD1

	// IFD1: 3 entries, next -> IFD0 (circular!).
	pos = int(ifd1)
	bo.PutUint16(buf[pos:], 3)
	pos += 2
	writeIFDEntryShortInline(buf[pos:], bo, tagCompression, compressionJPEG)
	pos += 12
	writeIFDEntry(buf[pos:], bo, tagStripOffsets, 4, 1, jpegOff)
	pos += 12
	writeIFDEntry(buf[pos:], bo, tagStripByteCounts, 4, 1, 50)
	pos += 12
	bo.PutUint32(buf[pos:], ifd0) // next -> IFD0 (circular)

	copy(buf[jpegOff:], jpeg)

	path := writeTempFile(t, buf)
	result, err := extractTIFFPreview(path)
	if err != nil {
		t.Fatalf("unexpected error (should handle circular chain): %v", err)
	}
	if len(result) != 50 {
		t.Fatalf("expected 50 bytes, got %d", len(result))
	}
}

func TestParseTIFF_OffsetBeyondEOF(t *testing.T) {
	bo := binary.LittleEndian
	// IFD with StripOffsets pointing way past EOF.
	ifdOffset := uint32(8)
	numEntries := uint16(3)
	ifdSize := 2 + int(numEntries)*12 + 4
	totalSize := 8 + ifdSize
	buf := make([]byte, totalSize)

	writeTIFFHeader(buf, bo, tiffMagic, ifdOffset)
	pos := int(ifdOffset)
	bo.PutUint16(buf[pos:], numEntries)
	pos += 2
	writeIFDEntryShortInline(buf[pos:], bo, tagCompression, compressionJPEG)
	pos += 12
	writeIFDEntry(buf[pos:], bo, tagStripOffsets, 4, 1, 999999) // way past EOF
	pos += 12
	writeIFDEntry(buf[pos:], bo, tagStripByteCounts, 4, 1, 100)
	pos += 12
	bo.PutUint32(buf[pos:], 0)

	path := writeTempFile(t, buf)
	_, err := extractTIFFPreview(path)
	if err == nil {
		t.Fatal("expected error for offset beyond EOF")
	}
}

func TestParseTIFF_EmptyFile(t *testing.T) {
	path := writeTempFile(t, []byte{})
	_, err := extractTIFFPreview(path)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestParseTIFF_TruncatedHeader(t *testing.T) {
	path := writeTempFile(t, []byte{0x49, 0x49, 0x2A, 0x00})
	_, err := extractTIFFPreview(path)
	if err == nil {
		t.Fatal("expected error for truncated header")
	}
}

func TestParseTIFF_ValueInOffsetField(t *testing.T) {
	// Create a TIFF where Compression is a SHORT stored inline (count*typeSize=2 <= 4).
	// This is the normal case for Compression, but let's also test StripByteCounts as SHORT inline.
	bo := binary.LittleEndian
	jpeg := fakeJPEG(30)

	ifdOffset := uint32(8)
	numEntries := uint16(3)
	ifdSize := 2 + int(numEntries)*12 + 4
	jpegOff := uint32(8 + ifdSize)
	totalSize := int(jpegOff) + 30
	buf := make([]byte, totalSize)

	writeTIFFHeader(buf, bo, tiffMagic, ifdOffset)
	pos := int(ifdOffset)
	bo.PutUint16(buf[pos:], numEntries)
	pos += 2

	// Compression as SHORT inline.
	writeIFDEntryShortInline(buf[pos:], bo, tagCompression, compressionJPEG)
	pos += 12

	// StripOffsets as LONG inline.
	writeIFDEntry(buf[pos:], bo, tagStripOffsets, 4, 1, jpegOff)
	pos += 12

	// StripByteCounts as SHORT inline (count=1, type=SHORT, value fits in 4 bytes).
	writeIFDEntryShortInline(buf[pos:], bo, tagStripByteCounts, 30)
	pos += 12

	bo.PutUint32(buf[pos:], 0)
	copy(buf[jpegOff:], jpeg)

	path := writeTempFile(t, buf)
	result, err := extractTIFFPreview(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 30 {
		t.Fatalf("expected 30 bytes, got %d", len(result))
	}
}

func TestParseTIFF_MultipleStrips(t *testing.T) {
	bo := binary.LittleEndian

	// Two strips of JPEG data. First strip has SOI, second has more data.
	strip1 := fakeJPEG(40) // starts with 0xFFD8
	strip2 := make([]byte, 60)
	for i := range strip2 {
		strip2[i] = byte((i + 100) % 251)
	}

	// Layout: header(8) + IFD(2+3*12+4=42) at 8
	// Strip offsets array (2 LONGs = 8 bytes) at 50
	// Strip byte counts array (2 LONGs = 8 bytes) at 58
	// Strip1 data at 66, Strip2 data at 106
	ifdOffset := uint32(8)
	numEntries := uint16(3)
	ifdSize := 2 + int(numEntries)*12 + 4

	arraysStart := uint32(8 + ifdSize)
	stripOffsetsArr := arraysStart
	stripByteCountsArr := arraysStart + 8
	strip1Off := arraysStart + 16
	strip2Off := strip1Off + 40

	totalSize := int(strip2Off) + 60
	buf := make([]byte, totalSize)

	writeTIFFHeader(buf, bo, tiffMagic, ifdOffset)
	pos := int(ifdOffset)
	bo.PutUint16(buf[pos:], numEntries)
	pos += 2

	writeIFDEntryShortInline(buf[pos:], bo, tagCompression, compressionJPEG)
	pos += 12

	// StripOffsets: count=2, type=LONG, offset -> stripOffsetsArr.
	writeIFDEntry(buf[pos:], bo, tagStripOffsets, 4, 2, stripOffsetsArr)
	pos += 12

	// StripByteCounts: count=2, type=LONG, offset -> stripByteCountsArr.
	writeIFDEntry(buf[pos:], bo, tagStripByteCounts, 4, 2, stripByteCountsArr)
	pos += 12

	bo.PutUint32(buf[pos:], 0) // no next IFD

	// Write strip offsets array.
	bo.PutUint32(buf[stripOffsetsArr:], strip1Off)
	bo.PutUint32(buf[stripOffsetsArr+4:], strip2Off)

	// Write strip byte counts array.
	bo.PutUint32(buf[stripByteCountsArr:], 40)
	bo.PutUint32(buf[stripByteCountsArr+4:], 60)

	// Write strip data.
	copy(buf[strip1Off:], strip1)
	copy(buf[strip2Off:], strip2)

	path := writeTempFile(t, buf)
	result, err := extractTIFFPreview(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// For multiple strips, the parser uses stripOffsets[0] as offset and total size.
	// The result should be 100 bytes starting from strip1Off.
	if len(result) != 100 {
		t.Fatalf("expected 100 bytes (total strips), got %d", len(result))
	}
	// First 2 bytes should be JPEG SOI.
	if result[0] != 0xFF || result[1] != 0xD8 {
		t.Fatal("missing JPEG SOI marker")
	}
}

func TestParseTIFF_SubIFDArrays(t *testing.T) {
	bo := binary.LittleEndian
	jpeg := fakeJPEG(80)

	// Layout:
	// header(8) at 0
	// IFD0(2+1*12+4=18) at 8 — has SubIFDs tag pointing to subIFD offset array
	// SubIFD offset array (1 LONG = 4 bytes) at 26
	// SubIFD(2+3*12+4=42) at 30
	// JPEG data at 72

	ifd0Offset := uint32(8)
	subIFDArrOffset := uint32(26)
	subIFDOffset := uint32(30)
	jpegOff := uint32(72)

	totalSize := int(jpegOff) + 80
	buf := make([]byte, totalSize)

	writeTIFFHeader(buf, bo, tiffMagic, ifd0Offset)

	// IFD0: 1 entry (SubIFDs tag).
	pos := int(ifd0Offset)
	bo.PutUint16(buf[pos:], 1)
	pos += 2
	// SubIFDs: count=1, type=LONG(4), value=subIFDArrOffset.
	// count=1, LONG fits in 4 bytes, so value is inline.
	writeIFDEntry(buf[pos:], bo, tagSubIFDs, 4, 1, subIFDOffset)
	pos += 12
	bo.PutUint32(buf[pos:], 0) // no next IFD

	// SubIFD offset array (not actually needed since count=1 stores inline).
	bo.PutUint32(buf[subIFDArrOffset:], subIFDOffset)

	// SubIFD: 3 entries with JPEG preview.
	pos = int(subIFDOffset)
	bo.PutUint16(buf[pos:], 3)
	pos += 2
	writeIFDEntryShortInline(buf[pos:], bo, tagCompression, compressionJPEG)
	pos += 12
	writeIFDEntry(buf[pos:], bo, tagStripOffsets, 4, 1, jpegOff)
	pos += 12
	writeIFDEntry(buf[pos:], bo, tagStripByteCounts, 4, 1, 80)
	pos += 12
	bo.PutUint32(buf[pos:], 0) // no next IFD

	copy(buf[jpegOff:], jpeg)

	path := writeTempFile(t, buf)
	result, err := extractTIFFPreview(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 80 {
		t.Fatalf("expected 80 bytes, got %d", len(result))
	}
}

func TestParseTIFF_JPEGInterchangeFormat(t *testing.T) {
	bo := binary.LittleEndian
	jpeg := fakeJPEG(150)

	ifdOffset := uint32(8)
	numEntries := uint16(2)
	ifdSize := 2 + int(numEntries)*12 + 4
	jpegOff := uint32(8 + ifdSize)
	totalSize := int(jpegOff) + 150
	buf := make([]byte, totalSize)

	writeTIFFHeader(buf, bo, tiffMagic, ifdOffset)
	pos := int(ifdOffset)
	bo.PutUint16(buf[pos:], numEntries)
	pos += 2

	// JPEGInterchangeFormat.
	writeIFDEntry(buf[pos:], bo, tagJPEGInterchangeFormat, 4, 1, jpegOff)
	pos += 12

	// JPEGInterchangeFormatLength.
	writeIFDEntry(buf[pos:], bo, tagJPEGInterchangeFormatLen, 4, 1, 150)
	pos += 12

	bo.PutUint32(buf[pos:], 0)
	copy(buf[jpegOff:], jpeg)

	path := writeTempFile(t, buf)
	result, err := extractTIFFPreview(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 150 {
		t.Fatalf("expected 150 bytes, got %d", len(result))
	}
	if result[0] != 0xFF || result[1] != 0xD8 {
		t.Fatal("missing JPEG SOI marker")
	}
}
