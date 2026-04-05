package scoring

import (
	"bytes"
	"context"
	"cullsnap/internal/logger"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"net/http"
	"time"
)

const (
	cloudRequestTimeout = 30 * time.Second
	defaultEndpoint     = "https://api.openai.com/v1/chat/completions"
	defaultModel        = "gpt-4o-mini"
)

// CloudProvider implements ScoringProvider using a cloud vision API (OpenAI-compatible).
type CloudProvider struct {
	name       string
	endpoint   string
	apiKeyFunc func() (string, error)
	httpClient *http.Client
}

// NewCloudProvider creates a cloud scoring provider.
// endpoint can be empty to use the default OpenAI endpoint.
// apiKeyFunc retrieves the API key (typically from OS keychain).
func NewCloudProvider(name, endpoint string, apiKeyFunc func() (string, error)) *CloudProvider {
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	return &CloudProvider{
		name:       name,
		endpoint:   endpoint,
		apiKeyFunc: apiKeyFunc,
		httpClient: &http.Client{Timeout: cloudRequestTimeout},
	}
}

func (p *CloudProvider) Name() string           { return p.name }
func (p *CloudProvider) RequiresAPIKey() bool   { return true }
func (p *CloudProvider) RequiresDownload() bool { return false }

// Available reports whether an API key is configured.
func (p *CloudProvider) Available() bool {
	if p.apiKeyFunc == nil {
		return false
	}
	key, err := p.apiKeyFunc()
	return err == nil && key != ""
}

// Score sends an image to the cloud vision API for face analysis.
func (p *CloudProvider) Score(ctx context.Context, imgData []byte) (*ScoreResult, error) {
	key, err := p.apiKeyFunc()
	if err != nil || key == "" {
		return nil, fmt.Errorf("no API key configured")
	}

	// Encode image as base64 for the API.
	b64 := base64.StdEncoding.EncodeToString(imgData)

	reqBody := openAIRequest{
		Model: defaultModel,
		Messages: []openAIMessage{
			{
				Role: "user",
				Content: []openAIContentPart{
					{
						Type: "text",
						Text: `Analyze this photo for face quality. Return ONLY valid JSON (no markdown):
{"faces": [{"bbox": [x1, y1, x2, y2], "confidence": 0.0-1.0, "eye_sharpness": 0.0-1.0, "eyes_open": true/false, "expression": 0.0-1.0}], "overall_score": 0.0-1.0}
bbox is pixel coordinates. overall_score considers face clarity, expression, and composition. If no faces, return empty faces array.`,
					},
					{
						Type: "image_url",
						ImageURL: &openAIImageURL{
							URL:    "data:image/jpeg;base64," + b64,
							Detail: "low",
						},
					},
				},
			},
		},
		MaxTokens: 300,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	logger.Log.Debug("scoring: sending cloud request", "provider", p.name)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloud API request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // HTTP response body

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cloud API error: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var apiResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("cloud API returned no choices")
	}

	content, ok := apiResp.Choices[0].Message.Content.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected response content type")
	}
	logger.Log.Debug("scoring: cloud response", "content", content)

	return parseCloudResponse(content)
}

// parseCloudResponse parses the JSON response from the cloud vision API.
func parseCloudResponse(content string) (*ScoreResult, error) {
	var resp cloudScoreResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("parse cloud response: %w", err)
	}

	result := &ScoreResult{
		OverallScore: resp.OverallScore,
		Confidence:   resp.OverallScore,
	}

	for _, face := range resp.Faces {
		bb := image.Rectangle{}
		if len(face.BBox) == 4 {
			bb = image.Rect(
				int(face.BBox[0]),
				int(face.BBox[1]),
				int(face.BBox[2]),
				int(face.BBox[3]),
			)
		}

		result.Faces = append(result.Faces, FaceRegion{
			BoundingBox:  bb,
			EyeSharpness: face.EyeSharpness,
			EyesOpen:     face.EyesOpen,
			Expression:   face.Expression,
			Confidence:   face.Confidence,
		})
	}

	return result, nil
}

// OpenAI API request/response types.

type openAIRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens"`
}

type openAIMessage struct {
	Role    string      `json:"role,omitempty"`
	Content interface{} `json:"content"` // string for response, []openAIContentPart for request
}

type openAIContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *openAIImageURL `json:"image_url,omitempty"`
}

type openAIImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail"`
}

type openAIResponse struct {
	Choices []openAIChoice `json:"choices"`
}

type openAIChoice struct {
	Message openAIMessage `json:"message"`
}

// Cloud response schema (what we ask the LLM to return).
type cloudScoreResponse struct {
	Faces        []cloudFace `json:"faces"`
	OverallScore float64     `json:"overall_score"`
}

type cloudFace struct {
	BBox         []float64 `json:"bbox"`
	Confidence   float64   `json:"confidence"`
	EyeSharpness float64   `json:"eye_sharpness"`
	EyesOpen     bool      `json:"eyes_open"`
	Expression   float64   `json:"expression"`
}
