package hfclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDownloadFileSimple(t *testing.T) {
	payload := []byte("hello world")
	sum := sha256.Sum256(payload)
	sumHex := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("X-Linked-Etag", sumHex)
			w.Header().Set("Content-Length", "11")
		case http.MethodGet:
			_, _ = w.Write(payload) // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "f.bin")
	expect := FileEntry{Path: "f.bin", Size: 11, SHA256: sumHex, IsLFS: true}

	n, err := downloadOneFile(context.Background(), srv.URL, dest, expect, nil)
	if err != nil {
		t.Fatalf("downloadOneFile: %v", err)
	}
	if n != 11 {
		t.Fatalf("bytes: %d", n)
	}
	got, _ := os.ReadFile(dest)
	if !strings.EqualFold(string(got), "hello world") {
		t.Fatalf("got %q", string(got))
	}
}

func TestDownloadFileResume(t *testing.T) {
	payload := []byte("0123456789ABCDEF")
	sum := sha256.Sum256(payload)
	sumHex := hex.EncodeToString(sum[:])

	var requestRanges []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("X-Linked-Etag", sumHex)
			w.Header().Set("Content-Length", "16")
			return
		}
		rng := r.Header.Get("Range")
		requestRanges = append(requestRanges, rng)
		if rng != "" {
			w.Header().Set("Content-Range", "bytes 8-15/16")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload[8:]) // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
			return
		}
		_, _ = w.Write(payload) // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "f.bin")
	if err := os.WriteFile(dest+".incomplete", payload[:8], 0o644); err != nil {
		t.Fatal(err)
	}
	expect := FileEntry{Path: "f.bin", Size: 16, SHA256: sumHex, IsLFS: true}
	if _, err := downloadOneFile(context.Background(), srv.URL, dest, expect, nil); err != nil {
		t.Fatalf("downloadOneFile: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q want %q", got, payload)
	}
	if len(requestRanges) != 1 || requestRanges[0] != "bytes=8-" {
		t.Fatalf("ranges: %v", requestRanges)
	}
}

func TestDownloadFileShaMismatchHardFails(t *testing.T) {
	payload := []byte("good")
	bogusSum := strings.Repeat("0", 64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("X-Linked-Etag", bogusSum)
			w.Header().Set("Content-Length", "4")
			return
		}
		_, _ = w.Write(payload) // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
	}))
	defer srv.Close()
	dir := t.TempDir()
	expect := FileEntry{Path: "x", Size: 4, SHA256: bogusSum, IsLFS: true}
	if _, err := downloadOneFile(context.Background(), srv.URL, filepath.Join(dir, "x"), expect, nil); err == nil {
		t.Fatal("expected SHA mismatch error")
	}
}

// keep io package referenced for future tests
var _ = io.Discard

func TestDownloadSnapshotHappy(t *testing.T) {
	files := map[string][]byte{
		"config.json":       []byte(`{"k":1}`),
		"model.safetensors": []byte("MODEL"),
	}
	sum := func(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
	expected := []FileEntry{
		{
			Path: "config.json", Size: int64(len(files["config.json"])),
			SHA1: mustGitBlobSha1(files["config.json"]),
		},
		{
			Path: "model.safetensors", Size: int64(len(files["model.safetensors"])),
			SHA256: sum(files["model.safetensors"]), IsLFS: true,
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/models/") && strings.Contains(r.URL.Path, "/tree/") {
			entries := `[
				{"type":"file","path":"config.json","oid":"` + mustGitBlobSha1(files["config.json"]) + `","size":` + itoa(len(files["config.json"])) + `},
				{"type":"file","path":"model.safetensors","oid":"x","size":` + itoa(len(files["model.safetensors"])) + `,
				 "lfs":{"oid":"` + sum(files["model.safetensors"]) + `","size":` + itoa(len(files["model.safetensors"])) + `,"pointerSize":1}}
			]`
			w.Header().Set("X-Repo-Commit", "1111111111111111111111111111111111111111")
			_, _ = w.Write([]byte(entries)) // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
			return
		}
		for name, content := range files {
			if strings.HasSuffix(r.URL.Path, "/"+name) {
				if r.Method == http.MethodHead {
					w.Header().Set("Content-Length", itoa(len(content)))
					if name == "model.safetensors" {
						w.Header().Set("X-Linked-Etag", sum(content))
					}
					return
				}
				_, _ = w.Write(content) // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
				return
			}
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	c := New("")
	c.baseURL = srv.URL
	dir := t.TempDir()
	dest := filepath.Join(dir, "snap")

	res, err := c.DownloadSnapshot(context.Background(),
		"foo/bar", "1111111111111111111111111111111111111111",
		[]string{"*.json", "*.safetensors"}, expected, dest, nil)
	if err != nil {
		t.Fatalf("DownloadSnapshot: %v", err)
	}
	if res.CommitSHA != "1111111111111111111111111111111111111111" {
		t.Fatal("commit")
	}
	if _, err := os.Stat(filepath.Join(dest, "config.json")); err != nil {
		t.Fatal("config.json missing")
	}
	if _, err := os.Stat(filepath.Join(dest, ".cullsnap-snapshot.json")); err != nil {
		t.Fatal("snapshot manifest missing")
	}
}

func TestDownloadSnapshotDriftRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Repo-Commit", "1111111111111111111111111111111111111111")
		_, _ = w.Write([]byte(`[]`)) // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter
	}))
	defer srv.Close()
	c := New("")
	c.baseURL = srv.URL
	expected := []FileEntry{{Path: "config.json", Size: 1, SHA1: "deadbeef"}}
	if _, err := c.DownloadSnapshot(context.Background(), "foo/bar",
		"1111111111111111111111111111111111111111",
		nil, expected, t.TempDir(), nil); err == nil {
		t.Fatal("expected drift error")
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

func mustGitBlobSha1(b []byte) string {
	s, err := GitBlobSHA1(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		panic(err)
	}
	return s
}
