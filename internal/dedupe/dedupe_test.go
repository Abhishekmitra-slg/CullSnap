package dedupe

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// createMinimalJPEG writes a 1x1 JPEG (no EXIF) to the given path.
func createMinimalJPEG(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create jpeg file: %v", err)
	}
	defer f.Close()

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
}

// --- ExtractDateTaken ---

func TestExtractDateTaken_NonExistentFile(t *testing.T) {
	_, valid := ExtractDateTaken("/no/such/file.jpg")
	if valid {
		t.Error("expected valid=false for non-existent file")
	}
}

func TestExtractDateTaken_JPEGWithoutEXIF(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "noexif.jpg")
	createMinimalJPEG(t, p)

	_, valid := ExtractDateTaken(p)
	if valid {
		t.Error("expected valid=false for JPEG without EXIF")
	}
}

// --- ExtractFullEXIF ---

func TestExtractFullEXIF_NonExistentFile(t *testing.T) {
	_, err := ExtractFullEXIF("/no/such/file.jpg")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestExtractFullEXIF_NonJPEGFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "plain.txt")
	if err := os.WriteFile(p, []byte("not a jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ExtractFullEXIF(p)
	if err == nil {
		t.Error("expected error for non-JPEG file")
	}
}

func TestExtractFullEXIF_JPEGWithoutEXIF(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "noexif.jpg")
	createMinimalJPEG(t, p)

	_, err := ExtractFullEXIF(p)
	if err == nil {
		t.Error("expected error for JPEG without EXIF data")
	}
}

// --- SortGroupsByDate ---

func TestSortGroupsByDate_EmptyGroups(t *testing.T) {
	err := SortGroupsByDate(context.Background(), nil, nil)
	if err != nil {
		t.Errorf("expected no error for empty groups, got %v", err)
	}

	err = SortGroupsByDate(context.Background(), []*DuplicateGroup{}, nil)
	if err != nil {
		t.Errorf("expected no error for zero-length slice, got %v", err)
	}
}

func TestSortGroupsByDate_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	groups := []*DuplicateGroup{
		{Photos: []*PhotoInfo{{Path: "/fake/a.jpg", IsUnique: true}}},
		{Photos: []*PhotoInfo{{Path: "/fake/b.jpg", IsUnique: true}}},
	}

	err := SortGroupsByDate(ctx, groups, nil)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestSortGroupsByDate_GroupsWithPhotos(t *testing.T) {
	dir := t.TempDir()
	// Create two JPEG files (no EXIF, so both will have invalid dates)
	p1 := filepath.Join(dir, "aaa.jpg")
	p2 := filepath.Join(dir, "bbb.jpg")
	createMinimalJPEG(t, p1)
	createMinimalJPEG(t, p2)

	g1 := &DuplicateGroup{Photos: []*PhotoInfo{{Path: p2, IsUnique: true}}}
	g2 := &DuplicateGroup{Photos: []*PhotoInfo{{Path: p1, IsUnique: true}}}
	groups := []*DuplicateGroup{g1, g2}

	err := SortGroupsByDate(context.Background(), groups, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both dates are invalid, so they should be sorted by path (aaa < bbb)
	if groups[0].Photos[0].Path != p1 {
		t.Errorf("expected first group path %s, got %s", p1, groups[0].Photos[0].Path)
	}
	if groups[1].Photos[0].Path != p2 {
		t.Errorf("expected second group path %s, got %s", p2, groups[1].Photos[0].Path)
	}
}

func TestSortGroupsByDate_EmptyPhotosInGroup(t *testing.T) {
	groups := []*DuplicateGroup{
		{Photos: []*PhotoInfo{}},
		{Photos: nil},
	}
	err := SortGroupsByDate(context.Background(), groups, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- MoveDuplicate ---

func TestMoveDuplicate_Success(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "photo.jpg")
	createMinimalJPEG(t, src)

	newPath, err := MoveDuplicate(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := filepath.Join(dir, "duplicates", "photo.jpg")
	if newPath != expected {
		t.Errorf("expected new path %s, got %s", expected, newPath)
	}

	// Original should no longer exist
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("expected original file to be removed after move")
	}

	// Destination should exist
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("expected destination file to exist: %v", err)
	}
}

func TestMoveDuplicate_NonExistentSource(t *testing.T) {
	_, err := MoveDuplicate("/no/such/dir/file.jpg")
	if err == nil {
		t.Error("expected error for non-existent source")
	}
}

// --- RelocateGroupDuplicates ---

func TestRelocateGroupDuplicates_EmptyGroups(t *testing.T) {
	errs := RelocateGroupDuplicates(context.Background(), nil, nil)
	if len(errs) != 0 {
		t.Errorf("expected no errors for nil groups, got %v", errs)
	}

	errs = RelocateGroupDuplicates(context.Background(), []*DuplicateGroup{}, nil)
	if len(errs) != 0 {
		t.Errorf("expected no errors for empty groups, got %v", errs)
	}
}

func TestRelocateGroupDuplicates_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	groups := []*DuplicateGroup{
		{Photos: []*PhotoInfo{{Path: "/fake/a.jpg", IsUnique: false}}},
	}

	errs := RelocateGroupDuplicates(ctx, groups, nil)
	if len(errs) == 0 {
		t.Fatal("expected errors for cancelled context")
	}
	if errs[0] != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", errs[0])
	}
}

func TestRelocateGroupDuplicates_MovesDuplicatesOnly(t *testing.T) {
	dir := t.TempDir()
	uniquePath := filepath.Join(dir, "unique.jpg")
	dupePath := filepath.Join(dir, "dupe.jpg")
	createMinimalJPEG(t, uniquePath)
	createMinimalJPEG(t, dupePath)

	dupePhoto := &PhotoInfo{Path: dupePath, IsUnique: false}
	uniquePhoto := &PhotoInfo{Path: uniquePath, IsUnique: true}

	groups := []*DuplicateGroup{
		{Photos: []*PhotoInfo{uniquePhoto, dupePhoto}},
	}

	var progressCalled bool
	errs := RelocateGroupDuplicates(context.Background(), groups, func(current, total int, msg string) {
		progressCalled = true
	})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !progressCalled {
		t.Error("expected progress callback to be called")
	}

	// Unique file should stay in place
	if _, err := os.Stat(uniquePath); err != nil {
		t.Error("unique file should not have been moved")
	}

	// Duplicate should have been moved
	expectedDupePath := filepath.Join(dir, "duplicates", "dupe.jpg")
	if _, err := os.Stat(expectedDupePath); err != nil {
		t.Errorf("expected duplicate to be moved to %s: %v", expectedDupePath, err)
	}
	if dupePhoto.Path != expectedDupePath {
		t.Errorf("expected PhotoInfo.Path updated to %s, got %s", expectedDupePath, dupePhoto.Path)
	}
}

func TestRelocateGroupDuplicates_NilCallback(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "file.jpg")
	createMinimalJPEG(t, p)

	groups := []*DuplicateGroup{
		{Photos: []*PhotoInfo{{Path: p, IsUnique: false}}},
	}

	errs := RelocateGroupDuplicates(context.Background(), groups, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}
