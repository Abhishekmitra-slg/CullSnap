package cloudsource

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"time"

	"cullsnap/internal/logger"

	"golang.org/x/oauth2"
)

const oauthTimeout = 5 * time.Minute

// generateCodeVerifier creates a PKCE code_verifier (43-128 random chars).
func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// generateCodeChallenge creates S256 code_challenge from verifier.
func generateCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// generateState creates a random state parameter.
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// OAuthResult holds the authorization code and state from the callback.
type OAuthResult struct {
	Code  string
	State string
	Error string
}

// StartOAuthFlow performs the OAuth2 authorization code flow with PKCE.
// It opens the system browser, starts a loopback listener, waits for the callback,
// and exchanges the code for tokens.
// Returns the obtained token or an error.
func StartOAuthFlow(ctx context.Context, config *oauth2.Config, openBrowser func(string) error) (*oauth2.Token, error) {
	// Start ephemeral loopback listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("oauth: failed to start listener: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	config.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/oauth/callback", port)

	// Generate PKCE and state
	verifier, err := generateCodeVerifier()
	if err != nil {
		return nil, fmt.Errorf("oauth: failed to generate verifier: %w", err)
	}
	challenge := generateCodeChallenge(verifier)

	state, err := generateState()
	if err != nil {
		return nil, fmt.Errorf("oauth: failed to generate state: %w", err)
	}

	// Build auth URL
	authURL := config.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)

	// Channel for result from callback
	resultCh := make(chan OAuthResult, 1)

	// Single-use HTTP handler
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		result := OAuthResult{
			Code:  r.URL.Query().Get("code"),
			State: r.URL.Query().Get("state"),
			Error: r.URL.Query().Get("error"),
		}

		if result.Error != "" {
			tmpl := template.Must(template.New("err").Parse(
				`<html><body><h2>Authorization Failed</h2><p>{{.}}</p><p>You can close this tab.</p></body></html>`))
			tmpl.Execute(w, result.Error) //nolint:errcheck
		} else {
			w.Write([]byte("<html><body><h2>Authorization Successful</h2><p>You can close this tab and return to CullSnap.</p></body></html>")) //nolint:errcheck
		}

		resultCh <- result
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener) //nolint:errcheck // shutdown handled below

	// Open browser
	logger.Log.Debug("oauth: opening browser for authorization", "url", authURL)
	if err := openBrowser(authURL); err != nil {
		return nil, fmt.Errorf("oauth: failed to open browser: %w", err)
	}

	// Wait for callback with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, oauthTimeout)
	defer cancel()

	var result OAuthResult
	select {
	case result = <-resultCh:
		// Got callback
	case <-timeoutCtx.Done():
		server.Shutdown(context.Background()) //nolint:errcheck
		return nil, fmt.Errorf("oauth: timed out waiting for authorization")
	}

	// Shutdown listener immediately (single-use)
	server.Shutdown(context.Background()) //nolint:errcheck

	if result.Error != "" {
		return nil, fmt.Errorf("oauth: authorization denied: %s", result.Error)
	}

	// Validate state
	if result.State != state {
		return nil, fmt.Errorf("oauth: state mismatch (possible CSRF)")
	}

	// Exchange code for token with PKCE verifier
	token, err := config.Exchange(ctx, result.Code,
		oauth2.SetAuthURLParam("code_verifier", verifier),
	)
	if err != nil {
		return nil, fmt.Errorf("oauth: token exchange failed: %w", err)
	}

	logger.Log.Debug("oauth: authorization complete")
	return token, nil
}
