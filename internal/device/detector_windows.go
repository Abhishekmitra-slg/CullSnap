//go:build windows

package device

import (
	"context"
	"cullsnap/internal/logger"
	"sync"
	"time"
)

const pollInterval = 5 * time.Second

// detectScript is the PowerShell script that enumerates connected portable
// devices via the Windows Portable Devices (WPD) PnP class. It matches any
// device with a USB Vendor ID and emits a JSON array to stdout. Device type
// classification (iPhone, Android, camera) is handled by parseWPDDevices.
const detectScript = `
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$devices = Get-PnpDevice -Class 'WPD' -Status 'OK' -ErrorAction SilentlyContinue |
    Where-Object {
        $_.InstanceId -match 'VID_[0-9A-Fa-f]+'
    }

if (-not $devices) {
    Write-Output '[]'
    exit 0
}

$result = @()
foreach ($dev in $devices) {
    $props = Get-PnpDeviceProperty -InstanceId $dev.InstanceId
    $serial = ($props | Where-Object KeyName -eq 'DEVPKEY_Device_Serial').Data

    $vid = ''
    $pid = ''
    if ($dev.InstanceId -match 'VID_([0-9A-Fa-f]+)') { $vid = '0x' + $Matches[1].ToLower() }
    if ($dev.InstanceId -match 'PID_([0-9A-Fa-f]+)') { $pid = '0x' + $Matches[1].ToLower() }

    $result += @{
        name      = $dev.FriendlyName
        serial    = if ($serial) { [string]$serial } else { '' }
        vendorID  = $vid
        productID = $pid
    }
}

$result | ConvertTo-Json -Compress
`

// storageDetectScript detects removable drives with DCIM folders.
const storageDetectScript = `
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

$drives = Get-WmiObject Win32_LogicalDisk -Filter "DriveType=2" -ErrorAction SilentlyContinue |
    Where-Object { Test-Path "$($_.DeviceID)\DCIM" }

if (-not $drives) {
    Write-Output '[]'
    exit 0
}

$result = @()
foreach ($d in $drives) {
    $label = if ($d.VolumeName) { $d.VolumeName } else { $d.DeviceID }
    $result += @{
        name   = $label
        path   = $d.DeviceID
        serial = "storage:$($d.DeviceID)"
    }
}

$result | ConvertTo-Json -Compress
`

// WindowsDetector polls Windows Portable Devices (WPD) and removable drives via
// PowerShell to detect phones, cameras, and storage devices.
type WindowsDetector struct {
	mu           sync.RWMutex
	devices      map[string]Device // keyed by serial
	onConnect    []func(Device)
	onDisconnect []func(Device)
	cancel       context.CancelFunc
}

var detectorInstance *WindowsDetector

// NewDetector creates a new Windows USB device detector.
func NewDetector() Detector {
	d := &WindowsDetector{
		devices: make(map[string]Device),
	}
	detectorInstance = d
	return d
}

// lookupDeviceBySerial returns a device from the detector's state by serial number.
func lookupDeviceBySerial(serial string) (Device, bool) {
	if detectorInstance == nil {
		return Device{}, false
	}
	detectorInstance.mu.RLock()
	defer detectorInstance.mu.RUnlock()
	dev, ok := detectorInstance.devices[serial]
	return dev, ok
}

// Start begins polling WPD for USB device changes.
// It blocks until the context is cancelled or Stop is called.
func (d *WindowsDetector) Start(ctx context.Context) {
	ctx, d.cancel = context.WithCancel(ctx)
	logger.Log.Info("device: Windows detector started, polling every 5s")

	// Do an immediate poll before entering the ticker loop.
	d.poll(ctx)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Log.Info("device: Windows detector stopped")
			return
		case <-ticker.C:
			d.poll(ctx)
		}
	}
}

// Stop signals the detector to stop polling.
func (d *WindowsDetector) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
}

// OnConnect registers a callback fired when a new device is detected.
func (d *WindowsDetector) OnConnect(fn func(Device)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onConnect = append(d.onConnect, fn)
}

// OnDisconnect registers a callback fired when a device is removed.
func (d *WindowsDetector) OnDisconnect(fn func(Device)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onDisconnect = append(d.onDisconnect, fn)
}

// ConnectedDevices returns a snapshot of currently connected devices.
func (d *WindowsDetector) ConnectedDevices() []Device {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := make([]Device, 0, len(d.devices))
	for _, dev := range d.devices {
		result = append(result, dev)
	}
	return result
}

// poll runs the WPD and storage detection scripts and reconciles results.
func (d *WindowsDetector) poll(ctx context.Context) {
	var found []Device

	// WPD devices (phones, cameras)
	out, err := runPowerShell(ctx, detectScript)
	if err != nil {
		logger.Log.Error("device: WPD PowerShell poll failed", "error", err)
	} else {
		wpdDevices, parseErr := parseWPDDevices(out)
		if parseErr != nil {
			logger.Log.Error("device: failed to parse WPD devices output", "error", parseErr)
		} else {
			found = append(found, wpdDevices...)
		}
	}

	// Removable drives with DCIM
	storageOut, storageErr := runPowerShell(ctx, storageDetectScript)
	if storageErr != nil {
		logger.Log.Debug("device: storage detection failed", "error", storageErr)
	} else {
		found = append(found, parseStorageDevices(storageOut)...)
	}

	d.reconcile(found)
}

// reconcile compares newly found devices against known state and fires callbacks.
func (d *WindowsDetector) reconcile(found []Device) {
	d.mu.Lock()

	// Build a set of found serials for quick lookup.
	foundMap := make(map[string]Device, len(found))
	for _, dev := range found {
		foundMap[dev.Serial] = dev
	}

	// Detect disconnects: devices we knew about that are no longer present.
	var disconnected []Device
	for serial, dev := range d.devices {
		if _, ok := foundMap[serial]; !ok {
			disconnected = append(disconnected, dev)
			delete(d.devices, serial)
		}
	}

	// Detect connects: devices found that we didn't know about.
	var connected []Device
	for serial, dev := range foundMap {
		if _, ok := d.devices[serial]; !ok {
			d.devices[serial] = dev
			connected = append(connected, dev)
		}
	}

	// Copy callback slices under lock so we can call them outside the lock.
	onConnect := make([]func(Device), len(d.onConnect))
	copy(onConnect, d.onConnect)
	onDisconnect := make([]func(Device), len(d.onDisconnect))
	copy(onDisconnect, d.onDisconnect)

	d.mu.Unlock()

	// Fire callbacks outside the lock to avoid deadlocks.
	for _, dev := range disconnected {
		logger.Log.Info("device: disconnected", "name", dev.Name, "serial", dev.Serial)
		for _, fn := range onDisconnect {
			fn(dev)
		}
	}
	for _, dev := range connected {
		logger.Log.Info("device: connected", "name", dev.Name, "serial", dev.Serial)
		for _, fn := range onConnect {
			fn(dev)
		}
	}
}
