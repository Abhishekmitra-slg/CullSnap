//go:build !linux

package device

// DependencyStatus reports the state of optional system packages for device import.
// On non-Linux platforms, all dependencies are considered satisfied.
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

// CheckDependencies returns an all-satisfied status on non-Linux platforms.
func CheckDependencies() DependencyStatus {
	return DependencyStatus{
		UsbmuxdRunning: true,
		GVFSAvailable:  true,
	}
}
