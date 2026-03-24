package image

import (
	"cullsnap/internal/logger"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateThumbnail_HEIC_Routing(t *testing.T) {
	logger.Init("/dev/null")

	tc := &ThumbCache{
		cacheDir:      t.TempDir(),
		useNativeSips: false,
	}
	heicFile := filepath.Join(t.TempDir(), "test.heic")
	if err := os.WriteFile(heicFile, []byte("not real heic"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := tc.GenerateThumbnail(heicFile, time.Now())
	if err == nil {
		t.Fatal("expected error for fake HEIC file")
	}
	// Verify the error comes from the HEIC conversion path, not the generic photo path
	if !strings.Contains(err.Error(), "HEIC") && !strings.Contains(err.Error(), "heic") {
		t.Errorf("expected HEIC-related error, got: %v", err)
	}
}
