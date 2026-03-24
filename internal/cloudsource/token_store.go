package cloudsource

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"cullsnap/internal/logger"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

const keychainService = "CullSnap"

// TokenStore stores OAuth2 tokens securely. It tries OS keychain first
// (via go-keyring), falls back to an AES-256-GCM encrypted file.
type TokenStore struct {
	fallbackDir string // for encrypted file fallback
}

// NewTokenStore creates a TokenStore that uses the given directory for
// encrypted-file fallback when the OS keychain is unavailable.
func NewTokenStore(fallbackDir string) *TokenStore {
	return &TokenStore{fallbackDir: fallbackDir}
}

// Save persists an OAuth2 token for the given provider. Tries keychain
// first, falls back to AES-256-GCM encrypted file.
func (ts *TokenStore) Save(providerID string, token *oauth2.Token) error {
	data, err := json.Marshal(token) //nolint:gosec // token must be marshaled for encrypted persistence
	if err != nil {
		return fmt.Errorf("token_store: marshal failed: %w", err)
	}

	// Try keychain first
	err = keyring.Set(keychainService, providerID, string(data))
	if err == nil {
		logger.Log.Debug("token_store: saved to keychain", "provider", providerID)
		return nil
	}
	logger.Log.Debug("token_store: keychain unavailable, using encrypted file", "provider", providerID, "error", err)

	// Fallback: encrypt and write to file
	return ts.saveEncrypted(providerID, data)
}

// Load retrieves a previously-saved OAuth2 token for the given provider.
func (ts *TokenStore) Load(providerID string) (*oauth2.Token, error) {
	// Try keychain first
	data, err := keyring.Get(keychainService, providerID)
	if err == nil {
		var token oauth2.Token
		if err := json.Unmarshal([]byte(data), &token); err != nil {
			return nil, fmt.Errorf("token_store: unmarshal failed: %w", err)
		}
		logger.Log.Debug("token_store: loaded from keychain", "provider", providerID)
		return &token, nil
	}

	// Fallback: read encrypted file
	return ts.loadEncrypted(providerID)
}

// Delete removes the token for the given provider from both keychain
// and encrypted file storage.
func (ts *TokenStore) Delete(providerID string) error {
	// Delete from keychain (ignore errors — may not exist)
	_ = keyring.Delete(keychainService, providerID)

	// Delete encrypted file if exists
	tokenPath := ts.tokenPath(providerID)
	_ = os.Remove(tokenPath)

	logger.Log.Debug("token_store: deleted", "provider", providerID)
	return nil
}

func (ts *TokenStore) tokenPath(providerID string) string {
	return filepath.Join(ts.fallbackDir, SanitizeID(providerID)+".enc")
}

func (ts *TokenStore) keyPath() string {
	return filepath.Join(ts.fallbackDir, ".key")
}

// getOrCreateKey loads or generates the encryption key (32 bytes, random).
func (ts *TokenStore) getOrCreateKey() ([]byte, error) {
	keyFile := ts.keyPath()

	data, err := os.ReadFile(keyFile)
	if err == nil && len(data) == 32 {
		return data, nil
	}

	// Generate new random key
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("token_store: failed to generate key: %w", err)
	}

	if err := os.MkdirAll(ts.fallbackDir, 0o700); err != nil {
		return nil, fmt.Errorf("token_store: failed to create dir: %w", err)
	}

	if err := os.WriteFile(keyFile, key, 0o600); err != nil {
		return nil, fmt.Errorf("token_store: failed to write key: %w", err)
	}

	logger.Log.Info("token_store: generated new encryption key (keychain unavailable)")
	return key, nil
}

func (ts *TokenStore) saveEncrypted(providerID string, plaintext []byte) error {
	key, err := ts.getOrCreateKey()
	if err != nil {
		return err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("token_store: cipher init failed: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("token_store: GCM init failed: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("token_store: nonce generation failed: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	if err := os.MkdirAll(ts.fallbackDir, 0o700); err != nil {
		return err
	}

	return os.WriteFile(ts.tokenPath(providerID), ciphertext, 0o600)
}

func (ts *TokenStore) loadEncrypted(providerID string) (*oauth2.Token, error) {
	key, err := ts.getOrCreateKey()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(ts.tokenPath(providerID))
	if err != nil {
		return nil, fmt.Errorf("token_store: no token found for %s: %w", providerID, err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("token_store: cipher init failed: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("token_store: GCM init failed: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("token_store: encrypted data too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("token_store: decryption failed: %w", err)
	}

	var token oauth2.Token
	if err := json.Unmarshal(plaintext, &token); err != nil {
		return nil, fmt.Errorf("token_store: unmarshal failed: %w", err)
	}

	return &token, nil
}
