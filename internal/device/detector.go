package device

import (
	"context"
	"time"
)

// Device represents a detected USB device (iPhone, iPad, Android phone, camera, or storage).
type Device struct {
	Name       string    `json:"name"`
	VendorID   string    `json:"vendorID"`
	ProductID  string    `json:"productID"`
	Serial     string    `json:"serial"`
	Type       string    `json:"type"`      // "iphone", "android", "camera", "storage", "" (legacy)
	MountPath  string    `json:"mountPath"` // Filesystem mount path (GVFS on Linux, empty on macOS/Windows)
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

// DependencyStatus reports the state of optional system packages for device import.
type DependencyStatus struct {
	UsbmuxdRunning  bool     `json:"usbmuxdRunning"`
	GVFSAvailable   bool     `json:"gvfsAvailable"`
	Gphoto2Path     string   `json:"gphoto2Path"`
	IdeviceInfoPath string   `json:"ideviceInfoPath"`
	DistroID        string   `json:"distroID"`
	DistroFamily    string   `json:"distroFamily"`
	DistroName      string   `json:"distroName"`
	InstallCommand  string   `json:"installCommand"`
	MissingPackages []string `json:"missingPackages"`
}
