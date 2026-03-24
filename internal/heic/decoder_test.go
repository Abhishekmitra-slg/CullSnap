package heic

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConvertToJPEG_MissingInput(t *testing.T) {
	err := ConvertToJPEG("/nonexistent/file.heic", "/tmp/out.jpg", false)
	if err == nil {
		t.Fatal("expected error for missing input file")
	}
}

func TestConvertToJPEG_InvalidExtension(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "test.txt")
	os.WriteFile(tmp, []byte("not heic"), 0o644) //nolint:errcheck // test helper
	err := ConvertToJPEG(tmp, filepath.Join(t.TempDir(), "out.jpg"), false)
	if err == nil {
		t.Fatal("expected error for non-HEIC input")
	}
}

func TestSanitizePathForExec(t *testing.T) {
	dangerous := []string{
		"photo; rm -rf /.heic",
		"photo$(whoami).heic",
		"photo`id`.heic",
		"photo\ninjection.heic",
	}
	for _, name := range dangerous {
		path := filepath.Join(t.TempDir(), name)
		err := ConvertToJPEG(path, filepath.Join(t.TempDir(), "out.jpg"), false)
		if err == nil {
			t.Errorf("expected error for dangerous filename: %s", name)
		}
	}
}
