package cloudsource

import (
	"context"
	"time"
)

// CloudSource is implemented by each cloud provider.
type CloudSource interface {
	ID() string
	DisplayName() string
	IsAvailable() bool
	Authenticate(ctx context.Context) error
	IsAuthenticated() bool
	ListAlbums(ctx context.Context) ([]Album, error)
	ListMediaInAlbum(ctx context.Context, albumID string) ([]RemoteMedia, error)
	Download(ctx context.Context, media RemoteMedia, localPath string,
		progressFn func(int64, int64)) error
	Disconnect() error
}

type Album struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	MediaCount int       `json:"mediaCount"`
	CoverURL   string    `json:"coverURL"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type RemoteMedia struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	MimeType    string    `json:"mimeType"`
	SizeBytes   int64     `json:"sizeBytes"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	DownloadURL string    `json:"-"` // never sent to frontend
}

type CloudSourceStatus struct {
	ProviderID  string `json:"providerID"`
	DisplayName string `json:"displayName"`
	IsAvailable bool   `json:"isAvailable"`
	IsConnected bool   `json:"isConnected"`
}
