//go:build linux

package device

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type osReleaseInfo struct {
	ID         string
	IDLike     string
	PrettyName string
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

func isProcessRunning(name string) bool {
	if _, err := exec.LookPath("pgrep"); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pgrep", "-x", name)
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
