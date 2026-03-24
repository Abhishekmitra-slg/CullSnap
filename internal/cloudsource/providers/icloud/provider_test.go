package icloud

import (
	"os"
	"testing"
	"time"

	"cullsnap/internal/cloudsource"
	"cullsnap/internal/logger"
)

func TestMain(m *testing.M) {
	logger.Init("/dev/null") //nolint:errcheck // test init
	os.Exit(m.Run())
}

func TestParseAlbumOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []cloudsource.Album
	}{
		{
			name:     "empty output",
			input:    "",
			expected: nil,
		},
		{
			name:  "single album",
			input: "Vacation 2024|||ABC-123|||42",
			expected: []cloudsource.Album{
				{ID: "ABC-123", Title: "Vacation 2024", MediaCount: 42},
			},
		},
		{
			name:  "multiple albums",
			input: "Vacation 2024|||ABC-123|||42###Family|||DEF-456|||15###Pets|||GHI-789|||7",
			expected: []cloudsource.Album{
				{ID: "ABC-123", Title: "Vacation 2024", MediaCount: 42},
				{ID: "DEF-456", Title: "Family", MediaCount: 15},
				{ID: "GHI-789", Title: "Pets", MediaCount: 7},
			},
		},
		{
			name:     "malformed entry skipped",
			input:    "bad entry###Good Album|||ID-1|||5",
			expected: []cloudsource.Album{
				{ID: "ID-1", Title: "Good Album", MediaCount: 5},
			},
		},
		{
			name:  "album with zero count",
			input: "Empty Album|||ID-EMPTY|||0",
			expected: []cloudsource.Album{
				{ID: "ID-EMPTY", Title: "Empty Album", MediaCount: 0},
			},
		},
		{
			name:     "whitespace only entries",
			input:    "  ###  ###  ",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			albums, err := parseAlbumOutput(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(albums) != len(tt.expected) {
				t.Fatalf("expected %d albums, got %d", len(tt.expected), len(albums))
			}
			for i, a := range albums {
				exp := tt.expected[i]
				if a.ID != exp.ID {
					t.Errorf("album[%d] ID: expected %q, got %q", i, exp.ID, a.ID)
				}
				if a.Title != exp.Title {
					t.Errorf("album[%d] Title: expected %q, got %q", i, exp.Title, a.Title)
				}
				if a.MediaCount != exp.MediaCount {
					t.Errorf("album[%d] MediaCount: expected %d, got %d", i, exp.MediaCount, a.MediaCount)
				}
			}
		})
	}
}

func TestParseMediaOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []cloudsource.RemoteMedia
	}{
		{
			name:     "empty output",
			input:    "",
			expected: nil,
		},
		{
			name:  "single media item",
			input: "IMG_1234.jpg|||MEDIA-001|||January 15, 2024 at 2:30:00 PM|||4500000",
			expected: []cloudsource.RemoteMedia{
				{ID: "MEDIA-001", Filename: "IMG_1234.jpg", SizeBytes: 4500000},
			},
		},
		{
			name:  "multiple media items",
			input: "IMG_001.jpg|||M-1|||2024-01-01 10:00:00|||1000###IMG_002.png|||M-2|||2024-01-02 11:00:00|||2000",
			expected: []cloudsource.RemoteMedia{
				{ID: "M-1", Filename: "IMG_001.jpg", SizeBytes: 1000},
				{ID: "M-2", Filename: "IMG_002.png", SizeBytes: 2000},
			},
		},
		{
			name:     "malformed entry skipped",
			input:    "bad|||entry###Good.jpg|||M-1|||2024-01-01 10:00:00|||5000",
			expected: []cloudsource.RemoteMedia{
				{ID: "M-1", Filename: "Good.jpg", SizeBytes: 5000},
			},
		},
		{
			name:  "zero size",
			input: "small.jpg|||M-1|||2024-01-01 10:00:00|||0",
			expected: []cloudsource.RemoteMedia{
				{ID: "M-1", Filename: "small.jpg", SizeBytes: 0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			media, err := parseMediaOutput(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(media) != len(tt.expected) {
				t.Fatalf("expected %d media, got %d", len(tt.expected), len(media))
			}
			for i, m := range media {
				exp := tt.expected[i]
				if m.ID != exp.ID {
					t.Errorf("media[%d] ID: expected %q, got %q", i, exp.ID, m.ID)
				}
				if m.Filename != exp.Filename {
					t.Errorf("media[%d] Filename: expected %q, got %q", i, exp.Filename, m.Filename)
				}
				if m.SizeBytes != exp.SizeBytes {
					t.Errorf("media[%d] SizeBytes: expected %d, got %d", i, exp.SizeBytes, m.SizeBytes)
				}
			}
		})
	}
}

func TestParseAppleScriptDate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Time
	}{
		{
			name:     "RFC3339 format",
			input:    "2024-06-15T14:30:00Z",
			expected: time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC),
		},
		{
			name:     "US long format",
			input:    "January 15, 2024 at 2:30:00 PM",
			expected: time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC),
		},
		{
			name:     "numeric US format",
			input:    "1/15/2024 2:30:00 PM",
			expected: time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC),
		},
		{
			name:     "ISO-like format",
			input:    "2024-01-15 14:30:00",
			expected: time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC),
		},
		{
			name:     "unparseable returns zero time",
			input:    "not a date",
			expected: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAppleScriptDate(tt.input)
			if !got.Equal(tt.expected) {
				t.Errorf("parseAppleScriptDate(%q) = %v, expected %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestProviderInterface(t *testing.T) {
	p := New(nil)

	if p.ID() != "icloud" {
		t.Errorf("ID() = %q, expected %q", p.ID(), "icloud")
	}
	if p.DisplayName() != "iCloud Photos" {
		t.Errorf("DisplayName() = %q, expected %q", p.DisplayName(), "iCloud Photos")
	}
	if err := p.Disconnect(); err != nil {
		t.Errorf("Disconnect() returned unexpected error: %v", err)
	}
}

func TestProviderImplementsCloudSource(t *testing.T) {
	var _ cloudsource.CloudSource = (*Provider)(nil)
}
