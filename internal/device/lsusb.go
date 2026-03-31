//go:build linux

package device

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
	"time"
)

var lsusbLineRe = regexp.MustCompile(`^Bus \d{3} Device \d{3}: ID ([0-9a-f]{4}):([0-9a-f]{4})\s+(.*)$`)

var knownPhoneVendors = map[string]bool{
	"04e8": true, // Samsung
	"18d1": true, // Google
	"22b8": true, // Motorola
	"2717": true, // Xiaomi
	"12d1": true, // Huawei
	"2a70": true, // OnePlus
	"0bb4": true, // HTC
	"1004": true, // LG
	"2ae5": true, // Fairphone
	"29a9": true, // Nothing
}

var knownCameraVendors = map[string]bool{
	"04a9": true, // Canon
	"04b0": true, // Nikon
	"054c": true, // Sony
	"04cb": true, // Fujifilm
	"07b4": true, // Olympus
	"04da": true, // Panasonic
	"0a17": true, // Pentax/Ricoh
	"1199": true, // Sigma
}

func classifyVendor(vendorID string) string {
	vid := strings.ToLower(vendorID)
	if vid == "05ac" {
		return "iphone"
	}
	if knownPhoneVendors[vid] {
		return "android"
	}
	if knownCameraVendors[vid] {
		return "camera"
	}
	return ""
}

func parseLsusb(data []byte) []Device {
	var devices []Device
	now := time.Now()

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		matches := lsusbLineRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		vendorID := matches[1]
		productID := matches[2]
		description := matches[3]

		devType := classifyVendor(vendorID)
		if devType == "" {
			continue
		}

		name := strings.TrimSpace(description)
		serial := vendorID + ":" + productID

		devices = append(devices, Device{
			Name:       name,
			VendorID:   "0x" + vendorID,
			ProductID:  "0x" + productID,
			Serial:     serial,
			Type:       devType,
			DetectedAt: now,
		})
	}

	return devices
}
