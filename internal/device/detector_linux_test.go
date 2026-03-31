//go:build linux

package device

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGVFSMounts_AFC(t *testing.T) {
	gvfsDir := t.TempDir()
	afcDir := filepath.Join(gvfsDir, "afc:host=00008030-001A2D640C30802E")
	if err := os.Mkdir(afcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	devices := parseGVFSMounts(gvfsDir)
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].Type != "iphone" {
		t.Errorf("Type = %q, want %q", devices[0].Type, "iphone")
	}
	if devices[0].MountPath != afcDir {
		t.Errorf("MountPath = %q, want %q", devices[0].MountPath, afcDir)
	}
	if devices[0].Serial == "" {
		t.Error("Serial should not be empty")
	}
}

func TestParseGVFSMounts_MTP(t *testing.T) {
	gvfsDir := t.TempDir()
	mtpDir := filepath.Join(gvfsDir, "mtp:host=SAMSUNG_Galaxy_S24_R5CN123ABC")
	if err := os.Mkdir(mtpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	devices := parseGVFSMounts(gvfsDir)
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].Type != "android" {
		t.Errorf("Type = %q, want %q", devices[0].Type, "android")
	}
}

func TestParseGVFSMounts_Gphoto2(t *testing.T) {
	gvfsDir := t.TempDir()
	gpDir := filepath.Join(gvfsDir, "gphoto2:host=Canon_EOS_R5")
	if err := os.Mkdir(gpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	devices := parseGVFSMounts(gvfsDir)
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].Type != "camera" {
		t.Errorf("Type = %q, want %q", devices[0].Type, "camera")
	}
}

func TestParseGVFSMounts_Mixed(t *testing.T) {
	gvfsDir := t.TempDir()
	os.Mkdir(filepath.Join(gvfsDir, "afc:host=IPHONE_UDID_123"), 0o755)
	os.Mkdir(filepath.Join(gvfsDir, "mtp:host=SAMSUNG_S24"), 0o755)
	os.Mkdir(filepath.Join(gvfsDir, "gphoto2:host=CANON_R5"), 0o755)
	os.Mkdir(filepath.Join(gvfsDir, "dav:host=nextcloud.example.com"), 0o755)
	devices := parseGVFSMounts(gvfsDir)
	if len(devices) != 3 {
		t.Errorf("expected 3 devices, got %d", len(devices))
	}
}

func TestParseGVFSMounts_EmptyDir(t *testing.T) {
	devices := parseGVFSMounts(t.TempDir())
	if len(devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(devices))
	}
}

func TestParseGVFSMounts_NonexistentDir(t *testing.T) {
	devices := parseGVFSMounts("/nonexistent/gvfs/path")
	if len(devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(devices))
	}
}

func TestScanMassStorage(t *testing.T) {
	mediaDir := t.TempDir()
	usbDrive := filepath.Join(mediaDir, "USB_DRIVE")
	os.MkdirAll(filepath.Join(usbDrive, "DCIM"), 0o755)
	devices := scanMassStorage(mediaDir)
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].Type != "storage" {
		t.Errorf("Type = %q, want %q", devices[0].Type, "storage")
	}
	if devices[0].MountPath != usbDrive {
		t.Errorf("MountPath = %q, want %q", devices[0].MountPath, usbDrive)
	}
}

func TestScanMassStorage_NoDCIM(t *testing.T) {
	mediaDir := t.TempDir()
	os.MkdirAll(filepath.Join(mediaDir, "USB_DRIVE", "Documents"), 0o755)
	devices := scanMassStorage(mediaDir)
	if len(devices) != 0 {
		t.Errorf("expected 0, got %d", len(devices))
	}
}

func TestValidateGVFSPath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		uid       int
		wantValid bool
	}{
		{"valid afc", "/run/user/1000/gvfs/afc:host=UDID123", 1000, true},
		{"valid mtp", "/run/user/1000/gvfs/mtp:host=DEVICE", 1000, true},
		{"wrong uid", "/run/user/1001/gvfs/afc:host=UDID123", 1000, false},
		{"escape attempt", "/run/user/1000/gvfs/../../etc/passwd", 1000, false},
		{"random path", "/tmp/fake/gvfs/afc:host=FAKE", 1000, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateGVFSPath(tt.path, tt.uid)
			if got != tt.wantValid {
				t.Errorf("validateGVFSPath(%q, %d) = %v, want %v", tt.path, tt.uid, got, tt.wantValid)
			}
		})
	}
}

func TestDeduplicateDevices(t *testing.T) {
	devices := []Device{
		{Serial: "ABC", Name: "iPhone (lsusb)", Type: "iphone"},
		{Serial: "ABC", Name: "iPhone (GVFS)", Type: "iphone", MountPath: "/run/user/1000/gvfs/afc:host=ABC"},
		{Serial: "DEF", Name: "Samsung", Type: "android"},
	}
	result := deduplicateDevices(devices)
	if len(result) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(result))
	}
	for _, d := range result {
		if d.Serial == "ABC" && d.MountPath == "" {
			t.Error("ABC should prefer the entry with MountPath")
		}
	}
}
