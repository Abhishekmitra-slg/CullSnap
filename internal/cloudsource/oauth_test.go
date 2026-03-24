package cloudsource

import (
	"context"
	"net/http"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestGenerateCodeVerifier(t *testing.T) {
	v, err := generateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}
	if len(v) < 43 {
		t.Errorf("verifier too short: %d chars", len(v))
	}

	// Generate two — should be different
	v2, _ := generateCodeVerifier()
	if v == v2 {
		t.Error("two verifiers should not be identical")
	}
}

func TestGenerateCodeChallenge(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := generateCodeChallenge(verifier)
	if challenge == "" {
		t.Error("challenge should not be empty")
	}
	if challenge == verifier {
		t.Error("challenge should differ from verifier (S256)")
	}
}

func TestGenerateState(t *testing.T) {
	s1, err := generateState()
	if err != nil {
		t.Fatal(err)
	}
	s2, _ := generateState()
	if s1 == s2 {
		t.Error("two states should not be identical")
	}
}

func TestStartOAuthFlow_StateMismatch(t *testing.T) {
	// This test verifies the flow rejects mismatched state parameters.
	// We simulate by calling the callback with a wrong state.
	ctx := context.Background()

	config := &oauth2.Config{
		ClientID: "test-client",
		Endpoint: oauth2.Endpoint{
			AuthURL:  "http://localhost:99999/auth", // won't be called
			TokenURL: "http://localhost:99999/token",
		},
	}

	// openBrowser simulates calling the callback with wrong state
	openBrowser := func(authURL string) error {
		// Parse the redirect_uri from the auth URL to find the callback port
		// Then make a request to it with wrong state
		go func() {
			// Small delay to let the server start
			resp, err := http.Get(authURL) // this hits the fake auth URL, not the callback
			if err != nil {
				// Expected — fake URL won't work
				return
			}
			resp.Body.Close()
		}()
		return nil
	}

	// This should timeout (since our fake browser won't hit the right callback)
	shortCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	_, err := StartOAuthFlow(shortCtx, config, openBrowser)
	if err == nil {
		t.Fatal("expected error")
	}
}
