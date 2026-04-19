package vlm

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// zipBuilder assembles an in-memory zip and writes it to a temp file for each
// test. Tests describe the zip declaratively (name + mode + bytes) so failure
// messages point at the construction rather than byte-level reader setup.
type zipEntrySpec struct {
	name string
	mode os.FileMode
	body []byte
}

func buildTestZip(t *testing.T, entries []zipEntrySpec) string {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		fh := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		fh.SetMode(mode)
		fw, err := w.CreateHeader(fh)
		if err != nil {
			t.Fatalf("build zip: create header %q: %v", e.name, err)
		}
		if _, err := fw.Write(e.body); err != nil {
			t.Fatalf("build zip: write %q: %v", e.name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("build zip: close: %v", err)
	}
	path := filepath.Join(t.TempDir(), "test.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("build zip: write file: %v", err)
	}
	return path
}

// TestExtractLlamaServerZipHappyPath exercises a minimal realistic zip layout
// and verifies that every allow-listed runtime file lands in destDir, that
// the binary gets an executable bit regardless of what the archive recorded,
// and that the "build/bin/" prefix is stripped so @loader_path / $ORIGIN
// rpath resolution works.
func TestExtractLlamaServerZipHappyPath(t *testing.T) {
	zipPath := buildTestZip(t, []zipEntrySpec{
		{name: "build/bin/llama-server", mode: 0o644, body: []byte("#!/bin/sh\necho fake\n")},
		{name: "build/bin/libllama.dylib", mode: 0o644, body: []byte("lib-llama-bytes")},
		{name: "build/bin/libggml-metal.dylib", mode: 0o644, body: []byte("metal-bytes")},
		{name: "build/bin/ggml-metal.metal", mode: 0o644, body: []byte("metal-shader")},
		{name: "build/bin/ggml-common.h", mode: 0o644, body: []byte("header")},
		{name: "build/bin/LICENSE", mode: 0o644, body: []byte("license text")},
		// These should NOT be extracted — wasted disk otherwise.
		{name: "build/bin/llama-bench", mode: 0o755, body: []byte("other bin")},
		{name: "build/bin/llama-cli", mode: 0o755, body: []byte("other bin")},
		{name: "README.md", mode: 0o644, body: []byte("ignored")},
	})

	dest := t.TempDir()
	written, err := ExtractLlamaServerZip(zipPath, dest)
	if err != nil {
		t.Fatalf("ExtractLlamaServerZip: %v", err)
	}
	// 6 allow-listed: binary + 2 libs + .metal + .h + LICENSE
	const wantWritten = 6
	if written != wantWritten {
		t.Errorf("extracted %d files, want %d", written, wantWritten)
	}

	expectedPresent := []string{"llama-server", "libllama.dylib", "libggml-metal.dylib", "ggml-metal.metal", "ggml-common.h", "LICENSE"}
	for _, name := range expectedPresent {
		if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
			t.Errorf("expected %s at dest, got stat err: %v", name, err)
		}
	}

	expectedAbsent := []string{"llama-bench", "llama-cli", "README.md"}
	for _, name := range expectedAbsent {
		if _, err := os.Stat(filepath.Join(dest, name)); err == nil {
			t.Errorf("%s was extracted but should have been filtered out", name)
		}
	}

	// Binary must be executable even though the zip entry was 0o644.
	binInfo, err := os.Stat(filepath.Join(dest, "llama-server"))
	if err != nil {
		t.Fatalf("stat extracted binary: %v", err)
	}
	if binInfo.Mode().Perm()&0o111 == 0 {
		t.Errorf("llama-server mode %v is not executable", binInfo.Mode())
	}
}

// TestExtractLlamaServerZipLinuxSharedObjects confirms the filter accepts the
// ".so" naming convention used by the ubuntu-x64 release zip.
func TestExtractLlamaServerZipLinuxSharedObjects(t *testing.T) {
	zipPath := buildTestZip(t, []zipEntrySpec{
		{name: "build/bin/llama-server", body: []byte("bin")},
		{name: "build/bin/libllama.so", body: []byte("so")},
		{name: "build/bin/libggml.so.1", body: []byte("versioned so")},
	})
	dest := t.TempDir()
	_, err := ExtractLlamaServerZip(zipPath, dest)
	if err != nil {
		t.Fatalf("ExtractLlamaServerZip: %v", err)
	}
	for _, name := range []string{"llama-server", "libllama.so", "libggml.so.1"} {
		if _, err := os.Stat(filepath.Join(dest, name)); err != nil {
			t.Errorf("expected %s at dest, got stat err: %v", name, err)
		}
	}
}

// TestExtractLlamaServerZipRejectsZipSlip asserts we refuse to write outside
// destDir when the entry name uses "..". These are the only zip-slip vectors
// that survive the "build/bin/" prefix filter — absolute paths and names
// missing the prefix are silently skipped before reaching path resolution.
func TestExtractLlamaServerZipRejectsZipSlip(t *testing.T) {
	cases := []string{
		"build/bin/../../../llama-server",
		"build/bin/../llama-server",
	}
	for _, evil := range cases {
		t.Run(evil, func(t *testing.T) {
			zipPath := buildTestZip(t, []zipEntrySpec{
				{name: "build/bin/llama-server", body: []byte("ok")},
				{name: evil, body: []byte("evil")},
			})
			_, err := ExtractLlamaServerZip(zipPath, t.TempDir())
			if err == nil || !strings.Contains(err.Error(), "zip-slip") {
				t.Fatalf("expected zip-slip error, got %v", err)
			}
		})
	}
}

// TestExtractLlamaServerZipSilentlyDropsOutOfPrefix verifies that entry
// names outside the "build/bin/" allowlist (absolute paths, top-level files,
// nested siblings) are quietly filtered rather than raising an error. The
// filter runs before path resolution, so a malicious archive that only
// contains out-of-prefix entries produces no output files and no extraction.
func TestExtractLlamaServerZipSilentlyDropsOutOfPrefix(t *testing.T) {
	zipPath := buildTestZip(t, []zipEntrySpec{
		{name: "build/bin/llama-server", body: []byte("ok")},
		{name: "build/bin/libllama.dylib", body: []byte("ok")},
		{name: "/tmp/llama-server", body: []byte("evil-abs-path")},
		{name: "other/llama-server", body: []byte("evil-sibling")},
	})
	dest := t.TempDir()
	n, err := ExtractLlamaServerZip(zipPath, dest)
	if err != nil {
		t.Fatalf("ExtractLlamaServerZip: %v", err)
	}
	if n != 2 {
		t.Errorf("extracted %d, want 2", n)
	}
	if _, err := os.Stat("/tmp/llama-server.tmp"); err == nil {
		t.Fatal("SECURITY: extractor wrote /tmp/llama-server — abs-path filter broken")
	}
}

// TestExtractLlamaServerZipMissingBinaryChmodFails documents the invariant:
// if the zip has no llama-server entry, the post-extract chmod fails because
// there is nothing to chmod. The error is explicit so callers know the
// archive was malformed rather than silently producing a broken runtime.
func TestExtractLlamaServerZipMissingBinaryChmodFails(t *testing.T) {
	zipPath := buildTestZip(t, []zipEntrySpec{
		{name: "build/bin/libllama.dylib", body: []byte("lib")},
	})
	_, err := ExtractLlamaServerZip(zipPath, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "chmod llama-server binary") {
		t.Fatalf("expected chmod-missing error, got %v", err)
	}
}

// TestIsLegacyZipAtBinaryPathDetectsZip and TestIsLegacyZipAtBinaryPathIgnoresBinary
// together cover the upgrade-recovery predicate used by provisionLlamaServer.
func TestIsLegacyZipAtBinaryPathDetectsZip(t *testing.T) {
	zipPath := buildTestZip(t, []zipEntrySpec{{name: "foo", body: []byte("bar")}})
	got, err := IsLegacyZipAtBinaryPath(zipPath)
	if err != nil {
		t.Fatalf("IsLegacyZipAtBinaryPath err = %v", err)
	}
	if !got {
		t.Error("expected IsLegacyZipAtBinaryPath to detect zip magic")
	}
}

func TestIsLegacyZipAtBinaryPathIgnoresBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fake-binary")
	// Mach-O 64 magic so this does not accidentally start with "PK\x03\x04".
	if err := os.WriteFile(path, []byte{0xCF, 0xFA, 0xED, 0xFE, 0x01, 0x02, 0x03, 0x04}, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := IsLegacyZipAtBinaryPath(path)
	if err != nil {
		t.Fatalf("IsLegacyZipAtBinaryPath err = %v", err)
	}
	if got {
		t.Error("expected IsLegacyZipAtBinaryPath to return false for non-zip file")
	}
}

func TestIsLegacyZipAtBinaryPathMissingFile(t *testing.T) {
	got, err := IsLegacyZipAtBinaryPath(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if got {
		t.Error("missing file should report false")
	}
}

// TestLlamaServerRuntimeReadyDetectsAllStates covers the three states that
// the provisioner distinguishes: no install, legacy zip-at-binary, and a
// proper extracted layout. The decision logic is load-bearing for whether
// provisionLlamaServer decides to download + extract or short-circuit.
func TestLlamaServerRuntimeReadyDetectsAllStates(t *testing.T) {
	cullsnapDir := t.TempDir()
	binDir := filepath.Join(cullsnapDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binaryPath := LlamaServerBinaryPath(cullsnapDir)

	// State 1: no install.
	if LlamaServerRuntimeReady(cullsnapDir) {
		t.Fatal("no install should not be ready")
	}

	// State 2: legacy zip-as-binary.
	zipBody := buildTestZip(t, []zipEntrySpec{{name: "anything", body: []byte("x")}})
	src, err := os.ReadFile(zipBody)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binaryPath, src, 0o755); err != nil {
		t.Fatal(err)
	}
	if LlamaServerRuntimeReady(cullsnapDir) {
		t.Error("legacy zip-as-binary state should not be ready")
	}

	// State 3: extracted binary + libllama side-by-side.
	if err := os.WriteFile(binaryPath, []byte{0xCF, 0xFA, 0xED, 0xFE}, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "libllama.dylib"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !LlamaServerRuntimeReady(cullsnapDir) {
		t.Error("binary + libllama should register as ready")
	}
}

// TestLlamaZipMemberKept pins down the allowlist so future changes to the
// filter are visible in the review diff rather than silently shrinking or
// growing what lands on users' disks.
func TestLlamaZipMemberKept(t *testing.T) {
	keep := []string{
		"llama-server", "llama-server.exe",
		"libllama.dylib", "libggml.dylib", "libggml-metal.dylib",
		"libllama.so", "libggml.so.1",
		"libllama.dll",
		"ggml-metal.metal", "ggml-common.h",
		"LICENSE", "LICENSE-curl", "NOTICE",
	}
	skip := []string{
		"llama-bench", "llama-cli", "llama-embedding",
		"README.md",
		"some-random-file",
	}
	for _, name := range keep {
		if !llamaZipMemberKept(name) {
			t.Errorf("expected %q to be kept", name)
		}
	}
	for _, name := range skip {
		if llamaZipMemberKept(name) {
			t.Errorf("expected %q to be skipped", name)
		}
	}
}

// TestLlamaServerZipPath anchors the staging location so it never collides
// with LlamaServerBinaryPath — the precise bug this PR is fixing.
func TestLlamaServerZipPath(t *testing.T) {
	const root = "/home/user/.cullsnap"
	zipPath := LlamaServerZipPath(root)
	binPath := LlamaServerBinaryPath(root)
	if zipPath == binPath {
		t.Errorf("LlamaServerZipPath (%q) must differ from LlamaServerBinaryPath (%q)", zipPath, binPath)
	}
	if filepath.Ext(zipPath) != ".zip" {
		t.Errorf("expected .zip extension on zip path, got %q", zipPath)
	}
}
