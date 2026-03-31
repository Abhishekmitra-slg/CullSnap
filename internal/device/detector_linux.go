//go:build linux

package device

import (
	"context"
	"cullsnap/internal/logger"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const pollInterval = 5 * time.Second

// detectorInstance holds a reference to the active detector so that
// ImportFromDevice can look up device mount paths by serial.
var detectorInstance *LinuxDetector

// LinuxDetector polls GVFS mounts, lsusb, and mass storage for device changes.
type LinuxDetector struct {
	mu           sync.RWMutex
	devices      map[string]Device
	onConnect    []func(Device)
	onDisconnect []func(Device)
	cancel       context.CancelFunc
	gvfsDir      string
	mediaDir     string
}

// NewDetector creates a new Linux USB device detector.
func NewDetector() Detector {
	uid := os.Getuid()
	gvfsDir := fmt.Sprintf("/run/user/%d/gvfs", uid)

	mediaDir := ""
	if u, err := user.Current(); err == nil {
		mediaDir = "/media/" + u.Username
	}

	d := &LinuxDetector{
		devices:  make(map[string]Device),
		gvfsDir:  gvfsDir,
		mediaDir: mediaDir,
	}
	detectorInstance = d
	return d
}

func (d *LinuxDetector) Start(ctx context.Context) {
	ctx, d.cancel = context.WithCancel(ctx)
	logger.Log.Info("device: Linux detector started, polling every 5s")

	d.poll(ctx)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Log.Info("device: Linux detector stopped")
			return
		case <-ticker.C:
			d.poll(ctx)
		}
	}
}

func (d *LinuxDetector) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
}

func (d *LinuxDetector) OnConnect(fn func(Device)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onConnect = append(d.onConnect, fn)
}

func (d *LinuxDetector) OnDisconnect(fn func(Device)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onDisconnect = append(d.onDisconnect, fn)
}

func (d *LinuxDetector) ConnectedDevices() []Device {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := make([]Device, 0, len(d.devices))
	for _, dev := range d.devices {
		result = append(result, dev)
	}
	return result
}

func (d *LinuxDetector) poll(ctx context.Context) {
	var found []Device

	// Tier 1: GVFS mount scanning
	found = append(found, parseGVFSMounts(d.gvfsDir)...)

	// Tier 2: lsusb parsing
	lsusbCtx, lsusbCancel := context.WithTimeout(ctx, 30*time.Second)
	defer lsusbCancel()
	if out, err := exec.CommandContext(lsusbCtx, "lsusb").Output(); err == nil {
		found = append(found, parseLsusb(out)...)
	} else {
		logger.Log.Debug("device: lsusb failed", "error", err)
	}

	// Tier 3: Mass storage scanning
	if d.mediaDir != "" {
		found = append(found, scanMassStorage(d.mediaDir)...)
	}

	d.reconcile(deduplicateDevices(found))
}

func (d *LinuxDetector) reconcile(found []Device) {
	d.mu.Lock()

	foundMap := make(map[string]Device, len(found))
	for _, dev := range found {
		foundMap[dev.Serial] = dev
	}

	var disconnected []Device
	for serial, dev := range d.devices {
		if _, ok := foundMap[serial]; !ok {
			disconnected = append(disconnected, dev)
			delete(d.devices, serial)
		}
	}

	var connected []Device
	for serial, dev := range foundMap {
		if _, ok := d.devices[serial]; !ok {
			d.devices[serial] = dev
			connected = append(connected, dev)
		}
	}

	onConnect := make([]func(Device), len(d.onConnect))
	copy(onConnect, d.onConnect)
	onDisconnect := make([]func(Device), len(d.onDisconnect))
	copy(onDisconnect, d.onDisconnect)

	d.mu.Unlock()

	for _, dev := range disconnected {
		logger.Log.Info("device: disconnected", "name", dev.Name, "serial", dev.Serial)
		for _, fn := range onDisconnect {
			fn(dev)
		}
	}
	for _, dev := range connected {
		logger.Log.Info("device: connected", "name", dev.Name, "serial", dev.Serial, "type", dev.Type)
		for _, fn := range onConnect {
			fn(dev)
		}
	}
}

func parseGVFSMounts(gvfsDir string) []Device {
	entries, err := os.ReadDir(gvfsDir)
	if err != nil {
		return nil
	}

	now := time.Now()
	var devices []Device

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		mountPath := filepath.Join(gvfsDir, name)

		var devType, serial, devName string

		switch {
		case strings.HasPrefix(name, "afc:host="):
			devType = "iphone"
			serial = strings.TrimPrefix(name, "afc:host=")
			devName = "Apple iPhone"
		case strings.HasPrefix(name, "mtp:host="):
			devType = "android"
			serial = strings.TrimPrefix(name, "mtp:host=")
			devName = formatGVFSDeviceName(serial)
		case strings.HasPrefix(name, "gphoto2:host="):
			devType = "camera"
			serial = strings.TrimPrefix(name, "gphoto2:host=")
			devName = formatGVFSDeviceName(serial)
		default:
			continue
		}

		if idx := strings.Index(serial, ","); idx != -1 {
			serial = serial[:idx]
		}

		devices = append(devices, Device{
			Name:       devName,
			Serial:     serial,
			Type:       devType,
			MountPath:  mountPath,
			DetectedAt: now,
		})
	}

	return devices
}

func formatGVFSDeviceName(hostID string) string {
	return strings.ReplaceAll(hostID, "_", " ")
}

func scanMassStorage(mediaDir string) []Device {
	entries, err := os.ReadDir(mediaDir)
	if err != nil {
		return nil
	}

	now := time.Now()
	var devices []Device

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		mountPath := filepath.Join(mediaDir, entry.Name())
		dcimPath := filepath.Join(mountPath, "DCIM")

		if info, err := os.Stat(dcimPath); err == nil && info.IsDir() {
			devices = append(devices, Device{
				Name:       entry.Name(),
				Serial:     "storage:" + entry.Name(),
				Type:       "storage",
				MountPath:  mountPath,
				DetectedAt: now,
			})
		}
	}

	return devices
}

func deduplicateDevices(devices []Device) []Device {
	seen := make(map[string]Device)
	for _, dev := range devices {
		existing, ok := seen[dev.Serial]
		if !ok {
			seen[dev.Serial] = dev
			continue
		}
		if existing.MountPath == "" && dev.MountPath != "" {
			seen[dev.Serial] = dev
		}
	}

	result := make([]Device, 0, len(seen))
	for _, dev := range seen {
		result = append(result, dev)
	}
	return result
}

func validateGVFSPath(path string, uid int) bool {
	expectedPrefix := fmt.Sprintf("/run/user/%d/gvfs/", uid)
	cleaned := filepath.Clean(path)
	return strings.HasPrefix(cleaned, expectedPrefix)
}

func lookupDeviceBySerial(serial string) (Device, bool) {
	if detectorInstance == nil {
		return Device{}, false
	}
	detectorInstance.mu.RLock()
	defer detectorInstance.mu.RUnlock()
	dev, ok := detectorInstance.devices[serial]
	return dev, ok
}
