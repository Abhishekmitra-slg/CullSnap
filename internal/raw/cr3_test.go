package raw

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// buildBMFFBox builds a BMFF box with the given type and payload.
func buildBMFFBox(boxType string, payload []byte) []byte {
	size := uint32(8 + len(payload))
	buf := make([]byte, size)
	binary.BigEndian.PutUint32(buf[0:4], size)
	copy(buf[4:8], boxType)
	copy(buf[8:], payload)
	return buf
}

// buildUUIDBox builds a BMFF uuid box with the given UUID and payload.
func buildUUIDBox(uuid [16]byte, payload []byte) []byte {
	size := uint32(8 + 16 + len(payload))
	buf := make([]byte, size)
	binary.BigEndian.PutUint32(buf[0:4], size)
	copy(buf[4:8], "uuid")
	copy(buf[8:24], uuid[:])
	copy(buf[24:], payload)
	return buf
}

// buildPRVWBox builds a PRVW sub-box with a minimal header and the given JPEG data.
func buildPRVWBox(jpegData []byte) []byte {
	// PRVW header: 14 bytes before JPEG (6 unknown + 2 width + 2 height + 2 unknown + 4 jpeg_size - but we just need SOI scan to work)
	header := make([]byte, 14)
	binary.BigEndian.PutUint16(header[4:6], 1620) // width
	binary.BigEndian.PutUint16(header[6:8], 1080) // height
	binary.BigEndian.PutUint32(header[10:14], uint32(len(jpegData)))

	header = append(header, jpegData...)
	return buildBMFFBox("PRVW", header)
}

// buildSyntheticCR3 builds a minimal synthetic CR3 file containing:
// ftyp box + uuid box (with PRVW UUID and PRVW sub-box containing jpegData).
func buildSyntheticCR3(jpegData []byte) []byte {
	ftyp := buildBMFFBox("ftyp", []byte("crx \x00\x00\x01\x00crx "))
	prvw := buildPRVWBox(jpegData)
	uuidBox := buildUUIDBox(prvwUUID, prvw)

	var file []byte
	file = append(file, ftyp...)
	file = append(file, uuidBox...)
	return file
}

// minimalJPEG is a tiny valid JPEG (SOI + EOI).
var minimalJPEG = []byte{0xFF, 0xD8, 0xFF, 0xD9}

func writeCR3TempFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExtractCR3Preview_ValidFile(t *testing.T) {
	cr3Data := buildSyntheticCR3(minimalJPEG)
	path := writeCR3TempFile(t, "test.cr3", cr3Data)

	got, err := extractCR3Preview(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should start with JPEG SOI marker
	if len(got) < 2 || got[0] != 0xFF || got[1] != 0xD8 {
		t.Fatalf("expected JPEG SOI, got %x", got[:min(4, len(got))])
	}

	// Should contain our minimal JPEG
	if len(got) != len(minimalJPEG) {
		t.Errorf("expected %d bytes, got %d", len(minimalJPEG), len(got))
	}
}

func TestExtractCR3Preview_NoPRVWBox(t *testing.T) {
	// Build a CR3 with a uuid box that has a different UUID (no PRVW)
	ftyp := buildBMFFBox("ftyp", []byte("crx \x00\x00\x01\x00crx "))
	otherUUID := [16]byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
	}
	uuidBox := buildUUIDBox(otherUUID, []byte("some other data here"))

	var cr3Data []byte
	cr3Data = append(cr3Data, ftyp...)
	cr3Data = append(cr3Data, uuidBox...)

	path := writeCR3TempFile(t, "no_prvw.cr3", cr3Data)

	_, err := extractCR3Preview(path)
	if err == nil {
		t.Fatal("expected error for file without PRVW box")
	}
}

func TestExtractCR3Preview_EmptyFile(t *testing.T) {
	path := writeCR3TempFile(t, "empty.cr3", []byte{})

	_, err := extractCR3Preview(path)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestExtractCR3Preview_TruncatedBox(t *testing.T) {
	// Create a file with a box header that claims a larger size than the file
	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header[0:4], 1000) // claims 1000 bytes
	copy(header[4:8], "ftyp")
	// But file is only 8 bytes

	path := writeCR3TempFile(t, "truncated.cr3", header)

	_, err := extractCR3Preview(path)
	if err == nil {
		t.Fatal("expected error for truncated file")
	}
}
