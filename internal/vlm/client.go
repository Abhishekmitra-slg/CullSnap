package vlm

import (
	"bytes"
	"context"
	"cullsnap/internal/logger"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	clientTimeout    = 120 * time.Second
	maxResponseBytes = 1 << 20 // 1 MB
	defaultTemp      = 0.1
)

// Client is an OpenAI-compatible HTTP client for VLM inference.
type Client struct {
	baseURL    string
	authToken  string
	modelName  string
	httpClient *http.Client
}

// NewClient creates a Client with a 120-second timeout.
func NewClient(baseURL, authToken, modelName string) *Client {
	return &Client{
		baseURL:   baseURL,
		authToken: authToken,
		modelName: modelName,
		httpClient: &http.Client{
			Timeout: clientTimeout,
		},
	}
}

// --- internal request / response types ---

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
}

// chatMessage holds a single chat turn.
// Content can be a plain string (text-only) or []contentPart (multimodal).
type chatMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// ChatCompletion sends a chat completion request to the OpenAI-compatible endpoint.
// If imagePaths is non-nil the user message is built as a multimodal content array
// containing a text part followed by one image_url part per path.
func (c *Client) ChatCompletion(
	ctx context.Context,
	systemPrompt, userPrompt string,
	imagePaths []string,
	maxTokens int,
) (content string, tokens int, err error) {
	if logger.Log != nil {
		logger.Log.Debug("vlm: client: ChatCompletion starting",
			slog.String("model", c.modelName),
			slog.String("baseURL", c.baseURL),
			slog.Int("imagePaths", len(imagePaths)),
			slog.Int("maxTokens", maxTokens),
		)
	}

	msgs := []chatMessage{
		{Role: "system", Content: systemPrompt},
	}

	if len(imagePaths) > 0 {
		parts := make([]contentPart, 0, 1+len(imagePaths))
		parts = append(parts, contentPart{Type: "text", Text: userPrompt})

		for _, p := range imagePaths {
			uri, uriErr := imageToDataURI(p)
			if uriErr != nil {
				return "", 0, fmt.Errorf("vlm: client: encode image %q: %w", p, uriErr)
			}
			parts = append(parts, contentPart{
				Type:     "image_url",
				ImageURL: &imageURL{URL: uri},
			})
		}

		msgs = append(msgs, chatMessage{Role: "user", Content: parts})
	} else {
		msgs = append(msgs, chatMessage{Role: "user", Content: userPrompt})
	}

	reqBody := chatRequest{
		Model:       c.modelName,
		Messages:    msgs,
		MaxTokens:   maxTokens,
		Temperature: defaultTemp,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", 0, fmt.Errorf("vlm: client: marshal request: %w", err)
	}

	endpoint := c.baseURL + "/v1/chat/completions"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", 0, fmt.Errorf("vlm: client: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: client: sending request", slog.String("endpoint", endpoint))
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", 0, fmt.Errorf("vlm: client: http do: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // HTTP response body close

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", 0, fmt.Errorf("vlm: client: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if logger.Log != nil {
			logger.Log.Debug("vlm: client: server error",
				slog.Int("status", resp.StatusCode),
				slog.String("body", string(body)),
			)
		}
		return "", 0, fmt.Errorf("vlm: client: server returned %d: %s", resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err = json.Unmarshal(body, &chatResp); err != nil {
		return "", 0, fmt.Errorf("vlm: client: decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", 0, fmt.Errorf("vlm: client: no choices in response")
	}

	content = chatResp.Choices[0].Message.Content
	tokens = chatResp.Usage.TotalTokens

	if logger.Log != nil {
		logger.Log.Debug("vlm: client: ChatCompletion complete",
			slog.Int("tokens", tokens),
			slog.Int("contentLen", len(content)),
		)
	}

	return content, tokens, nil
}

// imageToDataURI reads the file at path, base64-encodes it, and returns a
// data URI suitable for the OpenAI image_url content part.
const maxImageBytes = 5 * 1024 * 1024 // 5MB — thumbnails should be well under this

func imageToDataURI(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat file: %w", err)
	}
	if info.Size() > maxImageBytes {
		return "", fmt.Errorf("image too large (%d bytes > %d max) — use a thumbnail", info.Size(), maxImageBytes)
	}

	data, err := os.ReadFile(path) // #nosec G304 — caller-supplied paths, desktop app
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	ext := filepath.Ext(path)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		// fall back to octet-stream rather than rejecting the file
		mimeType = "application/octet-stream"
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	uri := fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)

	if logger.Log != nil {
		logger.Log.Debug("vlm: client: encoded image to data URI",
			slog.String("path", path),
			slog.String("mimeType", mimeType),
			slog.Int("encodedLen", len(encoded)),
		)
	}

	return uri, nil
}
