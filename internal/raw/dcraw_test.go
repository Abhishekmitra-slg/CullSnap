package raw

import (
	"testing"
)

func TestExtractPreviewDcraw_NotAvailable(t *testing.T) {
	// Save and restore state.
	origAvailable := dcrawAvailable
	dcrawAvailable = false
	defer func() { dcrawAvailable = origAvailable }()

	_, err := ExtractPreviewDcraw("/some/file.raf")
	if err == nil {
		t.Fatal("expected error when dcraw not available")
	}
}

func TestInit_GracefulWhenDownloadFails(t *testing.T) {
	origPath := dcrawPath
	origAvailable := dcrawAvailable
	defer func() {
		dcrawPath = origPath
		dcrawAvailable = origAvailable
	}()

	dcrawPath = "/nonexistent/path/dcraw"
	dcrawAvailable = false

	err := Init()
	if err != nil {
		t.Fatalf("Init should not error on download failure: %v", err)
	}
}
