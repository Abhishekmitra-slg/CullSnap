package raw

import (
	"cullsnap/internal/logger"
	"cullsnap/internal/model"
	"log/slog"
	"os"
	"testing"
)

func initTestLogger() {
	if logger.Log == nil {
		logger.Log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
}

func TestPairRAWJPEG_SameBaseNameSameDir(t *testing.T) {
	initTestLogger()

	photos := []model.Photo{
		{Path: "/photos/IMG_001.cr3"},
		{Path: "/photos/IMG_001.jpg"},
	}

	result := PairRAWJPEG(photos)

	if result[0].CompanionPath != "/photos/IMG_001.jpg" {
		t.Errorf("RAW CompanionPath = %q, want %q", result[0].CompanionPath, "/photos/IMG_001.jpg")
	}
	if result[1].CompanionPath != "/photos/IMG_001.cr3" {
		t.Errorf("JPEG CompanionPath = %q, want %q", result[1].CompanionPath, "/photos/IMG_001.cr3")
	}
	if !result[1].IsRAWCompanion {
		t.Error("JPEG should be marked as IsRAWCompanion")
	}
	if result[0].IsRAWCompanion {
		t.Error("RAW should NOT be marked as IsRAWCompanion")
	}
}

func TestPairRAWJPEG_SameBaseNameDifferentDir(t *testing.T) {
	initTestLogger()

	photos := []model.Photo{
		{Path: "/photos/dir1/IMG_001.cr3"},
		{Path: "/photos/dir2/IMG_001.jpg"},
	}

	result := PairRAWJPEG(photos)

	if result[0].CompanionPath != "" {
		t.Errorf("RAW CompanionPath = %q, want empty (different dirs)", result[0].CompanionPath)
	}
	if result[1].CompanionPath != "" {
		t.Errorf("JPEG CompanionPath = %q, want empty (different dirs)", result[1].CompanionPath)
	}
}

func TestPairRAWJPEG_RAWWithoutCompanion(t *testing.T) {
	initTestLogger()

	photos := []model.Photo{
		{Path: "/photos/IMG_001.cr3"},
	}

	result := PairRAWJPEG(photos)

	if result[0].CompanionPath != "" {
		t.Errorf("RAW CompanionPath = %q, want empty (no companion)", result[0].CompanionPath)
	}
}

func TestPairRAWJPEG_JPEGWithoutCompanion(t *testing.T) {
	initTestLogger()

	photos := []model.Photo{
		{Path: "/photos/IMG_001.jpg"},
	}

	result := PairRAWJPEG(photos)

	if result[0].CompanionPath != "" {
		t.Errorf("JPEG CompanionPath = %q, want empty (no companion)", result[0].CompanionPath)
	}
	if result[0].IsRAWCompanion {
		t.Error("standalone JPEG should NOT be marked as IsRAWCompanion")
	}
}

func TestPairRAWJPEG_CaseInsensitive(t *testing.T) {
	initTestLogger()

	photos := []model.Photo{
		{Path: "/photos/img_001.cr3"},
		{Path: "/photos/IMG_001.JPG"},
	}

	result := PairRAWJPEG(photos)

	if result[0].CompanionPath != "/photos/IMG_001.JPG" {
		t.Errorf("RAW CompanionPath = %q, want %q", result[0].CompanionPath, "/photos/IMG_001.JPG")
	}
	if result[1].CompanionPath != "/photos/img_001.cr3" {
		t.Errorf("JPEG CompanionPath = %q, want %q", result[1].CompanionPath, "/photos/img_001.cr3")
	}
	if !result[1].IsRAWCompanion {
		t.Error("JPEG should be marked as IsRAWCompanion (case-insensitive match)")
	}
}

func TestPairRAWJPEG_MultipleFormats(t *testing.T) {
	initTestLogger()

	photos := []model.Photo{
		{Path: "/photos/IMG_001.cr3"},
		{Path: "/photos/IMG_001.jpg"},
		{Path: "/photos/IMG_002.arw"},
		{Path: "/photos/IMG_002.jpeg"},
		{Path: "/photos/IMG_003.nef"},
	}

	result := PairRAWJPEG(photos)

	// CR3+JPG pair
	if result[0].CompanionPath != "/photos/IMG_001.jpg" {
		t.Errorf("CR3 CompanionPath = %q, want %q", result[0].CompanionPath, "/photos/IMG_001.jpg")
	}
	if !result[1].IsRAWCompanion {
		t.Error("JPG companion of CR3 should be marked as IsRAWCompanion")
	}

	// ARW+JPEG pair
	if result[2].CompanionPath != "/photos/IMG_002.jpeg" {
		t.Errorf("ARW CompanionPath = %q, want %q", result[2].CompanionPath, "/photos/IMG_002.jpeg")
	}
	if !result[3].IsRAWCompanion {
		t.Error("JPEG companion of ARW should be marked as IsRAWCompanion")
	}

	// NEF alone
	if result[4].CompanionPath != "" {
		t.Errorf("NEF CompanionPath = %q, want empty (no companion)", result[4].CompanionPath)
	}
}

func TestPairRAWJPEG_JpegExtensions(t *testing.T) {
	initTestLogger()

	photos := []model.Photo{
		{Path: "/photos/IMG_001.arw"},
		{Path: "/photos/IMG_001.jpeg"},
	}

	result := PairRAWJPEG(photos)

	if result[0].CompanionPath != "/photos/IMG_001.jpeg" {
		t.Errorf("ARW CompanionPath = %q, want %q", result[0].CompanionPath, "/photos/IMG_001.jpeg")
	}
	if !result[1].IsRAWCompanion {
		t.Error(".jpeg file should be marked as IsRAWCompanion")
	}
}

func TestPairRAWJPEG_EmptyList(t *testing.T) {
	initTestLogger()

	result := PairRAWJPEG([]model.Photo{})

	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
}
