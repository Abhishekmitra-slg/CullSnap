//go:build linux

package device

import (
	"context"
	"fmt"
)

// ImportFromDevice is not yet implemented on Linux.
func ImportFromDevice(_ context.Context, _, _ string) (string, int, error) {
	return "", 0, fmt.Errorf("device import is not yet available on Linux")
}
