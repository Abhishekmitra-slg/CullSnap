package hfclient

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestWithFileLockBasic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x")
	called := false
	err := withFileLock(context.Background(), target, func() error {
		called = true
		return nil
	})
	if err != nil || !called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}

func TestWithFileLockBlocking(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x")
	holdReleased := make(chan struct{})
	go func() {
		_ = withFileLock(context.Background(), target, func() error {
			time.Sleep(200 * time.Millisecond)
			return nil
		})
		close(holdReleased)
	}()
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	err := withFileLock(context.Background(), target, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 100*time.Millisecond {
		t.Fatal("second lock acquired too quickly — flock not blocking")
	}
	<-holdReleased
}
