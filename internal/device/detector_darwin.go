//go:build darwin

package device

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"sync"
	"time"

	"cullsnap/internal/logger"
)

const (
	pollInterval  = 5 * time.Second
	appleVendorID = "0x05ac"
)

// spUSBData is the top-level structure from system_profiler SPUSBDataType -json.
type spUSBData struct {
	SPUSBDataType []spUSBBus `json:"SPUSBDataType"`
}

// spUSBBus represents a USB bus entry.
type spUSBBus struct {
	Name  string      `json:"_name"`
	Items []spUSBItem `json:"_items"`
}

// spUSBItem represents a USB device or hub entry, which may contain nested items.
type spUSBItem struct {
	Name      string      `json:"_name"`
	VendorID  string      `json:"vendor_id"`
	ProductID string      `json:"product_id"`
	SerialNum string      `json:"serial_num"`
	Items     []spUSBItem `json:"_items"`
}

// DarwinDetector polls system_profiler on macOS to detect iPhone/iPad connections.
type DarwinDetector struct {
	mu           sync.RWMutex
	devices      map[string]Device // keyed by serial
	onConnect    []func(Device)
	onDisconnect []func(Device)
	cancel       context.CancelFunc
}

// NewDetector creates a new macOS USB device detector.
func NewDetector() Detector {
	return &DarwinDetector{
		devices: make(map[string]Device),
	}
}

// Start begins polling system_profiler for USB device changes.
// It blocks until the context is cancelled or Stop is called.
func (d *DarwinDetector) Start(ctx context.Context) {
	ctx, d.cancel = context.WithCancel(ctx)
	logger.Log.Info("device: detector started, polling every 5s")

	// Do an immediate poll before entering the ticker loop.
	d.poll(ctx)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Log.Info("device: detector stopped")
			return
		case <-ticker.C:
			d.poll(ctx)
		}
	}
}

// Stop signals the detector to stop polling.
func (d *DarwinDetector) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
}

// OnConnect registers a callback fired when a new device is detected.
func (d *DarwinDetector) OnConnect(fn func(Device)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onConnect = append(d.onConnect, fn)
}

// OnDisconnect registers a callback fired when a device is removed.
func (d *DarwinDetector) OnDisconnect(fn func(Device)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onDisconnect = append(d.onDisconnect, fn)
}

// ConnectedDevices returns a snapshot of currently connected devices.
func (d *DarwinDetector) ConnectedDevices() []Device {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := make([]Device, 0, len(d.devices))
	for _, dev := range d.devices {
		result = append(result, dev)
	}
	return result
}

// poll runs system_profiler and reconciles the result with known devices.
func (d *DarwinDetector) poll(ctx context.Context) {
	out, err := runSystemProfiler(ctx)
	if err != nil {
		logger.Log.Error("device: system_profiler failed", "error", err)
		return
	}

	found, err := ParseUSBDevices(out)
	if err != nil {
		logger.Log.Error("device: failed to parse system_profiler output", "error", err)
		return
	}

	d.reconcile(found)
}

// reconcile compares newly found devices against known state and fires callbacks.
func (d *DarwinDetector) reconcile(found []Device) {
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

// runSystemProfiler executes system_profiler and returns its stdout.
func runSystemProfiler(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "system_profiler", "SPUSBDataType", "-json")
	return cmd.Output()
}

// ParseUSBDevices parses system_profiler SPUSBDataType JSON output and returns
// Apple iPhone/iPad devices found at any nesting level.
func ParseUSBDevices(data []byte) ([]Device, error) {
	var spData spUSBData
	if err := json.Unmarshal(data, &spData); err != nil {
		return nil, err
	}

	var devices []Device
	now := time.Now()
	for _, bus := range spData.SPUSBDataType {
		devices = collectAppleDevices(bus.Items, devices, now)
	}
	return devices, nil
}

// collectAppleDevices recursively walks USB items to find iPhone/iPad devices.
func collectAppleDevices(items []spUSBItem, devices []Device, now time.Time) []Device {
	for _, item := range items {
		if isAppleMobileDevice(item) {
			devices = append(devices, Device{
				Name:       item.Name,
				VendorID:   normalizeVendorID(item.VendorID),
				ProductID:  item.ProductID,
				Serial:     item.SerialNum,
				DetectedAt: now,
			})
		}
		// Recurse into nested hubs/items regardless of whether this item matched.
		if len(item.Items) > 0 {
			devices = collectAppleDevices(item.Items, devices, now)
		}
	}
	return devices
}

// isAppleMobileDevice returns true if the item is an Apple iPhone or iPad.
func isAppleMobileDevice(item spUSBItem) bool {
	vid := normalizeVendorID(item.VendorID)
	if vid != appleVendorID {
		return false
	}
	nameLower := strings.ToLower(item.Name)
	return strings.Contains(nameLower, "iphone") || strings.Contains(nameLower, "ipad")
}

// normalizeVendorID strips vendor name suffixes like "0x05ac (Apple Inc.)".
func normalizeVendorID(raw string) string {
	if idx := strings.Index(raw, " "); idx != -1 {
		return raw[:idx]
	}
	return raw
}
