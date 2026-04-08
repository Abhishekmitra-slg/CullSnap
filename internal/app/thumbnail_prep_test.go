package app

import (
	"cullsnap/internal/model"
	"testing"
	"time"
)

func TestBuildThumbnailItems(t *testing.T) {
	now := time.Now()
	photos := []model.Photo{
		{Path: "/photos/a.jpg", TakenAt: now},
		{Path: "/photos/b.png", TakenAt: now.Add(-time.Hour)},
		{Path: "/photos/c.heic", TakenAt: now.Add(-2 * time.Hour)},
	}

	items := buildThumbnailItems(photos)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	for i, item := range items {
		if item.Path != photos[i].Path {
			t.Errorf("item %d: expected path %q, got %q", i, photos[i].Path, item.Path)
		}
		if !item.ModTime.Equal(photos[i].TakenAt) {
			t.Errorf("item %d: expected modTime %v, got %v", i, photos[i].TakenAt, item.ModTime)
		}
	}
}

func TestBuildThumbnailItems_Empty(t *testing.T) {
	items := buildThumbnailItems(nil)
	if len(items) != 0 {
		t.Errorf("expected 0 items for nil input, got %d", len(items))
	}

	items = buildThumbnailItems([]model.Photo{})
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty input, got %d", len(items))
	}
}

func TestDetectHEICInfo_NoHEIC(t *testing.T) {
	items := []thumbnailItem{
		{Path: "/photos/a.jpg"},
		{Path: "/photos/b.png"},
		{Path: "/photos/c.cr2"},
	}

	count, decoder := detectHEICInfo(items, false)
	if count != 0 {
		t.Errorf("expected 0 HEIC files, got %d", count)
	}
	if decoder != "" {
		t.Errorf("expected empty decoder, got %q", decoder)
	}
}

func TestDetectHEICInfo_WithHEIC(t *testing.T) {
	items := []thumbnailItem{
		{Path: "/photos/a.heic"},
		{Path: "/photos/b.jpg"},
		{Path: "/photos/c.heif"},
	}

	count, decoder := detectHEICInfo(items, false)
	if count != 2 {
		t.Errorf("expected 2 HEIC files, got %d", count)
	}
	if decoder != "ffmpeg" {
		t.Errorf("expected ffmpeg decoder when useNativeSips=false, got %q", decoder)
	}
}

func TestDetectHEICInfo_CaseInsensitive(t *testing.T) {
	items := []thumbnailItem{
		{Path: "/photos/a.HEIC"},
		{Path: "/photos/b.Heif"},
	}

	count, _ := detectHEICInfo(items, false)
	if count != 2 {
		t.Errorf("expected 2 HEIC files (case insensitive), got %d", count)
	}
}

func TestDetectHEICInfo_Empty(t *testing.T) {
	count, decoder := detectHEICInfo(nil, false)
	if count != 0 || decoder != "" {
		t.Errorf("expected 0/empty for nil input, got %d/%q", count, decoder)
	}
}

func TestApplyThumbnailPaths(t *testing.T) {
	photos := []model.Photo{
		{Path: "/photos/a.jpg"},
		{Path: "/photos/b.png"},
		{Path: "/photos/c.jpg"},
	}
	thumbnailMap := map[string]string{
		"/photos/a.jpg": "/cache/thumb_a.jpg",
		"/photos/c.jpg": "/cache/thumb_c.jpg",
		// b.png intentionally missing — simulates a failed thumbnail
	}

	applyThumbnailPaths(photos, thumbnailMap)

	if photos[0].ThumbnailPath != "/cache/thumb_a.jpg" {
		t.Errorf("expected thumb_a.jpg, got %q", photos[0].ThumbnailPath)
	}
	if photos[1].ThumbnailPath != "" {
		t.Errorf("expected empty ThumbnailPath for b.png (not in map), got %q", photos[1].ThumbnailPath)
	}
	if photos[2].ThumbnailPath != "/cache/thumb_c.jpg" {
		t.Errorf("expected thumb_c.jpg, got %q", photos[2].ThumbnailPath)
	}
}

func TestApplyThumbnailPaths_EmptyMap(t *testing.T) {
	photos := []model.Photo{
		{Path: "/photos/a.jpg"},
	}
	applyThumbnailPaths(photos, map[string]string{})

	if photos[0].ThumbnailPath != "" {
		t.Errorf("expected empty ThumbnailPath, got %q", photos[0].ThumbnailPath)
	}
}

func TestApplyThumbnailPaths_NilInputs(t *testing.T) {
	// Should not panic
	applyThumbnailPaths(nil, nil)
	applyThumbnailPaths([]model.Photo{}, nil)
	applyThumbnailPaths(nil, map[string]string{})
}
