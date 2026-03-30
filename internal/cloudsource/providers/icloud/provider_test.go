//go:build darwin

package icloud

import (
	"bytes"
	"context"
	"cullsnap/internal/cloudsource"
	"cullsnap/internal/logger"
	"os"
	"path/filepath"
	"testing"
	"time"
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
			name:  "malformed entry skipped",
			input: "bad entry###Good Album|||ID-1|||5",
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
			name:  "malformed entry skipped",
			input: "bad|||entry###Good.jpg|||M-1|||2024-01-01 10:00:00|||5000",
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

func TestDownload_ExistingFile_Skips(t *testing.T) {
	p := New(nil)
	tmpFile := filepath.Join(t.TempDir(), "existing.jpg")
	if err := os.WriteFile(tmpFile, []byte("photo data"), 0o600); err != nil {
		t.Fatal(err)
	}

	media := cloudsource.RemoteMedia{ID: "test-id", Filename: "IMG_001.jpg"}
	err := p.Download(context.Background(), media, tmpFile, nil)
	if err != nil {
		t.Errorf("expected nil error for existing file, got: %v", err)
	}
}

func TestIsSequentialDownload(t *testing.T) {
	p := New(nil)
	if !p.IsSequentialDownload() {
		t.Error("iCloud provider should return true for IsSequentialDownload")
	}
}

func TestCopyFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	srcPath := filepath.Join(srcDir, "source.jpg")
	dstPath := filepath.Join(dstDir, "dest.jpg")
	content := []byte("test photo content 12345")

	if err := os.WriteFile(srcPath, content, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %q, want %q", string(got), string(content))
	}

	info, err := os.Stat(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestValidatePhotoID_Valid(t *testing.T) {
	valid := []string{
		"ABC-123",
		"B18B4FDD-B235-4255-9CA6-C398E4E42D4A/L0/001",
		"simple",
		"with_underscore",
	}
	for _, id := range valid {
		if err := validatePhotoID(id); err != nil {
			t.Errorf("validatePhotoID(%q) returned unexpected error: %v", id, err)
		}
	}
}

func TestValidatePhotoID_Invalid(t *testing.T) {
	invalid := []string{
		`has"quote`,
		"has space",
		"has;semicolon",
		"has\nnewline",
		"",
	}
	for _, id := range invalid {
		if err := validatePhotoID(id); err == nil {
			t.Errorf("validatePhotoID(%q) expected error, got nil", id)
		}
	}
}

// canned osascript output fixtures

const cannedCountOutput = "9247"

const cannedPageOutput = `IMG_0001.HEIC|||ABC-001|||Sunday, January 5, 2025 at 10:15:30 AM|||3145728###IMG_0002.JPG|||ABC-002|||Monday, January 6, 2025 at 11:20:45 AM|||2097152###IMG_0003.MOV|||ABC-003|||Tuesday, January 7, 2025 at 2:30:00 PM|||52428800`

const cannedPageOutputEmpty = ""

const cannedPageOutputMalformed = `IMG_0004.HEIC|||ABC-004|||Wednesday, January 8, 2025 at 8:00:00 AM|||1048576###malformed_no_pipe`

func TestParseItemCount(t *testing.T) {
	count, err := parseItemCount(cannedCountOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 9247 {
		t.Fatalf("expected 9247, got %d", count)
	}
}

func TestParseItemCount_Zero(t *testing.T) {
	count, err := parseItemCount("0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
}

func TestParseItemCount_Invalid(t *testing.T) {
	_, err := parseItemCount("not-a-number")
	if err == nil {
		t.Fatal("expected error for non-numeric input, got nil")
	}
}

func TestParseMediaOutput_ThreeItems(t *testing.T) {
	media, err := parseMediaOutput(cannedPageOutput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(media) != 3 {
		t.Fatalf("expected 3 items, got %d", len(media))
	}

	// first item
	if media[0].Filename != "IMG_0001.HEIC" {
		t.Errorf("item 0 filename: got %q, want %q", media[0].Filename, "IMG_0001.HEIC")
	}
	if media[0].ID != "ABC-001" {
		t.Errorf("item 0 ID: got %q, want %q", media[0].ID, "ABC-001")
	}
	if media[0].SizeBytes != 3145728 {
		t.Errorf("item 0 size: got %d, want 3145728", media[0].SizeBytes)
	}

	// third item (video)
	if media[2].Filename != "IMG_0003.MOV" {
		t.Errorf("item 2 filename: got %q, want %q", media[2].Filename, "IMG_0003.MOV")
	}
}

func TestParseMediaOutput_Empty(t *testing.T) {
	media, err := parseMediaOutput(cannedPageOutputEmpty)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if media != nil && len(media) != 0 {
		t.Fatalf("expected nil or empty slice, got %v", media)
	}
}

func TestParseMediaOutput_SkipsMalformed(t *testing.T) {
	// The fixture has one valid 4-part entry and one malformed 2-part entry.
	// parseMediaOutput must skip the malformed one silently.
	media, err := parseMediaOutput(cannedPageOutputMalformed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(media) != 1 {
		t.Fatalf("expected 1 valid item (malformed skipped), got %d", len(media))
	}
	if media[0].Filename != "IMG_0004.HEIC" {
		t.Errorf("unexpected filename: %q", media[0].Filename)
	}
}

func TestBuildPageRanges(t *testing.T) {
	cases := []struct {
		total    int
		pageSize int
		wantLen  int
		first    [2]int // [start, end] of first range (1-based AppleScript)
		last     [2]int // [start, end] of last range
	}{
		{total: 3, pageSize: 500, wantLen: 1, first: [2]int{1, 3}, last: [2]int{1, 3}},
		{total: 500, pageSize: 500, wantLen: 1, first: [2]int{1, 500}, last: [2]int{1, 500}},
		{total: 501, pageSize: 500, wantLen: 2, first: [2]int{1, 500}, last: [2]int{501, 501}},
		{total: 1000, pageSize: 500, wantLen: 2, first: [2]int{1, 500}, last: [2]int{501, 1000}},
		{total: 1001, pageSize: 500, wantLen: 3, first: [2]int{1, 500}, last: [2]int{1001, 1001}},
		{total: 9247, pageSize: 500, wantLen: 19, first: [2]int{1, 500}, last: [2]int{9001, 9247}},
	}

	for _, tc := range cases {
		ranges := buildPageRanges(tc.total, tc.pageSize)
		if len(ranges) != tc.wantLen {
			t.Errorf("total=%d pageSize=%d: got %d ranges, want %d",
				tc.total, tc.pageSize, len(ranges), tc.wantLen)
			continue
		}
		first := ranges[0]
		if first[0] != tc.first[0] || first[1] != tc.first[1] {
			t.Errorf("total=%d: first range got [%d,%d], want [%d,%d]",
				tc.total, first[0], first[1], tc.first[0], tc.first[1])
		}
		last := ranges[len(ranges)-1]
		if last[0] != tc.last[0] || last[1] != tc.last[1] {
			t.Errorf("total=%d: last range got [%d,%d], want [%d,%d]",
				tc.total, last[0], last[1], tc.last[0], tc.last[1])
		}
	}
}
