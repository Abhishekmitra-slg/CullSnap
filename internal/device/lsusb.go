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
