package hfclient

import (
	"context"
	"cullsnap/internal/logger"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "https://huggingface.co"

// Client is a minimal HuggingFace Hub client.
type Client struct {
	httpClient *http.Client
	token      string
	userAgent  string
	baseURL    string // overridable for tests; defaults to defaultBaseURL
}

// New returns a Client. Pass "" for unauthenticated access.
func New(token string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		token:      token,
		userAgent:  "CullSnap/dev (+https://github.com/Abhishekmitra-slg/CullSnap)",
		baseURL:    defaultBaseURL,
	}
}

// SetUserAgent overrides the User-Agent header.
func (c *Client) SetUserAgent(ua string) { c.userAgent = ua }

type rawTreeEntry struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Oid  string `json:"oid"`
	Size int64  `json:"size"`
	LFS  *struct {
		Oid  string `json:"oid"`
		Size int64  `json:"size"`
	} `json:"lfs,omitempty"`
	XetHash string `json:"xetHash,omitempty"`
}

// FetchTree returns the recursive tree at revision and the resolved commit SHA.
func (c *Client) FetchTree(ctx context.Context, repo, revision string) ([]TreeEntry, string, error) {
	url := fmt.Sprintf("%s/api/models/%s/tree/%s?recursive=true", c.baseURL, repo, revision)
	if logger.Log != nil {
		logger.Log.Debug("hfclient: fetch tree", "repo", repo, "revision", revision)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("hfclient: build request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("hfclient: tree request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return nil, "", fmt.Errorf("hfclient: tree %q: 401 unauthorized (missing or invalid HF token)", repo)
	case http.StatusForbidden:
		return nil, "", fmt.Errorf("hfclient: tree %q: 403 forbidden (gated repo, accept license at https://huggingface.co/%s)", repo, repo)
	case http.StatusNotFound:
		return nil, "", fmt.Errorf("hfclient: tree %q@%q: 404 not found", repo, revision)
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", fmt.Errorf("hfclient: tree %q: status %d: %s", repo, resp.StatusCode, string(body))
	}

	var raw []rawTreeEntry
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, "", fmt.Errorf("hfclient: tree decode: %w", err)
	}

	out := make([]TreeEntry, 0, len(raw))
	for _, r := range raw {
		if r.Type != "file" {
			continue
		}
		if err := validateSiblingPath(r.Path); err != nil {
			return nil, "", err
		}
		e := TreeEntry{
			Path:    r.Path,
			Size:    r.Size,
			SHA1:    r.Oid,
			XetHash: r.XetHash,
		}
		if r.LFS != nil {
			e.IsLFS = true
			e.SHA256 = r.LFS.Oid
			e.Size = r.LFS.Size // authoritative for LFS
		}
		out = append(out, e)
	}
	commit := resp.Header.Get("X-Repo-Commit")
	if logger.Log != nil {
		logger.Log.Debug("hfclient: tree fetched", "repo", repo, "commit", commit, "files", len(out))
	}
	return out, commit, nil
}

// ResolveURL returns the resolve URL for a given file at a commit.
func (c *Client) ResolveURL(repo, commit, path string) string {
	return fmt.Sprintf("%s/%s/resolve/%s/%s", c.baseURL, repo, commit, path)
}
