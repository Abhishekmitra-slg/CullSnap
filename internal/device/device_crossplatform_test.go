package device

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewDetector(t *testing.T) {
	d := NewDetector()
	if d == nil {
		t.Fatal("NewDetector returned nil")
	}
	// Verify ConnectedDevices doesn't panic
	devices := d.ConnectedDevices()
	_ = devices
}

func TestNewDetector_StartStop(t *testing.T) {
	d := NewDetector()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so Start returns quickly
	// Verify Start/Stop don't panic
	d.Start(ctx)
	d.Stop()
}

func TestNewDetector_Callbacks(t *testing.T) {
	d := NewDetector()
	// Verify registering callbacks doesn't panic
	d.OnConnect(func(_ Device) {})
	d.OnDisconnect(func(_ Device) {})
}

func TestDeviceStruct(t *testing.T) {
	d := Device{
		Name:      "Test iPhone",
		VendorID:  "0x05ac",
		ProductID: "0x12a8",
		Serial:    "ABC123",
	}
	if d.Name != "Test iPhone" {
		t.Errorf("Name = %q, want %q", d.Name, "Test iPhone")
	}
	if d.VendorID != "0x05ac" {
		t.Errorf("VendorID = %q, want %q", d.VendorID, "0x05ac")
	}
	if d.ProductID != "0x12a8" {
		t.Errorf("ProductID = %q, want %q", d.ProductID, "0x12a8")
	}
	if d.Serial != "ABC123" {
		t.Errorf("Serial = %q, want %q", d.Serial, "ABC123")
	}
}

func TestDeviceStruct_ZeroValue(t *testing.T) {
	var d Device
	if d.Name != "" {
		t.Errorf("zero value Name should be empty, got %q", d.Name)
	}
	if d.Serial != "" {
		t.Errorf("zero value Serial should be empty, got %q", d.Serial)
	}
	if !d.DetectedAt.IsZero() {
		t.Errorf("zero value DetectedAt should be zero time")
	}
}

func TestDeviceStruct_TypeAndMountPath(t *testing.T) {
	d := Device{
		Name:      "Samsung Galaxy S24",
		VendorID:  "0x04e8",
		ProductID: "0x6860",
		Serial:    "R5CN123ABC",
		Type:      "android",
		MountPath: "/run/user/1000/gvfs/mtp:host=SAMSUNG_Galaxy_S24_R5CN123ABC",
	}
	if d.Type != "android" {
		t.Errorf("Type = %q, want %q", d.Type, "android")
	}
	if d.MountPath == "" {
		t.Error("MountPath should not be empty")
	}
}

func TestDeviceStruct_TypeZeroValue(t *testing.T) {
	var d Device
	if d.Type != "" {
		t.Errorf("zero value Type should be empty, got %q", d.Type)
	}
	if d.MountPath != "" {
		t.Errorf("zero value MountPath should be empty, got %q", d.MountPath)
	}
}

func TestSanitizeSerial_CrossPlatform(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"normal serial", "abc123def456", "abc123def456"},
		{"empty string", "", "_"},
		{"with spaces", "abc 123 def", "abc_123_def"},
		{"path traversal", "../../../etc/passwd", ".._.._.._etc_passwd"},
		{"dots and dashes kept", "abc-123.xyz", "abc-123.xyz"},
		{"special chars stripped", "abc$%^&*()123", "abc_______123"},
		{"slashes", "abc/def\\ghi", "abc_def_ghi"},
		{"all special", "!@#$%^", "______"},
		{"underscores preserved", "abc_123_def", "abc_123_def"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeSerial(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeSerial(%q) = %q, want %q", tt.input, got, tt.want)
			}
			// Result should never contain path separators
			if filepath.Base(got) != got {
				t.Errorf("SanitizeSerial(%q) = %q contains path separator", tt.input, got)
			}
		})
	}
}

func TestCountFiles_CrossPlatform(t *testing.T) {
	dir := t.TempDir()

	// Empty dir
	if got := countFiles(dir); got != 0 {
		t.Errorf("countFiles(empty) = %d, want 0", got)
	}

	// Create some files and a subdirectory
	for _, name := range []string{"a.jpg", "b.png", "c.heic"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := countFiles(dir); got != 3 {
		t.Errorf("countFiles = %d, want 3", got)
	}
}

func TestCountFiles_MissingDir(t *testing.T) {
	if got := countFiles("/nonexistent/path/that/does/not/exist"); got != 0 {
		t.Errorf("countFiles(nonexistent) = %d, want 0", got)
	}
}

func TestImportFromDevice_NonDarwin(t *testing.T) {
	// ImportFromDevice should return an error on non-darwin or succeed on darwin.
	// Either way it should not panic.
	_, _, err := ImportFromDevice(context.Background(), "test-serial", t.TempDir())
	_ = err
}
