package cloudsource

import (
	"cullsnap/internal/logger"
	"encoding/json"
	"os"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestMain(m *testing.M) {
	logger.Init("/dev/null")
	os.Exit(m.Run())
}

func TestTokenStore_EncryptedFallback_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	token := &oauth2.Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	}

	if err := store.saveEncrypted("test_provider", mustMarshal(t, token)); err != nil {
		t.Fatalf("saveEncrypted failed: %v", err)
	}

	loaded, err := store.loadEncrypted("test_provider")
	if err != nil {
		t.Fatalf("loadEncrypted failed: %v", err)
	}

	if loaded.AccessToken != token.AccessToken {
		t.Errorf("AccessToken mismatch: got %q, want %q", loaded.AccessToken, token.AccessToken)
	}
	if loaded.RefreshToken != token.RefreshToken {
		t.Errorf("RefreshToken mismatch: got %q, want %q", loaded.RefreshToken, token.RefreshToken)
	}
}

func TestTokenStore_Delete(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	token := &oauth2.Token{AccessToken: "test", RefreshToken: "test", TokenType: "Bearer"}
	if err := store.saveEncrypted("test_provider", mustMarshal(t, token)); err != nil {
		t.Fatalf("saveEncrypted failed: %v", err)
	}

	if err := store.Delete("test_provider"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := store.loadEncrypted("test_provider")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestTokenStore_CorruptedData(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	// Create a key first
	if _, err := store.getOrCreateKey(); err != nil {
		t.Fatalf("getOrCreateKey failed: %v", err)
	}

	// Write garbage as encrypted token
	if err := os.WriteFile(store.tokenPath("bad_provider"), []byte("garbage"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err := store.loadEncrypted("bad_provider")
	if err == nil {
		t.Fatal("expected error for corrupted data")
	}
}

func TestTokenStore_KeyPersistence(t *testing.T) {
	dir := t.TempDir()

	// Create two stores pointing to same dir — should use same key
	store1 := NewTokenStore(dir)
	store2 := NewTokenStore(dir)

	token := &oauth2.Token{AccessToken: "persist-test", RefreshToken: "r", TokenType: "Bearer"}
	if err := store1.saveEncrypted("provider1", mustMarshal(t, token)); err != nil {
		t.Fatalf("saveEncrypted failed: %v", err)
	}

	loaded, err := store2.loadEncrypted("provider1")
	if err != nil {
		t.Fatalf("second store couldn't load: %v", err)
	}
	if loaded.AccessToken != "persist-test" {
		t.Errorf("got %q, want %q", loaded.AccessToken, "persist-test")
	}
}

func mustMarshal(t *testing.T, token *oauth2.Token) []byte {
	t.Helper()
	data, err := json.Marshal(token)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
