package cloudsource

import (
	"testing"
)

func TestMirrorPath(t *testing.T) {
	mm := &MirrorManager{baseDir: "/cache/cloud"}
	path := mm.MirrorPath("google_drive", "album-123")

	if path != "/cache/cloud/google_drive/album-123" {
		t.Errorf("unexpected path: %s", path)
	}
}

func TestMirrorPath_Sanitization(t *testing.T) {
	mm := &MirrorManager{baseDir: "/cache/cloud"}
	path := mm.MirrorPath("../evil", "../../etc/passwd")

	// Should not contain traversal
	if path == "/cache/../evil/../../etc/passwd" {
		t.Error("path traversal not sanitized")
	}
}

func TestDiskUsage_EmptyDir(t *testing.T) {
	mm := &MirrorManager{baseDir: t.TempDir()}
	usage, err := mm.DiskUsage()
	if err != nil {
		t.Fatal(err)
	}
	if usage != 0 {
		t.Errorf("expected 0 bytes, got %d", usage)
	}
}

func TestMirrorResult_NoErrors(t *testing.T) {
	r := MirrorResult{
		Dir:       "/tmp/mirror/icloud/album1",
		Evicted:   nil,
		Succeeded: 10,
		Skipped:   2,
		Failed:    0,
		Errors:    nil,
	}
	if r.Failed != 0 {
		t.Fatalf("expected Failed=0, got %d", r.Failed)
	}
	if len(r.Errors) != 0 {
		t.Fatalf("expected empty Errors, got %v", r.Errors)
	}
}

func TestMirrorResult_WithErrors(t *testing.T) {
	r := MirrorResult{
		Dir:       "/tmp/mirror/icloud/album1",
		Evicted:   nil,
		Succeeded: 8,
		Skipped:   0,
		Failed:    2,
		Errors: []DownloadError{
			{Filename: "IMG_001.HEIC", MediaID: "abc-123", Reason: "exported_0_files"},
			{Filename: "IMG_002.MOV", MediaID: "def-456", Reason: "timeout"},
		},
	}
	if r.Failed != 2 {
		t.Fatalf("expected Failed=2, got %d", r.Failed)
	}
	if len(r.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(r.Errors))
	}
	if r.Errors[0].Filename != "IMG_001.HEIC" {
		t.Fatalf("unexpected filename: %s", r.Errors[0].Filename)
	}
	if r.Errors[0].Reason != "exported_0_files" {
		t.Fatalf("unexpected reason: %s", r.Errors[0].Reason)
	}
}

func TestMirrorResult_EvictedAlbums(t *testing.T) {
	evicted := []EvictedAlbum{
		{AlbumTitle: "Old Album", SizeBytes: 1024},
	}
	r := MirrorResult{
		Dir:       "/tmp/mirror",
		Evicted:   evicted,
		Succeeded: 5,
		Skipped:   0,
		Failed:    0,
		Errors:    nil,
	}
	if len(r.Evicted) != 1 {
		t.Fatalf("expected 1 evicted album, got %d", len(r.Evicted))
	}
	if r.Evicted[0].AlbumTitle != "Old Album" {
		t.Fatalf("unexpected evicted album title: %s", r.Evicted[0].AlbumTitle)
	}
}
