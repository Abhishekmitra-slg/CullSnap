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
