//go:build !darwin

package device

import "context"

// StubDetector is a no-op detector for non-macOS platforms.
type StubDetector struct{}

// NewDetector returns a stub detector on non-macOS platforms.
func NewDetector() Detector                         { return &StubDetector{} }
func (d *StubDetector) Start(_ context.Context)     {}
func (d *StubDetector) Stop()                       {}
func (d *StubDetector) OnConnect(_ func(Device))    {}
func (d *StubDetector) OnDisconnect(_ func(Device)) {}
func (d *StubDetector) ConnectedDevices() []Device  { return nil }
