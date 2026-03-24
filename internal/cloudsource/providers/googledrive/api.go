package googledrive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"cullsnap/internal/logger"
)

const driveAPIBase = "https://www.googleapis.com/drive/v3"

// progressWriter wraps an io.Writer and reports progress via a callback.
type progressWriter struct {
	dest       io.Writer
	written    int64
	total      int64
	progressFn func(int64, int64)
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.dest.Write(p)
	pw.written += int64(n)
	if pw.progressFn != nil {
		pw.progressFn(pw.written, pw.total)
	}
	return n, err
}

// DriveClient wraps the Google Drive v3 REST API.
type DriveClient struct {
	httpClient *http.Client
	baseURL    string // overridable for testing
}

// NewDriveClient creates a client with the given authenticated HTTP client.
func NewDriveClient(httpClient *http.Client) *DriveClient {
	return &DriveClient{httpClient: httpClient, baseURL: driveAPIBase}
}

// FileListResponse mirrors the Drive API files.list response.
type FileListResponse struct {
	Files         []DriveFile `json:"files"`
	NextPageToken string      `json:"nextPageToken"`
}

// DriveFile mirrors a Drive API file resource.
type DriveFile struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	MimeType     string              `json:"mimeType"`
	Size         string              `json:"size"` // string in API, parse to int64
	ModifiedTime string              `json:"modifiedTime"`
	CreatedTime  string              `json:"createdTime"`
	Parents      []string            `json:"parents"`
	ImageMedia   *ImageMediaMetadata `json:"imageMediaMetadata,omitempty"`
}

// ImageMediaMetadata holds image dimensions from the Drive API.
type ImageMediaMetadata struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// ListFolders lists top-level folders in the user's Drive.
func (c *DriveClient) ListFolders(ctx context.Context, pageToken string) (*FileListResponse, error) {
	params := url.Values{
		"q":        {"mimeType='application/vnd.google-apps.folder' and trashed=false and 'root' in parents"},
		"fields":   {"files(id,name,modifiedTime),nextPageToken"},
		"pageSize": {"100"},
	}
	if pageToken != "" {
		params.Set("pageToken", pageToken)
	}
	return c.listFiles(ctx, params)
}

// ListImagesInFolder lists image files in a specific folder.
func (c *DriveClient) ListImagesInFolder(ctx context.Context, folderID, pageToken string) (*FileListResponse, error) {
	q := fmt.Sprintf("'%s' in parents and mimeType contains 'image/' and trashed=false", folderID)
	params := url.Values{
		"q":        {q},
		"fields":   {"files(id,name,mimeType,size,modifiedTime,createdTime,imageMediaMetadata),nextPageToken"},
		"pageSize": {"1000"},
	}
	if pageToken != "" {
		params.Set("pageToken", pageToken)
	}
	return c.listFiles(ctx, params)
}

func (c *DriveClient) listFiles(ctx context.Context, params url.Values) (*FileListResponse, error) {
	reqURL := fmt.Sprintf("%s/files?%s", c.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("drive: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("drive: rate limited (HTTP 429)")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("drive: API error %d: %s", resp.StatusCode, string(body))
	}

	var result FileListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("drive: decode response failed: %w", err)
	}
	return &result, nil
}

// DownloadFile downloads a file by ID to the given writer.
func (c *DriveClient) DownloadFile(ctx context.Context, fileID string, dest io.Writer, progressFn func(int64, int64)) error {
	reqURL := fmt.Sprintf("%s/files/%s?alt=media", c.baseURL, url.PathEscape(fileID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("drive: download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("drive: download error %d: %s", resp.StatusCode, string(body))
	}

	pw := &progressWriter{
		dest:      dest,
		total:     resp.ContentLength,
		progressFn: progressFn,
	}

	written, err := io.Copy(pw, resp.Body)
	if err != nil {
		return fmt.Errorf("drive: copy failed: %w", err)
	}

	logger.Log.Debug("drive: download complete", "fileID", fileID, "bytes", written)
	return nil
}
