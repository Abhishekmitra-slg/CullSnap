package cloudsource

import (
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestTokenStore_Save_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	token := &oauth2.Token{
		AccessToken:  "save-roundtrip-access",
		RefreshToken: "save-roundtrip-refresh",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour).Truncate(time.Second),
	}

	// Save tries keychain first, falls back to encrypted file.
	// On CI (Linux), keychain is typically unavailable, so it falls back.
	if err := store.Save("test_save", token); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Load("test_save")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.AccessToken != token.AccessToken {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, token.AccessToken)
	}
	if loaded.RefreshToken != token.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", loaded.RefreshToken, token.RefreshToken)
	}
	if loaded.TokenType != token.TokenType {
		t.Errorf("TokenType = %q, want %q", loaded.TokenType, token.TokenType)
	}
}

func TestTokenStore_Load_NotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	_, err := store.Load("nonexistent_provider")
	if err == nil {
		t.Error("expected error for loading nonexistent token")
	}
}

func TestTokenStore_Save_Delete_Load(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	token := &oauth2.Token{
		AccessToken:  "delete-test",
		RefreshToken: "r",
		TokenType:    "Bearer",
	}

	if err := store.Save("delete_provider", token); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if err := store.Delete("delete_provider"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := store.Load("delete_provider")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestTokenStore_Save_Overwrite(t *testing.T) {
	dir := t.TempDir()
	store := NewTokenStore(dir)

	token1 := &oauth2.Token{AccessToken: "first", TokenType: "Bearer"}
	token2 := &oauth2.Token{AccessToken: "second", TokenType: "Bearer"}

	if err := store.Save("overwrite", token1); err != nil {
		t.Fatalf("first Save failed: %v", err)
	}
	if err := store.Save("overwrite", token2); err != nil {
		t.Fatalf("second Save failed: %v", err)
	}

	loaded, err := store.Load("overwrite")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.AccessToken != "second" {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, "second")
	}
}
