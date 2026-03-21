package raw

import "testing"

func TestIsRAWExt(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		{".cr2", true},
		{".CR2", true},
		{".Cr2", true},
		{".cr3", true},
		{".arw", true},
		{".nef", true},
		{".dng", true},
		{".raf", true},
		{".rw2", true},
		{".orf", true},
		{".nrw", true},
		{".pef", true},
		{".srw", true},
		{".jpg", false},
		{".png", false},
		{".mp4", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsRAWExt(tt.ext); got != tt.want {
			t.Errorf("IsRAWExt(%q) = %v, want %v", tt.ext, got, tt.want)
		}
	}
}

func TestFormatName(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".cr3", "CR3"}, {".CR3", "CR3"}, {".arw", "ARW"}, {".nef", "NEF"},
	}
	for _, tt := range tests {
		if got := FormatName(tt.ext); got != tt.want {
			t.Errorf("FormatName(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func TestImageExtensions(t *testing.T) {
	exts := ImageExtensions()
	if !exts[".jpg"] || !exts[".cr3"] || !exts[".raf"] {
		t.Error("ImageExtensions missing expected extensions")
	}
	if exts[".mp4"] {
		t.Error("ImageExtensions should not include video")
	}
}
