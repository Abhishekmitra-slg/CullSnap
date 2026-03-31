//go:build linux

package device

import (
	"testing"
)

func TestParseLsusb_MultipleDevices(t *testing.T) {
	output := `Bus 002 Device 001: ID 1d6b:0003 Linux Foundation 3.0 root hub
Bus 001 Device 004: ID 05ac:12a8 Apple, Inc. iPhone
Bus 001 Device 003: ID 04e8:6860 Samsung Electronics Co., Ltd Galaxy series, misc. (MTP mode)
Bus 001 Device 002: ID 04a9:32d8 Canon, Inc. EOS R5
Bus 001 Device 001: ID 1d6b:0002 Linux Foundation 2.0 root hub`

	devices := parseLsusb([]byte(output))
	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}

	found := false
	for _, d := range devices {
		if d.VendorID == "0x05ac" {
			found = true
			if d.Type != "iphone" {
				t.Errorf("Apple device type = %q, want %q", d.Type, "iphone")
			}
		}
	}
	if !found {
		t.Error("Apple device not found")
	}

	found = false
	for _, d := range devices {
		if d.VendorID == "0x04e8" {
			found = true
			if d.Type != "android" {
				t.Errorf("Samsung type = %q, want %q", d.Type, "android")
			}
		}
	}
	if !found {
		t.Error("Samsung not found")
	}

	found = false
	for _, d := range devices {
		if d.VendorID == "0x04a9" {
			found = true
			if d.Type != "camera" {
				t.Errorf("Canon type = %q, want %q", d.Type, "camera")
			}
		}
	}
	if !found {
		t.Error("Canon not found")
	}
}

func TestParseLsusb_Empty(t *testing.T) {
	devices := parseLsusb([]byte(""))
	if len(devices) != 0 {
		t.Errorf("expected 0, got %d", len(devices))
	}
}

func TestParseLsusb_NoKnownDevices(t *testing.T) {
	output := `Bus 001 Device 001: ID 1d6b:0002 Linux Foundation 2.0 root hub
Bus 002 Device 001: ID 1d6b:0003 Linux Foundation 3.0 root hub`
	devices := parseLsusb([]byte(output))
	if len(devices) != 0 {
		t.Errorf("expected 0, got %d", len(devices))
	}
}

func TestParseLsusb_MalformedLine(t *testing.T) {
	output := `Bus 001 Device 004: ID 05ac:12a8 Apple, Inc. iPhone
this is garbage
Bus 001 Device 003: ID 04e8:6860 Samsung Electronics (MTP)`
	devices := parseLsusb([]byte(output))
	if len(devices) != 2 {
		t.Errorf("expected 2, got %d", len(devices))
	}
}
