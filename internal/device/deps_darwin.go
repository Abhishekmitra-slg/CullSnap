//go:build darwin

package device

import (
	"os/exec"
)

// CheckDependencies checks for optional system packages on macOS.
// gphoto2 is needed for Android MTP import; everything else works natively.
func CheckDependencies() DependencyStatus {
	status := DependencyStatus{
		UsbmuxdRunning: true,  // macOS has built-in usbmuxd
		GVFSAvailable:  false, // GVFS is Linux-only
	}

	if path, err := resolveSecureBinary("gphoto2"); err == nil {
		status.Gphoto2Path = path
	} else {
		status.MissingPackages = append(status.MissingPackages, "gphoto2")
		status.InstallCommand = "brew install gphoto2"
	}

	if path, err := exec.LookPath("ideviceinfo"); err == nil {
		status.IdeviceInfoPath = path
	}

	return status
}
