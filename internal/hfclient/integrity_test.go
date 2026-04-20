package hfclient

import (
	"bytes"
	"testing"
)

func TestGitBlobSHA1(t *testing.T) {
	// Reference: `git hash-object` of a file containing "hello\n"
	// git hash-object <(printf "hello\n") -> ce013625030ba8dba906f756967f9e9ca394464a
	in := []byte("hello\n")
	got, err := GitBlobSHA1(bytes.NewReader(in), int64(len(in)))
	if err != nil {
		t.Fatalf("GitBlobSHA1: %v", err)
	}
	want := "ce013625030ba8dba906f756967f9e9ca394464a"
	if got != want {
		t.Fatalf("GitBlobSHA1: got %s want %s", got, want)
	}
}

func TestFileHasherSHA256(t *testing.T) {
	h := newFileHasher()
	if _, err := h.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	got := h.SumHex()
	want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Fatalf("SumHex: got %s want %s", got, want)
	}
}
