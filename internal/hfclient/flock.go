package hfclient

import (
	"context"
	"fmt"
	"time"

	"github.com/gofrs/flock"
)

// withFileLock acquires a POSIX flock on target+".lock" for the duration of fn.
// Polls every 100ms; honors ctx cancellation.
func withFileLock(ctx context.Context, target string, fn func() error) error {
	lk := flock.New(target + ".lock")
	locked, err := lk.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("hfclient: lock %q: %w", target, err)
	}
	if !locked {
		return fmt.Errorf("hfclient: lock %q: not acquired", target)
	}
	defer func() { _ = lk.Unlock() }()
	return fn()
}
