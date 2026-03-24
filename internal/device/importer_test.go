package device

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeSerial(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"normal serial", "abc123def456", "abc123def456"},
		{"empty string", "", "_"},
		{"with spaces", "abc 123 def", "abc_123_def"},
		{"path traversal", "../../../etc/passwd", ".._.._.._etc_passwd"},
		{"dots and dashes kept", "abc-123.xyz", "abc-123.xyz"},
		{"special chars stripped", "abc$%^&*()123", "abc_______123"},
		{"unicode chars", "abc\u00e9\u00f1\u00fc123", "abc___123"},
		{"slashes", "abc/def\\ghi", "abc_def_ghi"},
		{"all special", "!@#$%^", "______"},
		{"underscores preserved", "abc_123_def", "abc_123_def"},
		{"leading dot", ".hidden", ".hidden"},
		{"double dots", "abc..def", "abc..def"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeSerial(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeSerial(%q) = %q, want %q", tt.input, got, tt.want)
			}

			// The result should never contain path separators.
			if filepath.Base(got) != got {
				t.Errorf("SanitizeSerial(%q) = %q contains path separator", tt.input, got)
			}
		})
	}
}

func TestCountFiles(t *testing.T) {
	dir := t.TempDir()

	// Empty dir.
	if got := countFiles(dir); got != 0 {
		t.Errorf("countFiles(empty) = %d, want 0", got)
	}

	// Create some files and a subdirectory.
	for _, name := range []string{"a.jpg", "b.png", "c.heic"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := countFiles(dir); got != 3 {
		t.Errorf("countFiles = %d, want 3", got)
	}
}

func TestCountFiles_NonexistentDir(t *testing.T) {
	if got := countFiles("/nonexistent/path/that/does/not/exist"); got != 0 {
		t.Errorf("countFiles(nonexistent) = %d, want 0", got)
	}
}
