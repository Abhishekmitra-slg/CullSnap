package heic

import (
	"cullsnap/internal/logger"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	logger.Init("/dev/null") //nolint:errcheck // test init
	os.Exit(m.Run())
}

func TestConvertToJPEG_HeifExtension(t *testing.T) {
	// Verify .heif extension is accepted (not just .heic)
	tmp := t.TempDir()
	src := filepath.Join(tmp, "test.heif")
	if err := os.WriteFile(src, []byte("fake heif"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Will fail at the conversion step (not a real HEIF file),
	// but should not fail at the extension check
	err := ConvertToJPEG(src, filepath.Join(tmp, "out.jpg"), false)
	if err == nil {
		t.Log("ConvertToJPEG succeeded for fake .heif (unexpected but ok)")
	}
	// The error should NOT be about unsupported extension
	if err != nil && err.Error() == `heic: unsupported extension ".heif"` {
		t.Error("should not reject .heif extension")
	}
}

func TestConvertToJPEG_OutputDirCreation(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "test.heic")
	if err := os.WriteFile(src, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(tmp, "nested", "dir")
	out := filepath.Join(outDir, "out.jpg")

	// Should not fail because output dir doesn't exist (it should create it)
	// Will fail at actual conversion (no real HEIC data or ffmpeg)
	_ = ConvertToJPEG(src, out, false)

	// Verify the output directory was created
	info, err := os.Stat(outDir)
	if err != nil {
		t.Fatalf("output dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestConvertFFmpeg_NoFFmpeg(t *testing.T) {
	tmp := t.TempDir()
	err := convertFFmpeg(filepath.Join(tmp, "test.heic"), filepath.Join(tmp, "out.jpg"))
	if err == nil {
		t.Error("expected error when ffmpeg is not available")
	}
}
