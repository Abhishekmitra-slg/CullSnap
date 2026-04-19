package vlm

import (
	"archive/zip"
	"cullsnap/internal/logger"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// llamaZipEntryPrefix is the subdirectory inside llama.cpp release zips where
// the runtime artifacts live. Everything outside this prefix is ignored.
const llamaZipEntryPrefix = "build/bin/"

// zipMagic is the standard local-file-header signature at the start of every
// ZIP archive. Used to detect legacy installations where a previous buggy
// provisioner wrote the zip straight to the binary path.
var zipMagic = []byte{0x50, 0x4B, 0x03, 0x04}

// LlamaServerZipPath returns the local staging location for the downloaded
// llama.cpp release zip. It deliberately lives next to the extracted binary
// so we can remove it with a single known path after extraction succeeds,
// while never colliding with LlamaServerBinaryPath.
func LlamaServerZipPath(cullsnapDir string) string {
	return filepath.Join(cullsnapDir, binSubdir, "llama-server.zip")
}

// LlamaServerRuntimeReady reports whether an extracted llama-server runtime
// appears usable: the binary exists and at least one of the core shared
// libraries (libllama.dylib or libllama.so) is co-located with it so the
// @loader_path / $ORIGIN rpath resolves at exec time.
//
// This is a cheap readiness check — it does not actually run the binary. It
// replaces the previous "binary exists" shortcut, which returned true for the
// legacy state where the zip itself sat at the binary path.
func LlamaServerRuntimeReady(cullsnapDir string) bool {
	binaryPath := LlamaServerBinaryPath(cullsnapDir)
	info, err := os.Stat(binaryPath)
	if err != nil || info.IsDir() {
		return false
	}
	isZip, _ := isZipFile(binaryPath)
	if isZip {
		if logger.Log != nil {
			logger.Log.Debug("vlm: legacy zip-as-binary detected at llama-server path",
				"path", binaryPath)
		}
		return false
	}
	dir := filepath.Dir(binaryPath)
	for _, lib := range []string{"libllama.dylib", "libllama.so"} {
		if _, err := os.Stat(filepath.Join(dir, lib)); err == nil {
			return true
		}
	}
	return false
}

// ExtractLlamaServerZip extracts the runtime artifacts from a llama.cpp
// release zip into destDir. It:
//   - Strips the "build/bin/" prefix so the extracted files sit directly in
//     destDir (required for macOS @loader_path / Linux $ORIGIN rpath lookups).
//   - Refuses entries that resolve outside destDir after cleaning (zip-slip).
//   - Filters members through llamaZipMemberKept so only runtime-necessary
//     files land on disk (binary, shared libraries, Metal shader, license
//     files). Other bundled binaries (llama-bench, llama-cli, ...) are skipped.
//   - Chmods the main binary to 0o755 regardless of the mode recorded in the
//     zip, since zip archives on Windows-hosted CI sometimes drop the exec bit.
//
// Returns the count of files written. destDir is created if missing.
func ExtractLlamaServerZip(zipPath, destDir string) (int, error) {
	if logger.Log != nil {
		logger.Log.Debug("vlm: extracting llama-server zip",
			"zip", zipPath, "dest", destDir)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return 0, fmt.Errorf("vlm: mkdir extract dest: %w", err)
	}

	// filepath.Clean once so every zip-slip check compares against a canonical
	// form (trailing separators stripped, ".." resolved within destDir).
	cleanDest, err := filepath.Abs(filepath.Clean(destDir))
	if err != nil {
		return 0, fmt.Errorf("vlm: resolve extract dest: %w", err)
	}

	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, fmt.Errorf("vlm: open zip: %w", err)
	}
	defer r.Close() //nolint:errcheck // read-only zip close

	binaryName := "llama-server"
	var written int
	for _, entry := range r.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		rel, ok := strings.CutPrefix(entry.Name, llamaZipEntryPrefix)
		if !ok || rel == "" {
			continue
		}
		if !llamaZipMemberKept(rel) {
			continue
		}

		outPath, slipErr := safeJoin(cleanDest, rel)
		if slipErr != nil {
			return written, slipErr
		}

		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return written, fmt.Errorf("vlm: mkdir for %q: %w", rel, err)
		}

		if err := writeZipEntry(entry, outPath); err != nil {
			return written, err
		}
		written++
	}

	binaryPath := filepath.Join(cleanDest, binaryName)
	if err := os.Chmod(binaryPath, 0o755); err != nil {
		return written, fmt.Errorf("vlm: chmod llama-server binary: %w", err)
	}

	if logger.Log != nil {
		logger.Log.Debug("vlm: llama-server zip extraction complete",
			"files_written", written, "dest", cleanDest)
	}
	return written, nil
}

// llamaZipMemberKept reports whether a zip entry (with llamaZipEntryPrefix
// already stripped) should be written to disk. The allowlist is deliberately
// conservative: the VLM backend only needs the llama-server process and its
// runtime dependencies, so we skip the dozen other CLI binaries in the
// release to save disk space and reduce install surface area.
func llamaZipMemberKept(rel string) bool {
	base := filepath.Base(rel)
	if base == "llama-server" || base == "llama-server.exe" {
		return true
	}
	// Shared libraries — macOS .dylib, Linux .so(+version), Windows .dll.
	if strings.HasPrefix(base, "lib") &&
		(strings.HasSuffix(base, ".dylib") || containsDotSo(base) || strings.HasSuffix(base, ".dll")) {
		return true
	}
	// Metal compute shader + its headers (macOS arm64 GPU backend).
	if strings.HasSuffix(base, ".metal") || strings.HasSuffix(base, ".h") {
		return true
	}
	// License / notice files for compliance when redistributing binaries.
	if strings.HasPrefix(base, "LICENSE") || strings.HasPrefix(base, "NOTICE") {
		return true
	}
	return false
}

// containsDotSo matches both plain "libfoo.so" and versioned "libfoo.so.1.2"
// names that show up in some Linux llama.cpp release packages.
func containsDotSo(name string) bool {
	return strings.HasSuffix(name, ".so") || strings.Contains(name, ".so.")
}

// safeJoin returns filepath.Join(base, rel) after rejecting any rel that
// escapes base via absolute paths, "..", or symlinks encoded in the zip.
// Returns a typed error so callers can identify the attack signature in logs.
func safeJoin(base, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("vlm: zip entry %q uses absolute path (zip-slip)", rel)
	}
	joined := filepath.Join(base, rel)
	cleaned, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("vlm: resolve zip entry path: %w", err)
	}
	// Trailing separator required so "/foo" does not match "/foo-bar".
	baseWithSep := base
	if !strings.HasSuffix(baseWithSep, string(os.PathSeparator)) {
		baseWithSep += string(os.PathSeparator)
	}
	if cleaned != base && !strings.HasPrefix(cleaned, baseWithSep) {
		return "", fmt.Errorf("vlm: zip entry %q escapes destination (zip-slip)", rel)
	}
	return cleaned, nil
}

// writeZipEntry copies a single zip.File to outPath using atomic
// create-write-rename semantics: a .tmp sibling is written first, then
// renamed over outPath so a crash mid-write never leaves a half-extracted
// binary that later "looks ready".
func writeZipEntry(entry *zip.File, outPath string) error {
	tmpPath := outPath + ".tmp"
	rc, err := entry.Open()
	if err != nil {
		return fmt.Errorf("vlm: open zip entry %q: %w", entry.Name, err)
	}
	defer rc.Close() //nolint:errcheck // read-only zip entry close

	mode := entry.Mode()
	if mode == 0 {
		mode = 0o644
	}

	// Use a named function so the copy+sync+close+rename sequence has a
	// single recovery path (remove the .tmp) that handles every error case
	// without the double-close pitfall of deferring Close while also
	// calling it explicitly before Rename.
	err = writeAtomic(tmpPath, outPath, mode.Perm(), rc, entry.Name)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// writeAtomic copies src into tmpPath, syncs+closes, then renames to outPath.
// Any failure after OpenFile means the caller should unlink tmpPath.
func writeAtomic(tmpPath, outPath string, perm os.FileMode, src io.Reader, entryName string) error {
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("vlm: open extract target %q: %w", outPath, err)
	}
	if _, err := io.Copy(f, src); err != nil { //nolint:gosec // zip members already filtered by llamaZipMemberKept
		_ = f.Close()
		return fmt.Errorf("vlm: copy zip entry %q: %w", entryName, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("vlm: sync extract target %q: %w", outPath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("vlm: close extract target %q: %w", outPath, err)
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		return fmt.Errorf("vlm: rename extract target %q: %w", outPath, err)
	}
	return nil
}

// IsLegacyZipAtBinaryPath reports whether the file at binaryPath is actually
// a ZIP archive. Earlier versions of provisionLlamaServer downloaded the
// llama.cpp release zip directly to the binary path without extracting,
// leaving a non-executable file that failed silently at exec time. This
// predicate lets the upgrade path detect that state and recover. Returns
// false (without error) when the path does not exist.
func IsLegacyZipAtBinaryPath(binaryPath string) (bool, error) {
	return isZipFile(binaryPath)
}

// isZipFile returns true when path begins with the ZIP local-file-header
// signature. Used to detect legacy installs where the zip was saved to the
// binary path.
func isZipFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer f.Close() //nolint:errcheck // read-only check

	buf := make([]byte, len(zipMagic))
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return false, err
	}
	if n < len(zipMagic) {
		return false, nil
	}
	for i, b := range zipMagic {
		if buf[i] != b {
			return false, nil
		}
	}
	return true, nil
}
