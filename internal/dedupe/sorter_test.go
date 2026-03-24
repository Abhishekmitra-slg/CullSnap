package dedupe

import (
	"os"
	"testing"
)

func TestIsTIFFBasedRAW(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		{".cr2", true},
		{".nef", true},
		{".arw", true},
		{".dng", true},
		{".orf", false},
		{".rw2", false},
		{".raf", false},
		{".pef", false},
		{".cr3", false},
		{".jpg", false},
		{".png", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := isTIFFBasedRAW(tt.ext)
			if got != tt.want {
				t.Errorf("isTIFFBasedRAW(%q) = %v, want %v", tt.ext, got, tt.want)
			}
		})
	}
}

func TestExtractDateTaken_TextFile(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/test.txt"
	if err := os.WriteFile(path, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, ok := ExtractDateTaken(path)
	if ok {
		t.Error("expected false for text file")
	}
}

func TestExtractFullEXIF_TextFile(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/test.txt"
	if err := os.WriteFile(path, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ExtractFullEXIF(path)
	if err == nil {
		t.Error("expected error for text file")
	}
}

func TestFullEXIF_ZeroValue(t *testing.T) {
	e := FullEXIF{}
	if e.Camera != "" {
		t.Error("zero FullEXIF should have empty Camera")
	}
	if e.DateTaken != "" {
		t.Error("zero FullEXIF should have empty DateTaken")
	}
}

func TestPhotoMeta_ZeroValue(t *testing.T) {
	m := PhotoMeta{}
	if m.HasDate {
		t.Error("zero PhotoMeta should have HasDate=false")
	}
	if m.CameraMake != "" {
		t.Error("zero PhotoMeta should have empty CameraMake")
	}
	if m.Group != nil {
		t.Error("zero PhotoMeta should have nil Group")
	}
}
