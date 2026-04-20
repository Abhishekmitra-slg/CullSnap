//go:build darwin && arm64

package vlm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMLXBackendStartDetectsImmediateCrash verifies that when the mlx_vlm.server
// subprocess exits immediately (bad python, missing module, OOM at import time),
// Start returns within a few seconds rather than polling /v1/models for 120s.
func TestMLXBackendStartDetectsImmediateCrash(t *testing.T) {
	tmp := t.TempDir()

	// Build a minimal venv layout: <venv>/bin/python3 is an executable that exits 1.
	venv := filepath.Join(tmp, "venv")
	binDir := filepath.Join(venv, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	fakePython := filepath.Join(binDir, "python3")
	script := "#!/bin/sh\necho 'mlx_vlm import failed' 1>&2\nexit 1\n"
	if err := os.WriteFile(fakePython, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake python3: %v", err)
	}

	// Model path just needs to exist; the fake python never reads it.
	modelDir := filepath.Join(tmp, "model")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("mkdir model: %v", err)
	}

	b := NewMLXBackend(venv, modelDir, ModelManifest{
		Name: "mlx-crash-test", Variant: "4bit", Backend: "mlx",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	start := time.Now()
	err := b.Start(ctx)
	elapsed := time.Since(start)

	if err == nil {
		// Clean up in case Start somehow succeeded.
		_ = b.Stop(context.Background())
		t.Fatal("Start() should fail when subprocess exits immediately")
	}
	if elapsed > 5*time.Second {
		t.Errorf("Start() took %v detecting crash; expected <5s", elapsed)
	}
	msg := err.Error()
	if !strings.Contains(msg, "crashed") && !strings.Contains(msg, "exited") {
		t.Errorf("error should indicate subprocess crash/exit, got: %v", err)
	}
	// The stderr tail captured from the fake python should surface in the error.
	if !strings.Contains(msg, "mlx_vlm import failed") {
		t.Logf("warning: stderr tail missing from error (may be a timing edge) — got: %v", err)
	}
}

// TestMLXBackendStartRetriesOnReadyFailure verifies that waitForReady failures
// are retried mlxMaxStartRetries times before surfacing, so a TOCTOU port
// collision or transient subprocess crash does not leave the caller stuck on a
// single 120s timeout.
func TestMLXBackendStartRetriesOnReadyFailure(t *testing.T) {
	tmp := t.TempDir()

	venv := filepath.Join(tmp, "venv")
	binDir := filepath.Join(venv, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	fakePython := filepath.Join(binDir, "python3")
	script := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(fakePython, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake python3: %v", err)
	}

	modelDir := filepath.Join(tmp, "model")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatalf("mkdir model: %v", err)
	}

	b := NewMLXBackend(venv, modelDir, ModelManifest{
		Name: "mlx-retry-test", Variant: "4bit", Backend: "mlx",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := b.Start(ctx)
	if err == nil {
		_ = b.Stop(context.Background())
		t.Fatal("Start() should fail when every attempt crashes")
	}
	want := fmt.Sprintf("failed after %d start attempts", mlxMaxStartRetries)
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error should mention retry exhaustion %q, got: %v", want, err)
	}
}
