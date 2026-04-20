package hfclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchTreeHappy(t *testing.T) {
	body := `[
      {"type":"file","path":"config.json","oid":"deadbeef","size":42},
      {"type":"file","path":"model.safetensors","oid":"f00","size":1024,
       "lfs":{"oid":"abc123","size":1024,"pointerSize":135}}
    ]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/models/foo/bar/tree/main" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("X-Repo-Commit", "1111111111111111111111111111111111111111")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL
	entries, commit, err := c.FetchTree(context.Background(), "foo/bar", "main")
	if err != nil {
		t.Fatalf("FetchTree: %v", err)
	}
	if commit != "1111111111111111111111111111111111111111" {
		t.Fatalf("commit: %s", commit)
	}
	if len(entries) != 2 {
		t.Fatalf("entries: %d", len(entries))
	}
	if entries[0].Path != "config.json" || entries[0].SHA1 != "deadbeef" || entries[0].IsLFS {
		t.Fatalf("entries[0]: %+v", entries[0])
	}
	if entries[1].SHA256 != "abc123" || !entries[1].IsLFS {
		t.Fatalf("entries[1]: %+v", entries[1])
	}
}

func TestFetchTreeRejectsBadPath(t *testing.T) {
	body := `[{"type":"file","path":"../etc/passwd","oid":"x","size":1}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Repo-Commit", "1111111111111111111111111111111111111111")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := New("")
	c.baseURL = srv.URL
	if _, _, err := c.FetchTree(context.Background(), "foo/bar", "main"); err == nil {
		t.Fatal("expected error on bad path")
	}
}

func TestFetchTreeFallbackToRevisionSHA(t *testing.T) {
	// HF tree endpoint dropped X-Repo-Commit around 2026-Q1; client must fall back
	// to /api/models/<repo>/revision/<rev> and parse the `sha` field.
	wantSHA := "abcdef0123456789abcdef0123456789abcdef01"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/models/foo/bar/tree/main":
			// No X-Repo-Commit header.
			_, _ = w.Write([]byte(`[]`)) // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
		case "/api/models/foo/bar/revision/main":
			_, _ = w.Write([]byte(`{"sha":"` + wantSHA + `"}`)) // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := New("")
	c.baseURL = srv.URL
	_, commit, err := c.FetchTree(context.Background(), "foo/bar", "main")
	if err != nil {
		t.Fatalf("FetchTree: %v", err)
	}
	if commit != wantSHA {
		t.Fatalf("commit: got %q want %q", commit, wantSHA)
	}
}

func TestFetchTreeAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer testtok" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("X-Repo-Commit", "1111111111111111111111111111111111111111")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	c := New("testtok")
	c.baseURL = srv.URL
	if _, _, err := c.FetchTree(context.Background(), "foo/bar", "main"); err != nil {
		t.Fatalf("FetchTree with auth: %v", err)
	}
}
