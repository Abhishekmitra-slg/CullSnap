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

	// Video Support
	IsVideo   bool    // True if media is a video
	Duration  float64 // Total video duration in seconds
	TrimStart float64 // Clipping start point in seconds
	TrimEnd   float64 // Clipping end point in seconds

	// RAW Support
	IsRAW          bool   `json:"isRAW"`          // True if file is a RAW format
	RAWFormat      string `json:"rawFormat"`      // "CR3", "ARW", "NEF", etc.
	CompanionPath  string `json:"companionPath"`  // Path to RAW+JPEG pair companion
	IsRAWCompanion bool   `json:"isRAWCompanion"` // True if this JPEG has a RAW companion
}

// Session represents a culling session.
type Session struct {
	ID        string
	CreatedAt time.Time
	Name      string
}
