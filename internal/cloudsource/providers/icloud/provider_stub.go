//go:build !darwin

package icloud

import (
	"context"
	"fmt"

	"cullsnap/internal/cloudsource"
)

// Provider is a stub for non-macOS platforms where iCloud Photos is unavailable.
type Provider struct{}

// New creates a stub iCloud Photos provider.
func New(_ *cloudsource.TokenStore) *Provider { return &Provider{} }

func (p *Provider) ID() string          { return "icloud" }
func (p *Provider) DisplayName() string { return "iCloud Photos" }
func (p *Provider) IsAvailable() bool   { return false }
func (p *Provider) IsAuthenticated() bool { return false }

func (p *Provider) Authenticate(_ context.Context) error {
	return fmt.Errorf("iCloud is only available on macOS")
}

func (p *Provider) ListAlbums(_ context.Context) ([]cloudsource.Album, error) {
	return nil, fmt.Errorf("iCloud is only available on macOS")
}

func (p *Provider) ListMediaInAlbum(_ context.Context, _ string) ([]cloudsource.RemoteMedia, error) {
	return nil, fmt.Errorf("iCloud is only available on macOS")
}

func (p *Provider) Download(_ context.Context, _ cloudsource.RemoteMedia, _ string, _ func(int64, int64)) error {
	return fmt.Errorf("iCloud is only available on macOS")
}

func (p *Provider) Disconnect() error { return nil }
