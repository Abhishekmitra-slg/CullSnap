package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureUVHashMismatchRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not the real binary"))
	}))
	defer srv.Close()
	p := &Provisioner{
		cullsnapDir: t.TempDir(),
		httpClient:  http.DefaultClient,
	}
	info := UVDownloadInfo{URL: srv.URL, SHA256: "0000000000000000000000000000000000000000000000000000000000000000"}
	if _, err := p.ensureUVFromInfo(context.Background(), info, nil); err == nil {
		t.Fatal("expected hash mismatch")
	}
}

func TestEnsureUVHappy(t *testing.T) {
	payload := []byte("FAKEBIN")
	sum := sha256.Sum256(payload)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	p := &Provisioner{cullsnapDir: t.TempDir(), httpClient: http.DefaultClient}
	info := UVDownloadInfo{URL: srv.URL, SHA256: hex.EncodeToString(sum[:])}
	path, err := p.ensureUVFromInfo(context.Background(), info, nil)
	if err != nil {
		t.Fatalf("ensureUVFromInfo: %v", err)
	}
	info2, _ := os.Stat(path)
	if info2 == nil || info2.Mode()&0o111 == 0 {
		t.Fatalf("uv not executable: %v", info2)
	}
	if filepath.Base(path) != "uv" {
		t.Fatalf("path: %s", path)
	}
}
