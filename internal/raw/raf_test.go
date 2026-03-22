package raw

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// buildMinimalRAF creates a minimal RAF file with an embedded JPEG preview.
func buildMinimalRAF(jpegData []byte) []byte {
	// Header: 16 (magic) + 4 (version) + 8 (camera ID) + 32 (camera string) = 60 bytes
	// Offset directory: 4 (version) + 20 (unknown) + 4 (jpeg offset) + 4 (jpeg length) = 32 bytes
	// Total header+dir = 92 bytes, then JPEG data follows.
	headerSize := 92
	totalSize := headerSize + len(jpegData)
	buf := make([]byte, totalSize)

	// Magic.
	copy(buf[0:16], rafMagic)

	// Format version.
	copy(buf[16:20], "0201")

	// Camera ID (8 bytes) — leave zeroed.
	// Camera string (32 bytes).
	copy(buf[28:60], "Fujifilm X-T4\x00")

	// Offset directory version.
	copy(buf[60:64], "0100")

	// Unknown 20 bytes (64-83) — leave zeroed.

	// JPEG offset (big-endian).
	binary.BigEndian.PutUint32(buf[84:88], uint32(headerSize))

	// JPEG length (big-endian).
	binary.BigEndian.PutUint32(buf[88:92], uint32(len(jpegData)))

	// JPEG data.
	copy(buf[headerSize:], jpegData)

	return buf
}

func writeRAFTempFile(t *testing.T, data []byte, name string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExtractRAFPreview_Valid(t *testing.T) {
	jpegData := fakeJPEG(500)
	rafData := buildMinimalRAF(jpegData)
	path := writeRAFTempFile(t, rafData, "test.raf")

	result, err := extractRAFPreview(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 500 {
		t.Fatalf("expected 500 bytes, got %d", len(result))
	}
	if result[0] != 0xFF || result[1] != 0xD8 {
		t.Fatal("missing JPEG SOI marker")
	}
}

func TestExtractRAFPreview_RealJPEG(t *testing.T) {
	jpegData := buildValidJPEG(t, 800, 600)
	rafData := buildMinimalRAF(jpegData)
	path := writeRAFTempFile(t, rafData, "test.raf")

	result, err := extractRAFPreview(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0] != 0xFF || result[1] != 0xD8 {
		t.Fatal("missing JPEG SOI marker")
	}
}

func TestExtractRAFPreview_InvalidMagic(t *testing.T) {
	data := make([]byte, 100)
	copy(data[0:16], "NOT_A_RAF_FILE!!")
	path := writeRAFTempFile(t, data, "bad.raf")

	_, err := extractRAFPreview(path)
	if err == nil {
		t.Fatal("expected error for invalid magic")
	}
}

func TestExtractRAFPreview_FileTooSmall(t *testing.T) {
	data := []byte("FUJIFILMCCD-RAW ") // just the magic, nothing else
	path := writeRAFTempFile(t, data, "tiny.raf")

	_, err := extractRAFPreview(path)
	if err == nil {
		t.Fatal("expected error for file too small")
	}
}

func TestExtractRAFPreview_ZeroOffset(t *testing.T) {
	// Build a valid RAF header but with JPEG offset = 0.
	buf := make([]byte, 92)
	copy(buf[0:16], rafMagic)
	copy(buf[16:20], "0201")
	// JPEG offset and length both zero.
	path := writeRAFTempFile(t, buf, "zero.raf")

	_, err := extractRAFPreview(path)
	if err == nil {
		t.Fatal("expected error for zero JPEG offset")
	}
}

func TestExtractRAFPreview_OffsetBeyondEOF(t *testing.T) {
	buf := make([]byte, 92)
	copy(buf[0:16], rafMagic)
	copy(buf[16:20], "0201")
	binary.BigEndian.PutUint32(buf[84:88], 999999) // offset way past EOF
	binary.BigEndian.PutUint32(buf[88:92], 100)
	path := writeRAFTempFile(t, buf, "oob.raf")

	_, err := extractRAFPreview(path)
	if err == nil {
		t.Fatal("expected error for offset beyond EOF")
	}
}

func TestExtractRAFPreview_NotJPEGData(t *testing.T) {
	// Valid header pointing to non-JPEG data.
	headerSize := 92
	badData := make([]byte, 50)
	badData[0] = 0x00
	badData[1] = 0x00

	buf := make([]byte, headerSize+len(badData))
	copy(buf[0:16], rafMagic)
	copy(buf[16:20], "0201")
	binary.BigEndian.PutUint32(buf[84:88], uint32(headerSize))
	binary.BigEndian.PutUint32(buf[88:92], uint32(len(badData)))
	copy(buf[headerSize:], badData)
	path := writeRAFTempFile(t, buf, "notjpeg.raf")

	_, err := extractRAFPreview(path)
	if err == nil {
		t.Fatal("expected error for non-JPEG data")
	}
}

func TestNullTermString(t *testing.T) {
	tests := []struct {
		input    []byte
		expected string
	}{
		{[]byte("Hello\x00World"), "Hello"},
		{[]byte("NoNull"), "NoNull"},
		{[]byte("\x00"), ""},
		{[]byte{}, ""},
	}

	for _, tt := range tests {
		got := nullTermString(tt.input)
		if got != tt.expected {
			t.Errorf("nullTermString(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
