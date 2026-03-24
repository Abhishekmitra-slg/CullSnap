package googledrive

import (
	"context"
	"cullsnap/internal/cloudsource"
	"cullsnap/internal/logger"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/browser"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Provider implements cloudsource.CloudSource for Google Drive.
type Provider struct {
	tokenStore  *cloudsource.TokenStore
	oauthConfig *oauth2.Config
	client      *DriveClient
	token       *oauth2.Token
	mu          sync.Mutex
}

// New creates a Google Drive provider.
func New(tokenStore *cloudsource.TokenStore, clientID, clientSecret string) *Provider {
	return &Provider{
		tokenStore: tokenStore,
		oauthConfig: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       []string{"https://www.googleapis.com/auth/drive.readonly"},
			Endpoint:     google.Endpoint,
		},
	}
}

func (p *Provider) ID() string          { return "google_drive" }
func (p *Provider) DisplayName() string { return "Google Drive" }
func (p *Provider) IsAvailable() bool   { return p.oauthConfig.ClientID != "" }

func (p *Provider) IsAuthenticated() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != nil {
		return true
	}
	// Try loading from token store
	token, err := p.tokenStore.Load("google_drive")
	if err == nil {
		p.token = token
		return true
	}
	return false
}

func (p *Provider) Authenticate(ctx context.Context) error {
	token, err := cloudsource.StartOAuthFlow(ctx, p.oauthConfig, func(authURL string) error {
		logger.Log.Info("drive: opening browser for OAuth", "url", authURL)
		return browser.OpenURL(authURL)
	})
	if err != nil {
		return fmt.Errorf("drive: authentication failed: %w", err)
	}

	p.mu.Lock()
	p.token = token
	p.client = nil // force re-create with new token
	p.mu.Unlock()

	if err := p.tokenStore.Save("google_drive", token); err != nil {
		logger.Log.Error("drive: failed to persist token", "error", err)
	}

	return nil
}

func (p *Provider) getClient(ctx context.Context) *DriveClient {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client == nil && p.token != nil {
		ts := p.oauthConfig.TokenSource(ctx, p.token)
		p.client = NewDriveClient(oauth2.NewClient(ctx, ts))
	}
	return p.client
}

func (p *Provider) ListAlbums(ctx context.Context) ([]cloudsource.Album, error) {
	client := p.getClient(ctx)
	if client == nil {
		return nil, fmt.Errorf("drive: not authenticated")
	}

	var albums []cloudsource.Album
	pageToken := ""
	for {
		resp, err := client.ListFolders(ctx, pageToken)
		if err != nil {
			return nil, err
		}
		for i := range resp.Files {
			f := &resp.Files[i]
			modTime, _ := time.Parse(time.RFC3339, f.ModifiedTime)
			albums = append(albums, cloudsource.Album{
				ID:        f.ID,
				Title:     f.Name,
				UpdatedAt: modTime,
			})
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	logger.Log.Debug("drive: listed folders", "count", len(albums))
	return albums, nil
}

func (p *Provider) ListMediaInAlbum(ctx context.Context, albumID string) ([]cloudsource.RemoteMedia, error) {
	client := p.getClient(ctx)
	if client == nil {
		return nil, fmt.Errorf("drive: not authenticated")
	}

	var media []cloudsource.RemoteMedia
	pageToken := ""
	for {
		resp, err := client.ListImagesInFolder(ctx, albumID, pageToken)
		if err != nil {
			return nil, err
		}
		for i := range resp.Files {
			f := &resp.Files[i]
			size, _ := strconv.ParseInt(f.Size, 10, 64)
			created, _ := time.Parse(time.RFC3339, f.CreatedTime)
			modified, _ := time.Parse(time.RFC3339, f.ModifiedTime)
			media = append(media, cloudsource.RemoteMedia{
				ID:        f.ID,
				Filename:  f.Name,
				MimeType:  f.MimeType,
				SizeBytes: size,
				CreatedAt: created,
				UpdatedAt: modified,
			})
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	logger.Log.Debug("drive: listed media", "albumID", albumID, "count", len(media))
	return media, nil
}

func (p *Provider) Download(ctx context.Context, media cloudsource.RemoteMedia, localPath string, progressFn func(int64, int64)) error {
	client := p.getClient(ctx)
	if client == nil {
		return fmt.Errorf("drive: not authenticated")
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0o700); err != nil {
		return err
	}

	f, err := os.OpenFile(localPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}

	dlErr := client.DownloadFile(ctx, media.ID, f, progressFn)
	if closeErr := f.Close(); closeErr != nil && dlErr == nil {
		return fmt.Errorf("drive: close file failed: %w", closeErr)
	}
	return dlErr
}

func (p *Provider) Disconnect() error {
	p.mu.Lock()
	p.token = nil
	p.client = nil
	p.mu.Unlock()
	return p.tokenStore.Delete("google_drive")
}
