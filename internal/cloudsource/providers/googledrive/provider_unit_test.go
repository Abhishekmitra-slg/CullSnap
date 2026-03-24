package googledrive

import (
	"context"
	"cullsnap/internal/cloudsource"
	"testing"
)

func TestNew(t *testing.T) {
	ts := cloudsource.NewTokenStore(t.TempDir())
	p := New(ts, "client-id", "client-secret")
	if p == nil {
		t.Fatal("New returned nil")
	}
}

func TestProvider_ID(t *testing.T) {
	ts := cloudsource.NewTokenStore(t.TempDir())
	p := New(ts, "", "")
	if got := p.ID(); got != "google_drive" {
		t.Errorf("ID() = %q, want %q", got, "google_drive")
	}
}

func TestProvider_DisplayName(t *testing.T) {
	ts := cloudsource.NewTokenStore(t.TempDir())
	p := New(ts, "", "")
	if got := p.DisplayName(); got != "Google Drive" {
		t.Errorf("DisplayName() = %q, want %q", got, "Google Drive")
	}
}

func TestProvider_IsAvailable_NoCredentials(t *testing.T) {
	ts := cloudsource.NewTokenStore(t.TempDir())
	p := New(ts, "", "")
	if p.IsAvailable() {
		t.Error("IsAvailable() should be false with empty client ID")
	}
}

func TestProvider_IsAvailable_WithCredentials(t *testing.T) {
	ts := cloudsource.NewTokenStore(t.TempDir())
	p := New(ts, "some-client-id", "some-secret")
	if !p.IsAvailable() {
		t.Error("IsAvailable() should be true with client ID set")
	}
}

func TestProvider_IsAuthenticated_NoToken(t *testing.T) {
	ts := cloudsource.NewTokenStore(t.TempDir())
	p := New(ts, "id", "secret")
	if p.IsAuthenticated() {
		t.Error("IsAuthenticated() should be false with no saved token")
	}
}

func TestProvider_ListAlbums_NotAuthenticated(t *testing.T) {
	ts := cloudsource.NewTokenStore(t.TempDir())
	p := New(ts, "id", "secret")
	_, err := p.ListAlbums(context.Background())
	if err == nil {
		t.Fatal("expected error for unauthenticated ListAlbums")
	}
}

func TestProvider_ListMediaInAlbum_NotAuthenticated(t *testing.T) {
	ts := cloudsource.NewTokenStore(t.TempDir())
	p := New(ts, "id", "secret")
	_, err := p.ListMediaInAlbum(context.Background(), "album1")
	if err == nil {
		t.Fatal("expected error for unauthenticated ListMediaInAlbum")
	}
}

func TestProvider_Download_NotAuthenticated(t *testing.T) {
	ts := cloudsource.NewTokenStore(t.TempDir())
	p := New(ts, "id", "secret")
	err := p.Download(context.Background(), cloudsource.RemoteMedia{ID: "test"}, "/tmp/test", nil)
	if err == nil {
		t.Fatal("expected error for unauthenticated Download")
	}
}

func TestProvider_Disconnect(t *testing.T) {
	ts := cloudsource.NewTokenStore(t.TempDir())
	p := New(ts, "id", "secret")
	err := p.Disconnect()
	if err != nil {
		t.Errorf("Disconnect error: %v", err)
	}
	if p.IsAuthenticated() {
		t.Error("should not be authenticated after disconnect")
	}
}

func TestProvider_ImplementsCloudSource(t *testing.T) {
	var _ cloudsource.CloudSource = (*Provider)(nil)
}

func TestNewDriveClient(t *testing.T) {
	c := NewDriveClient(nil)
	if c == nil {
		t.Fatal("NewDriveClient returned nil")
	}
	if c.baseURL != driveAPIBase {
		t.Errorf("baseURL = %q, want %q", c.baseURL, driveAPIBase)
	}
}
