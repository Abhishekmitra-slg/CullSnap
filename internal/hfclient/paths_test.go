package hfclient

import "testing"

func TestValidateSiblingPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		ok   bool
	}{
		{"normal", "model.safetensors", true},
		{"nested", "subdir/file.json", true},
		{"empty", "", false},
		{"leading slash", "/etc/passwd", false},
		{"backslash leading", `\etc\passwd`, false},
		{"parent traversal", "../../etc/passwd", false},
		{"hidden parent", "a/../b", false},
		{"null byte", "good\x00bad", false},
		{"colon", "C:\\path", false},
		{"backslash", `a\b`, false},
		{"non-canonical trailing", "a/b/", false},
		{"non-canonical double slash", "a//b", false},
		{"absolute", "/abs/path", false},
		{"hidden dotdot in middle", "a/..b", true},
		{"named dotdot", ".gitattributes", true},
		{"deep nested", "a/b/c/d/e.json", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSiblingPath(tt.in)
			if (err == nil) != tt.ok {
				t.Fatalf("validateSiblingPath(%q): err=%v want ok=%v", tt.in, err, tt.ok)
			}
		})
	}
}
