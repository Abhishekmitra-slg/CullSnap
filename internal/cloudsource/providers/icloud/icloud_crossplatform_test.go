package icloud

import (
	"context"
	"cullsnap/internal/cloudsource"
	"testing"
)

func TestProviderStub_ID(t *testing.T) {
	p := New(nil)
	if p.ID() != "icloud" {
		t.Errorf("ID = %q, want %q", p.ID(), "icloud")
	}
}

func TestProviderStub_DisplayName(t *testing.T) {
	p := New(nil)
	if p.DisplayName() != "iCloud Photos" {
		t.Errorf("DisplayName = %q, want %q", p.DisplayName(), "iCloud Photos")
	}
}

func TestProviderStub_IsAvailable(t *testing.T) {
	p := New(nil)
	// On non-darwin: false. On darwin: true or false depending on system.
	// Just verify no panic.
	_ = p.IsAvailable()
}

func TestProviderStub_IsAuthenticated(t *testing.T) {
	p := New(nil)
	_ = p.IsAuthenticated()
}

func TestProviderStub_Disconnect(t *testing.T) {
	p := New(nil)
	err := p.Disconnect()
	if err != nil {
		t.Errorf("Disconnect error: %v", err)
	}
}

func TestProviderStub_Authenticate(t *testing.T) {
	p := New(nil)
	// On non-darwin, this should return an error.
	// On darwin, behavior depends on system state.
	// Just verify no panic.
	_ = p.Authenticate(context.Background())
}

func TestProviderStub_ListAlbums(t *testing.T) {
	p := New(nil)
	_, err := p.ListAlbums(context.Background())
	// Non-darwin returns error; darwin may succeed or fail.
	// Just verify no panic.
	_ = err
}

func TestProviderStub_ListMediaInAlbum(t *testing.T) {
	p := New(nil)
	_, err := p.ListMediaInAlbum(context.Background(), "test-album")
	_ = err
}

func TestProviderStub_Download(t *testing.T) {
	p := New(nil)
	media := cloudsource.RemoteMedia{ID: "test", Filename: "test.jpg"}
	err := p.Download(context.Background(), media, "/tmp/test.jpg", nil)
	_ = err
}

func TestProviderImplementsCloudSourceInterface(t *testing.T) {
	var _ cloudsource.CloudSource = (*Provider)(nil)
}

func TestProviderImplementsSequentialDownloader(t *testing.T) {
	var _ cloudsource.SequentialDownloader = (*Provider)(nil)
}
