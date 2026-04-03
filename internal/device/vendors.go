package device

import "strings"

// knownPhoneVendors maps USB vendor IDs to the "android" device type.
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
	"0fce": true, // Sony Mobile
	"22d9": true, // OPPO
	"2d95": true, // Vivo
	"19d2": true, // ZTE
	"17ef": true, // Lenovo
	"0b05": true, // ASUS
}

// knownCameraVendors maps USB vendor IDs to the "camera" device type.
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

// classifyVendor returns a device type based on USB vendor ID.
// Returns "" for unknown vendors.
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
