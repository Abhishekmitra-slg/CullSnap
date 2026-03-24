package device

import (
	"cullsnap/internal/logger"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	logger.Init("/dev/null") //nolint:errcheck // test init
	os.Exit(m.Run())
}

const testSystemProfilerOutput = `{
	"SPUSBDataType": [
		{
			"_name": "USB31Bus",
			"_items": [
				{
					"_name": "iPhone",
					"vendor_id": "0x05ac (Apple Inc.)",
					"product_id": "0x12a8",
					"serial_num": "abc123def456",
					"_items": []
				},
				{
					"_name": "AirPods Pro",
					"vendor_id": "0x05ac (Apple Inc.)",
					"product_id": "0x2002",
					"serial_num": "airpods789"
				}
			]
		},
		{
			"_name": "USB31Bus",
			"_items": [
				{
					"_name": "USB Hub",
					"vendor_id": "0x1234",
					"product_id": "0x0001",
					"_items": [
						{
							"_name": "iPad Pro",
							"vendor_id": "0x05ac",
							"product_id": "0x12ab",
							"serial_num": "ipad999xyz"
						}
					]
				}
			]
		}
	]
}`

const testEmptyOutput = `{
	"SPUSBDataType": []
}`

const testNoAppleDevices = `{
	"SPUSBDataType": [
		{
			"_name": "USB31Bus",
			"_items": [
				{
					"_name": "External SSD",
					"vendor_id": "0x1234",
					"product_id": "0x5678",
					"serial_num": "ssd001"
				}
			]
		}
	]
}`

func TestParseUSBDevices_FindsIPhoneAndIPad(t *testing.T) {
	devices, err := ParseUSBDevices([]byte(testSystemProfilerOutput))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	// Check iPhone
	var iphone, ipad *Device
	for i := range devices {
		switch devices[i].Name {
		case "iPhone":
			iphone = &devices[i]
		case "iPad Pro":
			ipad = &devices[i]
		}
	}

	if iphone == nil {
		t.Fatal("expected to find iPhone")
	}
	if iphone.VendorID != "0x05ac" {
		t.Errorf("iPhone vendor_id = %q, want %q", iphone.VendorID, "0x05ac")
	}
	if iphone.ProductID != "0x12a8" {
		t.Errorf("iPhone product_id = %q, want %q", iphone.ProductID, "0x12a8")
	}
	if iphone.Serial != "abc123def456" {
		t.Errorf("iPhone serial = %q, want %q", iphone.Serial, "abc123def456")
	}

	if ipad == nil {
		t.Fatal("expected to find iPad Pro")
	}
	if ipad.Serial != "ipad999xyz" {
		t.Errorf("iPad serial = %q, want %q", ipad.Serial, "ipad999xyz")
	}
}

func TestParseUSBDevices_FiltersNonMobileAppleDevices(t *testing.T) {
	devices, err := ParseUSBDevices([]byte(testSystemProfilerOutput))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, d := range devices {
		if d.Name == "AirPods Pro" {
			t.Error("AirPods Pro should be filtered out (not iPhone/iPad)")
		}
	}
}

func TestParseUSBDevices_NestedDevice(t *testing.T) {
	devices, err := ParseUSBDevices([]byte(testSystemProfilerOutput))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, d := range devices {
		if d.Name == "iPad Pro" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find iPad Pro nested under USB Hub")
	}
}

func TestParseUSBDevices_EmptyBusList(t *testing.T) {
	devices, err := ParseUSBDevices([]byte(testEmptyOutput))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(devices))
	}
}

func TestParseUSBDevices_NoAppleDevices(t *testing.T) {
	devices, err := ParseUSBDevices([]byte(testNoAppleDevices))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(devices))
	}
}

func TestParseUSBDevices_InvalidJSON(t *testing.T) {
	_, err := ParseUSBDevices([]byte(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestNormalizeVendorID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"0x05ac (Apple Inc.)", "0x05ac"},
		{"0x05ac", "0x05ac"},
		{"0x1234 (Some Vendor)", "0x1234"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeVendorID(tt.input)
		if got != tt.want {
			t.Errorf("normalizeVendorID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestReconcile_ConnectDisconnect(t *testing.T) {
	det := &DarwinDetector{
		devices: make(map[string]Device),
	}

	var connected, disconnected []Device
	det.OnConnect(func(d Device) { connected = append(connected, d) })
	det.OnDisconnect(func(d Device) { disconnected = append(disconnected, d) })

	// Simulate first poll: iPhone appears.
	devices1, _ := ParseUSBDevices([]byte(testSystemProfilerOutput))
	det.reconcile(devices1)

	if len(connected) != 2 {
		t.Fatalf("expected 2 connect events, got %d", len(connected))
	}
	if len(disconnected) != 0 {
		t.Fatalf("expected 0 disconnect events, got %d", len(disconnected))
	}

	// Reset trackers.
	connected = nil
	disconnected = nil

	// Simulate second poll: no devices.
	det.reconcile(nil)

	if len(connected) != 0 {
		t.Errorf("expected 0 connect events, got %d", len(connected))
	}
	if len(disconnected) != 2 {
		t.Errorf("expected 2 disconnect events, got %d", len(disconnected))
	}

	if len(det.ConnectedDevices()) != 0 {
		t.Errorf("expected 0 connected devices, got %d", len(det.ConnectedDevices()))
	}
}

func TestReconcile_NoChangeNoDuplicate(t *testing.T) {
	det := &DarwinDetector{
		devices: make(map[string]Device),
	}

	var connectCount int
	det.OnConnect(func(_ Device) { connectCount++ })

	devices1, _ := ParseUSBDevices([]byte(testSystemProfilerOutput))

	// First poll.
	det.reconcile(devices1)
	if connectCount != 2 {
		t.Fatalf("expected 2 connect events, got %d", connectCount)
	}

	// Second poll with same devices — no new events.
	det.reconcile(devices1)
	if connectCount != 2 {
		t.Errorf("expected still 2 connect events (no duplicates), got %d", connectCount)
	}
}
