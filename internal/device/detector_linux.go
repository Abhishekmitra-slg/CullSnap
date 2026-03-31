//go:build linux

package device

import (
	"context"
	"log"
	"sync"
	"time"
)

const linuxPollInterval = 5 * time.Second

// LinuxDetector polls for USB devices on Linux using lsusb.
type LinuxDetector struct {
	mu           sync.Mutex
	connected    map[string]Device
	onConnect    func(Device)
	onDisconnect func(Device)
	stopCh       chan struct{}
}

// NewDetector returns a Linux USB detector backed by lsusb polling.
func NewDetector() Detector {
	return &LinuxDetector{
		connected: make(map[string]Device),
		stopCh:    make(chan struct{}),
	}
}

func (d *LinuxDetector) OnConnect(fn func(Device)) { d.mu.Lock(); d.onConnect = fn; d.mu.Unlock() }
func (d *LinuxDetector) OnDisconnect(fn func(Device)) {
	d.mu.Lock()
	d.onDisconnect = fn
	d.mu.Unlock()
}

func (d *LinuxDetector) ConnectedDevices() []Device {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]Device, 0, len(d.connected))
	for _, dev := range d.connected {
		out = append(out, dev)
	}
	return out
}

func (d *LinuxDetector) Stop() {
	select {
	case <-d.stopCh:
	default:
		close(d.stopCh)
	}
}

func (d *LinuxDetector) Start(ctx context.Context) {
	log.Println("[device/linux] detector started")
	ticker := time.NewTicker(linuxPollInterval)
	defer ticker.Stop()

	d.poll()

	for {
		select {
		case <-ctx.Done():
			log.Println("[device/linux] detector stopped (context cancelled)")
			return
		case <-d.stopCh:
			log.Println("[device/linux] detector stopped")
			return
		case <-ticker.C:
			d.poll()
		}
	}
}

func (d *LinuxDetector) poll() {
	devices, err := runLsusb()
	if err != nil {
		log.Printf("[device/linux] lsusb error: %v", err)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	seen := make(map[string]bool)
	for _, dev := range devices {
		key := dev.VendorID + ":" + dev.ProductID
		seen[key] = true
		if _, exists := d.connected[key]; !exists {
			d.connected[key] = dev
			log.Printf("[device/linux] device connected: %s (%s)", dev.Name, dev.Type)
			if d.onConnect != nil {
				go d.onConnect(dev)
			}
		}
	}

	for key, dev := range d.connected {
		if !seen[key] {
			delete(d.connected, key)
			log.Printf("[device/linux] device disconnected: %s (%s)", dev.Name, dev.Type)
			if d.onDisconnect != nil {
				go d.onDisconnect(dev)
			}
		}
	}
}
