package ui

import (
	"cullsnap/internal/model"
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestThumbnailGrid_Creation(t *testing.T) {
	// Initialize a test app (required for Fyne widgets)
	a := test.NewApp()
	defer a.Quit()

	grid := NewThumbnailGrid()

	if grid.Columns != 4 {
		t.Errorf("Expected default 4 columns, got %d", grid.Columns)
	}

	if len(grid.Photos) != 0 {
		t.Error("Expected empty photos initially")
	}
}

func TestThumbnailGrid_SetPhotos(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	grid := NewThumbnailGrid()

	photos := []model.Photo{
		{Path: "p1.jpg"},
		{Path: "p2.jpg"},
		{Path: "p3.jpg"},
		{Path: "p4.jpg"},
		{Path: "p5.jpg"},
	}

	grid.SetPhotos(photos)

	if len(grid.Photos) != 5 {
		t.Errorf("Expected 5 photos, got %d", len(grid.Photos))
	}

	// widget.List Length calculation
	// 5 photos, 4 columns -> 2 rows
	expectedRows := 2
	rows := grid.List.Length()
	if rows != expectedRows {
		t.Errorf("Expected %d rows, got %d", expectedRows, rows)
	}
}
