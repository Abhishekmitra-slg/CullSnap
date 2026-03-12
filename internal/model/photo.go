package model

import "time"

// Photo represents a single image file.
type Photo struct {
	Path          string
	ThumbnailPath string // Cached thumbnail for fast grid display
	Width         int
	Height        int
	Size          int64
	TakenAt       time.Time
}

// Session represents a culling session.
type Session struct {
	ID        string
	CreatedAt time.Time
	Name      string
}
