package googledrive

import (
	"bytes"
	"context"
	"cullsnap/internal/logger"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	logger.Init("/dev/null") //nolint:errcheck // test init
	os.Exit(m.Run())
}

func newTestClient(server *httptest.Server) *DriveClient {
	return &DriveClient{
		httpClient: server.Client(),
		baseURL:    server.URL,
	}
}

// writeJSON writes a JSON string to the response writer in test handlers.
// Uses io.Copy to satisfy static analysis (no direct ResponseWriter.Write).
func writeJSON(w http.ResponseWriter, jsonStr string) {
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, strings.NewReader(jsonStr)) //nolint:errcheck // test helper
}

func TestListFolders(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/files") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		callCount++
		if callCount == 1 {
			writeJSON(w, `{
				"files": [
					{"id": "folder1", "name": "Vacation 2024", "modifiedTime": "2024-06-15T10:30:00Z"}
				],
				"nextPageToken": "page2token"
			}`)
		} else {
			writeJSON(w, `{
				"files": [
					{"id": "folder2", "name": "Wedding", "modifiedTime": "2024-07-20T14:00:00Z"}
				]
			}`)
		}
	}))
	defer server.Close()

	client := newTestClient(server)
	ctx := context.Background()

	// Simulate pagination loop like provider does
	var allFolders []DriveFile
	pageToken := ""
	for {
		resp, err := client.ListFolders(ctx, pageToken)
		if err != nil {
			t.Fatalf("ListFolders failed: %v", err)
		}
		allFolders = append(allFolders, resp.Files...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	if len(allFolders) != 2 {
		t.Fatalf("expected 2 folders, got %d", len(allFolders))
	}
	if allFolders[0].ID != "folder1" || allFolders[0].Name != "Vacation 2024" {
		t.Errorf("unexpected first folder: %+v", allFolders[0])
	}
	if allFolders[1].ID != "folder2" || allFolders[1].Name != "Wedding" {
		t.Errorf("unexpected second folder: %+v", allFolders[1])
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls for pagination, got %d", callCount)
	}
}

func TestListImagesInFolder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if !strings.Contains(q, "'folder123' in parents") {
			t.Errorf("expected folder123 in query, got: %s", q)
		}
		if !strings.Contains(q, "mimeType contains 'image/'") {
			t.Errorf("expected image filter in query, got: %s", q)
		}

		writeJSON(w, `{
			"files": [
				{
					"id": "img1",
					"name": "DSC_0001.jpg",
					"mimeType": "image/jpeg",
					"size": "4521984",
					"modifiedTime": "2024-06-15T10:30:00Z",
					"createdTime": "2024-06-14T08:00:00Z",
					"imageMediaMetadata": {"width": 6000, "height": 4000}
				},
				{
					"id": "img2",
					"name": "DSC_0002.png",
					"mimeType": "image/png",
					"size": "8192000",
					"modifiedTime": "2024-06-15T11:00:00Z",
					"createdTime": "2024-06-14T09:00:00Z"
				}
			]
		}`)
	}))
	defer server.Close()

	client := newTestClient(server)
	resp, err := client.ListImagesInFolder(context.Background(), "folder123", "")
	if err != nil {
		t.Fatalf("ListImagesInFolder failed: %v", err)
	}

	if len(resp.Files) != 2 {
		t.Fatalf("expected 2 images, got %d", len(resp.Files))
	}

	img := resp.Files[0]
	if img.ID != "img1" || img.Name != "DSC_0001.jpg" || img.MimeType != "image/jpeg" {
		t.Errorf("unexpected image: %+v", img)
	}
	if img.Size != "4521984" {
		t.Errorf("expected size 4521984, got %s", img.Size)
	}
	if img.ImageMedia == nil || img.ImageMedia.Width != 6000 || img.ImageMedia.Height != 4000 {
		t.Errorf("unexpected image metadata: %+v", img.ImageMedia)
	}
}

func TestDownloadFile(t *testing.T) {
	fileContent := []byte("fake JPEG binary data for testing download functionality")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/files/file42") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("alt") != "media" {
			t.Errorf("expected alt=media, got: %s", r.URL.Query().Get("alt"))
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		io.Copy(w, bytes.NewReader(fileContent)) //nolint:errcheck // test mock
	}))
	defer server.Close()

	client := newTestClient(server)
	var buf bytes.Buffer
	var lastWritten, lastTotal int64

	err := client.DownloadFile(context.Background(), "file42", &buf, func(written, total int64) {
		lastWritten = written
		lastTotal = total
	})
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), fileContent) {
		t.Errorf("downloaded content mismatch: got %d bytes", buf.Len())
	}
	if lastWritten != int64(len(fileContent)) {
		t.Errorf("expected progress written=%d, got %d", len(fileContent), lastWritten)
	}
	if lastTotal != int64(len(fileContent)) {
		t.Errorf("expected progress total=%d, got %d", len(fileContent), lastTotal)
	}
}

func TestListFolders_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		writeJSON(w, `{"error": {"code": 429, "message": "Rate Limit Exceeded"}}`)
	}))
	defer server.Close()

	client := newTestClient(server)
	_, err := client.ListFolders(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for rate limit")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("expected 'rate limited' in error, got: %s", err.Error())
	}
}

func TestListFolders_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.Copy(w, strings.NewReader(`{"files": [{"id": "broken"`)) //nolint:errcheck // test mock: intentionally truncated JSON
	}))
	defer server.Close()

	client := newTestClient(server)
	_, err := client.ListFolders(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "decode response failed") {
		t.Errorf("expected 'decode response failed' in error, got: %s", err.Error())
	}
}
