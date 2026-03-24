package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanDirectory_HEIC(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "photo1.heic"), []byte("fake"), 0644)
	os.WriteFile(filepath.Join(dir, "photo2.heif"), []byte("fake"), 0644)
	os.WriteFile(filepath.Join(dir, "photo3.jpg"), []byte("fake"), 0644)

	photos, err := ScanDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(photos) != 3 {
		t.Fatalf("expected 3 photos, got %d", len(photos))
	}

	for _, p := range photos {
		ext := filepath.Ext(p.Path)
		if ext == ".heic" || ext == ".heif" {
			if p.IsRAW {
				t.Errorf("HEIC file %s should not be marked as RAW", p.Path)
			}
		}
	}
}
