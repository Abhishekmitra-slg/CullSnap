//go:build !linux && !darwin && !windows

package device

// CheckDependencies returns an all-satisfied status on unsupported platforms.
func CheckDependencies() DependencyStatus {
	return DependencyStatus{
		UsbmuxdRunning: true,
		GVFSAvailable:  true,
	}
}
