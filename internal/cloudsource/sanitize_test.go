package cloudsource

import (
	"path"
	"testing"
)

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abc123", "abc123"},
		{"abc-def_ghi.jpg", "abc-def_ghi.jpg"},
		{"../../../etc/passwd", ".._.._.._etc_passwd"},
		{"/etc/passwd", "_etc_passwd"},
		{"file\x00name", "file_name"},
		{"hello world", "hello_world"},
		{"", "_"},
		{"..", "_"},
		{".", "_"},
		{"../..", ".._.."},
	}
	for _, tt := range tests {
		got := SanitizeID(tt.input)
		if got != tt.expected {
			t.Errorf("SanitizeID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
		if got != path.Base(got) {
			t.Errorf("SanitizeID(%q) = %q contains directory separator", tt.input, got)
		}
	}
}

func TestSanitizeID_NoTraversal(t *testing.T) {
	dangerous := []string{
		"../../etc/passwd",
		"..\\..\\windows\\system32",
		"/absolute/path",
		"normal/../traversal",
	}
	for _, input := range dangerous {
		got := SanitizeID(input)
		if got != path.Base(got) {
			t.Errorf("SanitizeID(%q) = %q still contains path separator", input, got)
		}
	}
}
