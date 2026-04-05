//go:build windows

package scoring

import (
	"context"
	"fmt"
)

// ProvisionONNXRuntime is not supported on Windows.
func ProvisionONNXRuntime(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("ONNX runtime provisioning not supported on Windows")
}
