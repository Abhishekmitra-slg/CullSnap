//go:build !linux

package device

// CheckDependencies returns an all-satisfied status on non-Linux platforms.
func CheckDependencies() DependencyStatus {
	return DependencyStatus{
		UsbmuxdRunning: true,
		GVFSAvailable:  true,
	}
}
