package device

import (
	"context"
	"time"
)

// Device represents a detected USB device (iPhone or iPad).
type Device struct {
	Name       string    `json:"name"`
	VendorID   string    `json:"vendorID"`
	ProductID  string    `json:"productID"`
	Serial     string    `json:"serial"`
	DetectedAt time.Time `json:"detectedAt"`
}

// Detector watches for USB device connect/disconnect events.
type Detector interface {
	// Start begins polling for device changes. Blocks until ctx is cancelled.
	Start(ctx context.Context)
	// Stop signals the detector to cease polling.
	Stop()
	// OnConnect registers a callback fired when a new device is detected.
	OnConnect(fn func(Device))
	// OnDisconnect registers a callback fired when a device is removed.
	OnDisconnect(fn func(Device))
	// ConnectedDevices returns the currently connected devices.
	ConnectedDevices() []Device
}
