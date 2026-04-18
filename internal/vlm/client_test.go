package vlm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// validResponse builds a minimal chatResponse JSON payload.
func validResponse(content string, totalTokens int) []byte {
	resp := chatResponse{}
	resp.Choices = []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}{
		{Message: struct {
			Content string `json:"content"`
		}{Content: content}},
	}
	resp.Usage.TotalTokens = totalTokens

	data, err := json.Marshal(resp)
	if err != nil {
		panic(err)
	}
	return data
}

// TestClientScorePhoto verifies auth header forwarding, model field presence,
// and that valid content + token count are returned.
func TestClientScorePhoto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Verify auth header.
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Decode body and verify model field.
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Model == "" {
			http.Error(w, "model field missing", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(validResponse("photo looks sharp and well-composed", 150))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token", "gemma-4b")
	content, tokens, err := client.ChatCompletion(
		context.Background(),
		"You are a photo scoring assistant.",
		"Score this photo.",
		nil,
		200,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content == "" {
		t.Error("expected non-empty content")
	}
	if tokens != 150 {
		t.Errorf("expected tokens=150, got %d", tokens)
	}
}

// TestClientChatCompletionWithImage verifies that a multimodal request body
// contains exactly two content parts: text + image_url.
func TestClientChatCompletionWithImage(t *testing.T) {
	// Create a real temp file so imageToDataURI can read it.
	tmp, err := os.CreateTemp(t.TempDir(), "test-*.jpg")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmp.Name())

	// Write a few bytes of content (not a real JPEG, but enough for base64 encoding).
	if _, err = tmp.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tmp.Close()

	var capturedParts []json.RawMessage

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// Find the user message and capture its content parts.
		for _, msg := range req.Messages {
			if msg.Role == "user" {
				var parts []json.RawMessage
				if jsonErr := json.Unmarshal(msg.Content, &parts); jsonErr == nil {
					capturedParts = parts
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(validResponse("image received", 80))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", "gemma-4b")
	_, _, err = client.ChatCompletion(
		context.Background(),
		"system prompt",
		"describe this image",
		[]string{tmp.Name()},
		100,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect exactly 2 parts: text + image_url.
	if len(capturedParts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(capturedParts))
	}

	// First part must have type "text".
	var textPart contentPart
	if err = json.Unmarshal(capturedParts[0], &textPart); err != nil {
		t.Fatalf("decode text part: %v", err)
	}
	if textPart.Type != "text" {
		t.Errorf("expected first part type=text, got %q", textPart.Type)
	}

	// Second part must have type "image_url" and a non-empty data URI.
	var imgPart contentPart
	if err = json.Unmarshal(capturedParts[1], &imgPart); err != nil {
		t.Fatalf("decode image part: %v", err)
	}
	if imgPart.Type != "image_url" {
		t.Errorf("expected second part type=image_url, got %q", imgPart.Type)
	}
	if imgPart.ImageURL == nil || !strings.HasPrefix(imgPart.ImageURL.URL, "data:") {
		t.Errorf("expected data URI, got %v", imgPart.ImageURL)
	}
}

// TestClientServerError verifies that a 500 response returns a non-nil error.
func TestClientServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "tok", "model")
	_, _, err := client.ChatCompletion(
		context.Background(),
		"sys",
		"user",
		nil,
		50,
	)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status 500, got: %v", err)
	}
}

// TestClientContextCancelled verifies that a cancelled context causes an error.
func TestClientContextCancelled(t *testing.T) {
	// Server that blocks long enough for cancel to fire.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wait for request context to be cancelled (client cancels before we respond).
		<-r.Context().Done()
		http.Error(w, "cancelled", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately before sending.

	client := NewClient(srv.URL, "tok", "model")
	_, _, err := client.ChatCompletion(ctx, "sys", "user", nil, 50)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}
