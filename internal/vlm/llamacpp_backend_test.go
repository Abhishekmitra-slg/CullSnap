package vlm

import (
	"testing"
)

func TestLlamaCppBackendName(t *testing.T) {
	b := NewLlamaCppBackend("/some/llama-server", "/some/model.gguf", ModelEntry{
		Name:    "gemma-4-e4b-it",
		Variant: "Q4_K_M",
		Backend: "llamacpp",
	})
	if got := b.Name(); got != "llamacpp" {
		t.Errorf("Name() = %q, want %q", got, "llamacpp")
	}
}

func TestLlamaCppBackendModelInfo(t *testing.T) {
	entry := ModelEntry{
		Name:      "gemma-4-e4b-it",
		Variant:   "Q4_K_M",
		Backend:   "llamacpp",
		SizeBytes: 2_800_000_000,
		RAMUsage:  3_200_000_000,
	}
	b := NewLlamaCppBackend("/some/llama-server", "/some/model.gguf", entry)
	info := b.ModelInfo()

	if info.Name != entry.Name {
		t.Errorf("ModelInfo().Name = %q, want %q", info.Name, entry.Name)
	}
	if info.Backend != "llamacpp" {
		t.Errorf("ModelInfo().Backend = %q, want %q", info.Backend, "llamacpp")
	}
	if info.MaxImages != llamaCppMaxImages {
		t.Errorf("ModelInfo().MaxImages = %d, want %d", info.MaxImages, llamaCppMaxImages)
	}
	if len(info.TokenBudgets) != len(llamaCppDefaultTokenBudgets) {
		t.Errorf("ModelInfo().TokenBudgets len = %d, want %d", len(info.TokenBudgets), len(llamaCppDefaultTokenBudgets))
	}
}

func TestLlamaCppBackendStartMissingBinary(t *testing.T) {
	b := NewLlamaCppBackend("/nonexistent/llama-server", "/some/model.gguf", ModelEntry{
		Name:    "gemma-4-e4b-it",
		Variant: "Q4_K_M",
		Backend: "llamacpp",
	})

	ctx := t.Context()
	err := b.Start(ctx)
	if err == nil {
		t.Fatal("Start() with missing binary should return an error, got nil")
	}
}

func TestGenerateSessionToken(t *testing.T) {
	tok1, err := generateSessionToken()
	if err != nil {
		t.Fatalf("generateSessionToken() error: %v", err)
	}
	if len(tok1) != 64 {
		t.Errorf("token length = %d, want 64", len(tok1))
	}

	tok2, err := generateSessionToken()
	if err != nil {
		t.Fatalf("generateSessionToken() second call error: %v", err)
	}
	if tok1 == tok2 {
		t.Errorf("two generated tokens are identical: %q — expected them to differ", tok1)
	}
}

func TestFindFreePort(t *testing.T) {
	port, err := findFreePort()
	if err != nil {
		t.Fatalf("findFreePort() error: %v", err)
	}
	if port < 1024 || port > 65535 {
		t.Errorf("port %d out of expected range [1024, 65535]", port)
	}
}
