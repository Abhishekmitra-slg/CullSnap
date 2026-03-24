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

func TestCheckDiskSpace_UnderLimit(t *testing.T) {
	mm := &MirrorManager{
		baseDir:    t.TempDir(),
		maxCacheMB: 1024, // 1 GB
	}
	err := mm.CheckDiskSpace(100 * 1024 * 1024) // 100 MB
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCheckDiskSpace_OverLimit(t *testing.T) {
	mm := &MirrorManager{
		baseDir:    t.TempDir(),
		maxCacheMB: 1, // 1 MB
	}
	err := mm.CheckDiskSpace(10 * 1024 * 1024) // 10 MB
	if err == nil {
		t.Error("expected error for over-limit download")
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
