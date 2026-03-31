package device

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf16"
)

// ProgressEvent is a structured JSON line emitted by CullSnap PowerShell scripts
// to report import progress back to the Go host over stdout.
type ProgressEvent struct {
	Type     string `json:"type"`
	Name     string `json:"name,omitempty"`
	Progress int    `json:"progress,omitempty"`
	Total    int    `json:"total,omitempty"`
	Copied   int    `json:"copied,omitempty"`
	Device   string `json:"device,omitempty"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
}

// encodePowerShellScript encodes a PowerShell script as base64 UTF-16LE suitable
// for use with powershell.exe -EncodedCommand. Encoding prevents quoting issues
// and allows arbitrary scripts to be passed safely on the command line.
func encodePowerShellScript(script string) string {
	runes := []rune(script)
	u16 := utf16.Encode(runes)
	buf := make([]byte, len(u16)*2)
	for i, c := range u16 {
		buf[2*i] = byte(c)
		buf[2*i+1] = byte(c >> 8)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// wpdDevice is the JSON shape emitted by the Windows WPD detection PowerShell script.
type wpdDevice struct {
	Name      string `json:"name"`
	Serial    string `json:"serial"`
	VendorID  string `json:"vendorID"`
	ProductID string `json:"productID"`
}

// parseWPDDevices parses the JSON output from the Windows WPD detection script.
// It handles both an array (multiple devices) and a single object (PowerShell's
// ConvertTo-Json outputs a bare object, not a one-element array, for a single
// result). Returns nil, nil for empty or whitespace-only input.
func parseWPDDevices(data []byte) ([]Device, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}

	now := time.Now()

	// Try array first — the normal case for two or more devices.
	var wpds []wpdDevice
	if err := json.Unmarshal(data, &wpds); err == nil {
		devices := make([]Device, 0, len(wpds))
		for _, w := range wpds {
			devices = append(devices, Device{
				Name:       w.Name,
				VendorID:   w.VendorID,
				ProductID:  w.ProductID,
				Serial:     w.Serial,
				DetectedAt: now,
			})
		}
		return devices, nil
	}

	// Fall back to a single object — PowerShell quirk for exactly one result.
	var single wpdDevice
	if err := json.Unmarshal(data, &single); err != nil {
		return nil, fmt.Errorf("powershell: parse WPD devices: %w", err)
	}
	return []Device{{
		Name:       single.Name,
		VendorID:   single.VendorID,
		ProductID:  single.ProductID,
		Serial:     single.Serial,
		DetectedAt: now,
	}}, nil
}

// parseProgressLine decodes a single JSON progress line emitted by a CullSnap
// PowerShell script. Returns an error if the line is empty or not valid JSON.
func parseProgressLine(line []byte) (ProgressEvent, error) {
	if len(line) == 0 {
		return ProgressEvent{}, fmt.Errorf("powershell: empty progress line")
	}
	var ev ProgressEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return ProgressEvent{}, fmt.Errorf("powershell: parse progress line: %w", err)
	}
	return ev, nil
}
