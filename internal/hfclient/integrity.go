package hfclient

import (
	"crypto/sha1" //nolint:gosec // git uses SHA-1 by spec; not for security
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
)

// GitBlobSHA1 computes the git blob OID for the given content, equivalent to:
//
//	sha1("blob " + size + "\0" + content)
//
// Used to verify non-LFS files (HF tree returns the git OID).
func GitBlobSHA1(r io.Reader, size int64) (string, error) {
	h := sha1.New() //nolint:gosec // nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-sha1
	if _, err := fmt.Fprintf(h, "blob %d\x00", size); err != nil {
		return "", err
	}
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fileHasher accumulates SHA-256 incrementally (use as io.Writer in MultiWriter).
type fileHasher struct {
	h hash.Hash
}

func newFileHasher() *fileHasher { return &fileHasher{h: sha256.New()} }

func (f *fileHasher) Write(p []byte) (int, error) { return f.h.Write(p) }

func (f *fileHasher) SumHex() string { return hex.EncodeToString(f.h.Sum(nil)) }
