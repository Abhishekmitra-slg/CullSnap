//go:build linux

package device

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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

type osReleaseInfo struct {
	ID         string
	IDLike     string
	PrettyName string
}

var allowedBinDirs = []string{
	"/usr/bin/",
	"/usr/local/bin/",
	"/usr/sbin/",
	"/bin/",
	"/sbin/",
}

func CheckDependencies() DependencyStatus {
	status := DependencyStatus{}

	osRelease, err := os.ReadFile("/etc/os-release")
	if err != nil {
		osRelease, err = os.ReadFile("/usr/lib/os-release")
	}
	if err == nil {
		info := parseOSRelease(osRelease)
		status.DistroID = info.ID
		status.DistroName = info.PrettyName
		status.DistroFamily = detectDistroFamily(info.ID, info.IDLike)
		status.InstallCommand = installCommandForFamily(status.DistroFamily)
	} else {
		status.DistroFamily = "unknown"
	}

	uid := os.Getuid()
	gvfsDir := fmt.Sprintf("/run/user/%d/gvfs", uid)
	if info, err := os.Stat(gvfsDir); err == nil && info.IsDir() {
		status.GVFSAvailable = true
	}

	if isProcessRunning("usbmuxd") {
		status.UsbmuxdRunning = true
	}

	if path, err := resolveSecureBinary("gphoto2"); err == nil {
		status.Gphoto2Path = path
	} else {
		status.MissingPackages = append(status.MissingPackages, "gphoto2")
	}

	if path, err := resolveSecureBinary("ideviceinfo"); err == nil {
		status.IdeviceInfoPath = path
	} else {
		status.MissingPackages = append(status.MissingPackages, "libimobiledevice-utils")
	}

	if !status.UsbmuxdRunning {
		if _, err := resolveSecureBinary("usbmuxd"); err != nil {
			status.MissingPackages = append(status.MissingPackages, "usbmuxd")
		}
	}

	return status
}

func resolveSecureBinary(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found: %w", name, err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("cannot resolve absolute path for %s: %w", name, err)
	}

	realPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("cannot resolve symlinks for %s: %w", name, err)
	}

	for _, prefix := range allowedBinDirs {
		if strings.HasPrefix(realPath, prefix) {
			return realPath, nil
		}
	}

	return "", fmt.Errorf("%s found at %s which is not in a trusted directory", name, realPath)
}

func isProcessRunning(name string) bool {
	cmd := exec.Command("pgrep", "-x", name)
	return cmd.Run() == nil
}

func parseOSRelease(data []byte) osReleaseInfo {
	var info osReleaseInfo
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value = strings.Trim(value, `"'`)
		switch key {
		case "ID":
			info.ID = value
		case "ID_LIKE":
			info.IDLike = value
		case "PRETTY_NAME":
			info.PrettyName = value
		}
	}
	return info
}

func detectDistroFamily(id, idLike string) string {
	id = strings.ToLower(id)
	idLike = strings.ToLower(idLike)
	allIDs := id + " " + idLike

	debianIDs := []string{"debian", "ubuntu", "linuxmint", "pop", "elementary", "zorin", "kali"}
	for _, d := range debianIDs {
		if strings.Contains(allIDs, d) {
			return "debian"
		}
	}

	fedoraIDs := []string{"fedora", "rhel", "centos", "rocky", "alma", "nobara"}
	for _, d := range fedoraIDs {
		if strings.Contains(allIDs, d) {
			return "fedora"
		}
	}

	archIDs := []string{"arch", "manjaro", "endeavouros", "garuda", "artix"}
	for _, d := range archIDs {
		if strings.Contains(allIDs, d) {
			return "arch"
		}
	}

	suseIDs := []string{"opensuse", "suse", "sles"}
	for _, d := range suseIDs {
		if strings.Contains(allIDs, d) {
			return "suse"
		}
	}

	return "unknown"
}

func installCommandForFamily(family string) string {
	switch family {
	case "debian":
		return "sudo apt install libimobiledevice-utils usbmuxd gphoto2"
	case "fedora":
		return "sudo dnf install libimobiledevice-utils usbmuxd gphoto2"
	case "arch":
		return "sudo pacman -S libimobiledevice usbmuxd gphoto2"
	case "suse":
		return "sudo zypper install libimobiledevice-utils usbmuxd gphoto2"
	default:
		return ""
	}
}
