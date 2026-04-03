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

func TestParseWPDDevices_AndroidPhone(t *testing.T) {
	input := `[{"name":"Galaxy S24","serial":"R5CN123ABC","vendorID":"0x04e8","productID":"0x6860"}]`
	devices, err := parseWPDDevices([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].Type != "android" {
		t.Errorf("type = %q, want %q", devices[0].Type, "android")
	}
}

func TestParseWPDDevices_Camera(t *testing.T) {
	input := `{"name":"EOS R5","serial":"canon001","vendorID":"0x04a9","productID":"0x32d8"}`
	devices, err := parseWPDDevices([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].Type != "camera" {
		t.Errorf("type = %q, want %q", devices[0].Type, "camera")
	}
}

func TestParseWPDDevices_MixedDevices(t *testing.T) {
	input := `[
		{"name":"Apple iPhone","serial":"abc123","vendorID":"0x05ac","productID":"0x12a8"},
		{"name":"Galaxy S24","serial":"R5CN123","vendorID":"0x04e8","productID":"0x6860"},
		{"name":"EOS R5","serial":"canon001","vendorID":"0x04a9","productID":"0x32d8"}
	]`
	devices, err := parseWPDDevices([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}

	types := map[string]bool{}
	for _, d := range devices {
		types[d.Type] = true
	}
	if !types["iphone"] || !types["android"] || !types["camera"] {
		t.Errorf("expected iphone, android, camera types; got %v", types)
	}
}

func TestParseWPDDevices_UnknownVendorSkipped(t *testing.T) {
	input := `[
		{"name":"Apple iPhone","serial":"abc","vendorID":"0x05ac","productID":"0x12a8"},
		{"name":"USB Hub","serial":"hub1","vendorID":"0x1d6b","productID":"0x0003"}
	]`
	devices, err := parseWPDDevices([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 1 {
		t.Errorf("expected 1 device (unknown vendor skipped), got %d", len(devices))
	}
}

func TestParseStorageDevices(t *testing.T) {
	input := `[{"name":"SDCARD","path":"E:","serial":"storage:E:"}]`
	devices := parseStorageDevices([]byte(input))
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].Type != "storage" {
		t.Errorf("type = %q, want %q", devices[0].Type, "storage")
	}
	if devices[0].MountPath != "E:" {
		t.Errorf("mountPath = %q, want %q", devices[0].MountPath, "E:")
	}
}

func TestParseStorageDevices_Single(t *testing.T) {
	input := `{"name":"USB Drive","path":"F:","serial":"storage:F:"}`
	devices := parseStorageDevices([]byte(input))
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].Name != "USB Drive" {
		t.Errorf("name = %q, want %q", devices[0].Name, "USB Drive")
	}
}

func TestParseStorageDevices_Empty(t *testing.T) {
	devices := parseStorageDevices([]byte(""))
	if len(devices) != 0 {
		t.Errorf("expected 0 devices for empty input, got %d", len(devices))
	}
	devices = parseStorageDevices([]byte("[]"))
	if len(devices) != 0 {
		t.Errorf("expected 0 devices for empty array, got %d", len(devices))
	}
}

func TestImportFromDevice_NonDarwin(t *testing.T) {
	// ImportFromDevice should return an error on non-darwin or succeed on darwin.
	// Either way it should not panic.
	_, _, err := ImportFromDevice(context.Background(), "test-serial", t.TempDir())
	_ = err
}
