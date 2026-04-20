//go:build integration

package hfclient

import (
	"context"
	"testing"
	"time"
)

func TestIntegrationFetchTreeMLXCommunity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := New("")
	entries, commit, err := c.FetchTree(ctx, "mlx-community/gemma-4-E2B-it-4bit", "main")
	if err != nil {
		t.Fatalf("FetchTree: %v", err)
	}
	if commit == "" || len(commit) != 40 {
		t.Fatalf("commit: %q", commit)
	}
	if len(entries) < 5 {
		t.Fatalf("entries: %d", len(entries))
	}
	var sawSafetensors bool
	for _, e := range entries {
		if e.Path == "model.safetensors" && e.IsLFS && len(e.SHA256) == 64 {
			sawSafetensors = true
		}
	}
	if !sawSafetensors {
		t.Fatal("model.safetensors with SHA-256 not found")
	}
}
