//go:build windows

package scoring

import (
	"context"
	"fmt"
)

// ONNXRuntimeLibName returns the platform-specific library filename.
// On Windows this is a placeholder since ONNX runtime is not supported.
func ONNXRuntimeLibName() string {
	return "onnxruntime.dll"
}

// ProvisionONNXRuntime is not supported on Windows.
func ProvisionONNXRuntime(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("ONNX runtime provisioning not supported on Windows")
}
