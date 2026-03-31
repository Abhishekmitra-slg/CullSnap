//go:build !darwin && !windows && !linux

package device

import (
	"context"
	"fmt"
)

// ImportFromDevice is not available on this platform.
func ImportFromDevice(_ context.Context, _, _ string) (string, int, error) {
	return "", 0, fmt.Errorf("device import is not available on this platform")
}
