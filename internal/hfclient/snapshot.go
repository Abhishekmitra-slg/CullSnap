package hfclient

import (
	"context"
	"cullsnap/internal/logger"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

const snapshotMetaFile = ".cullsnap-snapshot.json"

type snapshotMeta struct {
	Schema       int                 `json:"schema"`
	Repo         string              `json:"repo"`
	CommitSHA    string              `json:"commit_sha"`
	DownloadedAt string              `json:"downloaded_at"`
	Files        []snapshotMetaEntry `json:"files"`
}

type snapshotMetaEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
	SHA1   string `json:"sha1,omitempty"`
}

// DownloadSnapshot fetches every file in expected into destDir, atomically.
func (c *Client) DownloadSnapshot(
	ctx context.Context,
	repo, commitSHA string,
	allowPatterns []string,
	expected []FileEntry,
	destDir string,
	progress SnapshotProgress,
) (*SnapshotResult, error) {
	start := time.Now()
	tree, gotCommit, err := c.FetchTree(ctx, repo, commitSHA)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(gotCommit, commitSHA) {
		return nil, fmt.Errorf("hfclient: snapshot drift: requested commit %s, server returned %s", commitSHA, gotCommit)
	}
	treeByPath := make(map[string]TreeEntry, len(tree))
	for _, e := range tree {
		treeByPath[e.Path] = e
	}
	if progress != nil {
		progress(SnapshotEvent{Kind: "tree-fetched", FilesTotal: len(expected)})
	}

	if err := assertManifestMatchesTree(expected, treeByPath); err != nil {
		return nil, err
	}

	if len(allowPatterns) > 0 {
		for _, e := range expected {
			if !matchesAny(e.Path, allowPatterns) {
				return nil, fmt.Errorf("hfclient: expected file %q does not match allowPatterns", e.Path)
			}
		}
	}

	tmpDir := destDir + ".incomplete"
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return nil, fmt.Errorf("hfclient: mkdir tmp: %w", err)
	}

	var aggTotal int64
	for _, e := range expected {
		aggTotal += e.Size
	}

	var (
		aggDoneMu   sync.Mutex
		aggDone     int64
		filesDone   int
		filesByPath = make(map[string]int64, len(expected))
	)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(8)
	for _, e := range expected {
		e := e
		g.Go(func() error {
			target := filepath.Join(tmpDir, e.Path)
			return withFileLock(gctx, target, func() error {
				if progress != nil {
					progress(SnapshotEvent{
						Kind: "file-start", File: e.Path, BytesTotal: e.Size,
						FilesTotal: len(expected),
					})
				}
				url := c.ResolveURL(repo, commitSHA, e.Path)
				n, err := downloadOneFile(gctx, url, target, e, func(delta int) {
					aggDoneMu.Lock()
					aggDone += int64(delta)
					snap := aggDone
					aggDoneMu.Unlock()
					if progress != nil {
						progress(SnapshotEvent{
							Kind: "file-bytes", File: e.Path,
							BytesTotal: e.Size, AggregateDone: snap, AggregateTotal: aggTotal,
							FilesTotal: len(expected),
						})
					}
				})
				if err != nil {
					return err
				}
				aggDoneMu.Lock()
				filesDone++
				filesByPath[e.Path] = n
				fd := filesDone
				ad := aggDone
				aggDoneMu.Unlock()
				if progress != nil {
					progress(SnapshotEvent{
						Kind: "file-done", File: e.Path,
						BytesDone: n, BytesTotal: e.Size,
						FilesDone: fd, FilesTotal: len(expected),
						AggregateDone: ad, AggregateTotal: aggTotal,
					})
				}
				return nil
			})
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	meta := snapshotMeta{
		Schema:       1,
		Repo:         repo,
		CommitSHA:    commitSHA,
		DownloadedAt: time.Now().UTC().Format(time.RFC3339),
	}
	for _, e := range expected {
		meta.Files = append(meta.Files, snapshotMetaEntry{
			Path: e.Path, Size: e.Size, SHA256: e.SHA256, SHA1: e.SHA1,
		})
	}
	metaBytes, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, snapshotMetaFile), metaBytes, 0o600); err != nil {
		return nil, fmt.Errorf("hfclient: write snapshot meta: %w", err)
	}

	if err := os.Rename(tmpDir, destDir); err != nil {
		if _, statErr := os.Stat(destDir); statErr == nil {
			_ = os.RemoveAll(tmpDir)
		} else {
			return nil, fmt.Errorf("hfclient: rename snapshot: %w", err)
		}
	}
	if progress != nil {
		progress(SnapshotEvent{
			Kind: "snapshot-done", FilesDone: len(expected),
			FilesTotal: len(expected), AggregateDone: aggTotal, AggregateTotal: aggTotal,
		})
	}
	if logger.Log != nil {
		logger.Log.Debug("hfclient: snapshot done", "repo", repo, "commit", commitSHA, "files", len(expected), "bytes", aggTotal)
	}
	return &SnapshotResult{
		Dir: destDir, CommitSHA: commitSHA,
		FilesByPath: filesByPath, Duration: time.Since(start),
	}, nil
}

func assertManifestMatchesTree(expected []FileEntry, tree map[string]TreeEntry) error {
	for _, e := range expected {
		t, ok := tree[e.Path]
		if !ok {
			return fmt.Errorf("hfclient: snapshot drift: manifest expects %q, tree missing", e.Path)
		}
		if t.Size != e.Size {
			return fmt.Errorf("hfclient: snapshot drift: %q size %d in manifest, %d in tree", e.Path, e.Size, t.Size)
		}
		if e.IsLFS && !strings.EqualFold(t.SHA256, e.SHA256) {
			return fmt.Errorf("hfclient: snapshot drift: %q SHA-256 mismatch", e.Path)
		}
		if !e.IsLFS && !strings.EqualFold(t.SHA1, e.SHA1) {
			return fmt.Errorf("hfclient: snapshot drift: %q SHA-1 mismatch", e.Path)
		}
	}
	return nil
}

func matchesAny(p string, patterns []string) bool {
	for _, pat := range patterns {
		if ok, _ := path.Match(pat, p); ok {
			return true
		}
		if ok, _ := path.Match(pat, filepath.Base(p)); ok {
			return true
		}
	}
	return false
}

const downloadChunkSize = 256 * 1024

// progressWriter is an io.Writer that calls fn after each successful Write.
type progressWriter struct {
	w  io.Writer
	fn func(int)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	if n > 0 && p.fn != nil {
		p.fn(n)
	}
	return n, err
}

// downloadOneFile streams the resource at url into destPath via destPath+".incomplete",
// resuming if a partial exists. Verifies SHA-256 (LFS) or git SHA-1 (non-LFS) against expect.
// Returns total bytes written. progressFn is called with bytes-since-last-call after each chunk.
func downloadOneFile(
	ctx context.Context,
	url, destPath string,
	expect FileEntry,
	progressFn func(int),
) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return 0, fmt.Errorf("hfclient: mkdir: %w", err)
	}

	if err := headVerify(ctx, url, expect); err != nil {
		return 0, err
	}

	partial := destPath + ".incomplete"
	var resumeOffset int64
	if info, err := os.Stat(partial); err == nil {
		resumeOffset = info.Size()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("hfclient: build GET: %w", err)
	}
	if resumeOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
	}
	cli := &http.Client{Timeout: 0}
	resp, err := cli.Do(req)
	if err != nil {
		return 0, fmt.Errorf("hfclient: GET %q: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusOK:
		if resumeOffset > 0 {
			_ = os.Remove(partial)
			resumeOffset = 0
		}
	case http.StatusPartialContent:
	case http.StatusRequestedRangeNotSatisfiable:
		_ = os.Remove(partial)
		return 0, fmt.Errorf("hfclient: 416 — discarded partial, retry needed")
	case http.StatusUnauthorized:
		return 0, fmt.Errorf("hfclient: %s: 401 unauthorized", url)
	case http.StatusForbidden:
		return 0, fmt.Errorf("hfclient: %s: 403 forbidden (license gate)", url)
	case http.StatusNotFound:
		return 0, fmt.Errorf("hfclient: %s: 404 not found (manifest drift)", url)
	default:
		return 0, fmt.Errorf("hfclient: %s: status %d", url, resp.StatusCode)
	}

	flag := os.O_CREATE | os.O_WRONLY | os.O_EXCL
	if resumeOffset > 0 {
		flag = os.O_WRONLY | os.O_APPEND
	}
	f, err := os.OpenFile(partial, flag, 0o644)
	if err != nil {
		return 0, fmt.Errorf("hfclient: open partial: %w", err)
	}

	hasher := newFileHasher()
	if resumeOffset > 0 {
		existing, openErr := os.Open(partial)
		if openErr != nil {
			_ = f.Close()
			return 0, fmt.Errorf("hfclient: re-hash open: %w", openErr)
		}
		if _, copyErr := io.Copy(hasher, existing); copyErr != nil {
			_ = existing.Close()
			_ = f.Close()
			return 0, fmt.Errorf("hfclient: re-hash copy: %w", copyErr)
		}
		_ = existing.Close()
	}

	sink := io.MultiWriter(f, hasher)
	pw := &progressWriter{w: sink, fn: progressFn}
	written, copyErr := io.CopyBuffer(pw, resp.Body, make([]byte, downloadChunkSize))
	if copyErr != nil {
		_ = f.Close()
		return 0, fmt.Errorf("hfclient: stream: %w", copyErr)
	}
	if err := f.Close(); err != nil {
		return 0, fmt.Errorf("hfclient: close partial: %w", err)
	}

	total := resumeOffset + written

	if expect.IsLFS {
		got := hasher.SumHex()
		if !strings.EqualFold(got, expect.SHA256) {
			_ = os.Remove(partial)
			return 0, fmt.Errorf("hfclient: SHA-256 mismatch for %q: got %s want %s", destPath, got, expect.SHA256)
		}
	} else {
		rf, err := os.Open(partial)
		if err != nil {
			return 0, fmt.Errorf("hfclient: reopen for git-sha1: %w", err)
		}
		got, hashErr := GitBlobSHA1(rf, total)
		_ = rf.Close()
		if hashErr != nil {
			return 0, fmt.Errorf("hfclient: git-sha1: %w", hashErr)
		}
		if !strings.EqualFold(got, expect.SHA1) {
			_ = os.Remove(partial)
			return 0, fmt.Errorf("hfclient: git-SHA1 mismatch for %q: got %s want %s", destPath, got, expect.SHA1)
		}
	}
	if err := os.Rename(partial, destPath); err != nil {
		return 0, fmt.Errorf("hfclient: rename partial: %w", err)
	}
	if logger.Log != nil {
		logger.Log.Debug("hfclient: file downloaded", "path", destPath, "bytes", total)
	}
	return total, nil
}

// headVerify checks that x-linked-etag (LFS) matches expect.SHA256, or that
// Content-Length (non-LFS) matches expect.Size. Defense in depth against tree/resolve mismatch.
func headVerify(ctx context.Context, url string, expect FileEntry) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return err
	}
	cli := &http.Client{Timeout: 30 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("hfclient: HEAD %q: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hfclient: HEAD %q: status %d", url, resp.StatusCode)
	}
	if expect.IsLFS {
		got := resp.Header.Get("X-Linked-Etag")
		if !strings.EqualFold(got, expect.SHA256) {
			return fmt.Errorf("hfclient: HEAD %q x-linked-etag mismatch: got %s want %s", url, got, expect.SHA256)
		}
	} else if cl := resp.ContentLength; cl > 0 && cl != expect.Size {
		return fmt.Errorf("hfclient: HEAD %q size mismatch: got %d want %d", url, cl, expect.Size)
	}
	return nil
}
