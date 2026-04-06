package scoring

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCloudProvider_Interface(t *testing.T) {
	var _ ScoringProvider = (*CloudProvider)(nil)
}

func TestCloudProvider_Name(t *testing.T) {
	p := NewCloudProvider("OpenAI Vision", "", nil)
	if p.Name() != "OpenAI Vision" {
		t.Errorf("Name = %q, want %q", p.Name(), "OpenAI Vision")
	}
}

func TestCloudProvider_RequiresAPIKey(t *testing.T) {
	p := NewCloudProvider("test", "", nil)
	if !p.RequiresAPIKey() {
		t.Error("cloud provider should require API key")
	}
}

func TestCloudProvider_RequiresDownload(t *testing.T) {
	p := NewCloudProvider("test", "", nil)
	if p.RequiresDownload() {
		t.Error("cloud provider should not require download")
	}
}

func TestCloudProvider_NotAvailableWithoutKey(t *testing.T) {
	p := NewCloudProvider("test", "", func() (string, error) {
		return "", nil // empty key
	})
	if p.Available() {
		t.Error("should not be available without API key")
	}
}

func TestCloudProvider_AvailableWithKey(t *testing.T) {
	p := NewCloudProvider("test", "", func() (string, error) {
		return "sk-test-key", nil
	})
	if !p.Available() {
		t.Error("should be available with API key")
	}
}

func TestCloudProvider_Score_MockServer(t *testing.T) {
	// Mock OpenAI-compatible server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		resp := openAIResponse{
			Choices: []openAIChoice{
				{
					Message: openAIMessage{
						Content: `{"faces": [{"bbox": [10, 20, 50, 60], "confidence": 0.95, "eye_sharpness": 0.8}], "overall_score": 0.85}`,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
	}))
	defer server.Close()

	p := NewCloudProvider("OpenAI Vision", server.URL, func() (string, error) {
		return "sk-test", nil
	})

	result, err := p.Score(context.Background(), []byte("fake-jpeg-data"))
	if err != nil {
		t.Fatalf("Score failed: %v", err)
	}
	if result == nil {
		t.Fatal("Score returned nil")
	}
	if result.OverallScore < 0.84 || result.OverallScore > 0.86 {
		t.Errorf("OverallScore = %f, want ~0.85", result.OverallScore)
	}
	if len(result.Faces) != 1 {
		t.Fatalf("expected 1 face, got %d", len(result.Faces))
	}
	if result.Faces[0].Confidence < 0.94 {
		t.Errorf("face confidence = %f, want ~0.95", result.Faces[0].Confidence)
	}
}

func TestCloudProvider_Score_NoKey(t *testing.T) {
	p := NewCloudProvider("test", "http://localhost:1", func() (string, error) {
		return "", nil
	})

	_, err := p.Score(context.Background(), []byte("fake"))
	if err == nil {
		t.Error("should fail without API key")
	}
}

func TestCloudProvider_Score_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewCloudProvider("test", server.URL, func() (string, error) {
		return "sk-test", nil
	})

	_, err := p.Score(context.Background(), []byte("fake"))
	if err == nil {
		t.Error("should fail on server error")
	}
}

func TestParseCloudResponse_Valid(t *testing.T) {
	jsonStr := `{"faces": [{"bbox": [10, 20, 100, 120], "confidence": 0.9, "eye_sharpness": 0.7, "eyes_open": true}], "overall_score": 0.8}`
	result, err := parseCloudResponse(jsonStr)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if result.OverallScore != 0.8 {
		t.Errorf("OverallScore = %f, want 0.8", result.OverallScore)
	}
	if len(result.Faces) != 1 {
		t.Fatalf("expected 1 face, got %d", len(result.Faces))
	}
	if result.Faces[0].Confidence != 0.9 {
		t.Errorf("expected Confidence = 0.9, got %f", result.Faces[0].Confidence)
	}
}

func TestParseCloudResponse_NoFaces(t *testing.T) {
	jsonStr := `{"faces": [], "overall_score": 0.3}`
	result, err := parseCloudResponse(jsonStr)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(result.Faces) != 0 {
		t.Error("expected no faces")
	}
}

func TestParseCloudResponse_Invalid(t *testing.T) {
	_, err := parseCloudResponse("not json")
	if err == nil {
		t.Error("should fail on invalid JSON")
	}
}
