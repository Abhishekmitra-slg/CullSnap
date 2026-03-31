//go:build windows

package device

// CheckDependencies returns an all-satisfied status on Windows.
// Windows has native MTP/PTP support via WPD — no extra packages needed.
func CheckDependencies() DependencyStatus {
	return DependencyStatus{
		UsbmuxdRunning: true,
		GVFSAvailable:  false,
	}
}
