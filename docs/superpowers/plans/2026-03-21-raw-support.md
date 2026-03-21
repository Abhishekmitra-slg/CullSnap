# RAW Image Format Support — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add full RAW image support (CR2, CR3, ARW, NEF, DNG, RAF, RW2, ORF, NRW, PEF, SRW) to CullSnap with dual-path preview extraction, RAW+JPEG pairing, UI badges, and comprehensive debug logging.

**Architecture:** Two-path preview extraction — Pure Go TIFF IFD parser + imagemeta for Path A (CR2/CR3/ARW/NEF/DNG), dcraw binary fallback for Path B (RAF/RW2/ORF/NRW/PEF/SRW and edge cases). `ExtractPreview()` returns raw JPEG bytes for efficiency. RAW+JPEG companion pairing by base filename. Full-res preview cache at `~/.cullsnap/previews/`.

**Tech Stack:** Go 1.25, github.com/evanoberholster/imagemeta (CR3/format detection), dcraw binary (fallback), React 19 + TypeScript (frontend badges)

**Spec:** `docs/superpowers/specs/2026-03-20-raw-support-design.md`

**Logger pattern:** `logger.Log.Info("message", "key", value)` — import `cullsnap/internal/logger`

**Important:** The logger is initialized at `slog.LevelInfo` (see `internal/logger/logger.go:33`). All `Debug`-level logs will be silently dropped. Task 1 includes making the log level configurable via `CULLSNAP_LOG_LEVEL` environment variable so debug logging is available when needed.

---

### Task 1: Shared Extension Registry + Model Changes

**Files:**
- Create: `internal/raw/extensions.go`
- Create: `internal/raw/extensions_test.go`
- Modify: `internal/model/photo.go:6-19`

- [ ] **Step 1: Create `internal/raw/extensions.go`**

```go
package raw

import "strings"

// Extensions is the single source of truth for all supported RAW file extensions.
// All consumers (scanner, dedup, media server, thumbnail cache) must use this map.
var Extensions = map[string]bool{
	".cr2": true, ".cr3": true, ".arw": true, ".nef": true, ".dng": true,
	".raf": true, ".rw2": true, ".orf": true, ".nrw": true, ".pef": true, ".srw": true,
}

// IsRAWExt returns true if the given extension (with leading dot) is a supported RAW format.
func IsRAWExt(ext string) bool {
	return Extensions[strings.ToLower(ext)]
}

// FormatName returns the uppercase format name for display (e.g., ".cr3" → "CR3").
func FormatName(ext string) string {
	return strings.ToUpper(strings.TrimPrefix(strings.ToLower(ext), "."))
}

// ImageExtensions returns all image extensions (JPEG + PNG + RAW) for use in file filters.
func ImageExtensions() map[string]bool {
	exts := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true,
	}
	for ext := range Extensions {
		exts[ext] = true
	}
	return exts
}
```

- [ ] **Step 2: Write tests for extensions**

```go
// internal/raw/extensions_test.go
package raw

import "testing"

func TestIsRAWExt(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		{".cr2", true}, {".CR2", true}, {".Cr2", true},
		{".cr3", true}, {".arw", true}, {".nef", true}, {".dng", true},
		{".raf", true}, {".rw2", true}, {".orf", true},
		{".nrw", true}, {".pef", true}, {".srw", true},
		{".jpg", false}, {".png", false}, {".mp4", false}, {"", false},
	}
	for _, tt := range tests {
		if got := IsRAWExt(tt.ext); got != tt.want {
			t.Errorf("IsRAWExt(%q) = %v, want %v", tt.ext, got, tt.want)
		}
	}
}

func TestFormatName(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".cr3", "CR3"}, {".CR3", "CR3"}, {".arw", "ARW"}, {".nef", "NEF"},
	}
	for _, tt := range tests {
		if got := FormatName(tt.ext); got != tt.want {
			t.Errorf("FormatName(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func TestImageExtensions(t *testing.T) {
	exts := ImageExtensions()
	if !exts[".jpg"] || !exts[".cr3"] || !exts[".raf"] {
		t.Error("ImageExtensions missing expected extensions")
	}
	if exts[".mp4"] {
		t.Error("ImageExtensions should not include video")
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/abhishekmitra/Local_Documents/Local_Developments/photo_sorter/CullSnap && go test ./internal/raw/ -v -run 'TestIsRAWExt|TestFormatName|TestImageExtensions'`
Expected: PASS

- [ ] **Step 4: Add RAW fields to Photo model**

Modify `internal/model/photo.go` — add after the video fields (after line 17):

```go
	// RAW Support
	IsRAW          bool   `json:"isRAW"`          // True if file is a RAW format
	RAWFormat      string `json:"rawFormat"`       // "CR3", "ARW", "NEF", etc.
	CompanionPath  string `json:"companionPath"`   // Path to RAW+JPEG pair companion
	IsRAWCompanion bool   `json:"isRAWCompanion"`  // True if this JPEG has a RAW companion
```

- [ ] **Step 5: Make logger level configurable**

Modify `internal/logger/logger.go:33` to respect `CULLSNAP_LOG_LEVEL` environment variable:

```go
func logLevel() slog.Level {
	switch strings.ToLower(os.Getenv("CULLSNAP_LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
```

Then in `Init()`, change line 33 from:
```go
handler := slog.NewJSONHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo})
```
to:
```go
handler := slog.NewJSONHandler(file, &slog.HandlerOptions{Level: logLevel()})
```

This ensures all Debug-level logging added in this feature can be activated by setting `CULLSNAP_LOG_LEVEL=debug`.

- [ ] **Step 6: Run full test suite to verify no regressions**

Run: `go test ./internal/... -count=1`
Expected: All existing tests PASS

- [ ] **Step 7: Commit**

```bash
git add internal/raw/extensions.go internal/raw/extensions_test.go internal/model/photo.go internal/logger/logger.go
git commit -m "feat(raw): add shared extension registry, model fields, and configurable log level"
```

---

### Task 2: TIFF IFD Parser

**Files:**
- Create: `internal/raw/tiff.go`
- Create: `internal/raw/tiff_test.go`

This is the core algorithm for extracting embedded JPEG previews from CR2, NEF, ARW, and DNG files.

- [ ] **Step 1: Create `internal/raw/tiff.go` with the IFD parser**

The parser must:
- Read 8-byte TIFF header (byte order, magic 42 vs 43, IFD0 offset)
- Reject BigTIFF (magic=43) gracefully
- Walk IFD chains with visited-offsets set (circular chain detection)
- Hard cap: max 20 IFDs
- Collect JPEG previews from Compression=6/7 IFDs via StripOffsets or JPEGInterchangeFormat
- Validate all offsets against file size
- Select largest JPEG preview
- Validate JPEG SOI marker (0xFFD8)
- Return raw JPEG bytes

Key constants:
```go
const (
	tagCompression               = 0x0103
	tagStripOffsets              = 0x0111
	tagStripByteCounts           = 0x0117
	tagJPEGInterchangeFormat     = 0x0201
	tagJPEGInterchangeFormatLen  = 0x0202
	tagNewSubFileType            = 0x00FE
	tagSubIFDs                   = 0x014a

	compressionOJPEG = 6
	compressionJPEG  = 7

	maxIFDs    = 20
	jpegSOI    = 0xFFD8
	tiffMagic  = 42
	bigTIFF    = 43
)
```

Include comprehensive debug logging:
- `logger.Log.Debug("tiff: parsing header", "path", path, "byteOrder", order, "magic", magic, "ifd0Offset", offset)`
- `logger.Log.Debug("tiff: visiting IFD", "index", i, "offset", ifdOffset, "entryCount", count)`
- `logger.Log.Debug("tiff: found JPEG preview", "ifdIndex", i, "offset", jpegOffset, "size", jpegSize)`
- `logger.Log.Debug("tiff: selected largest preview", "offset", bestOffset, "size", bestSize)`

- [ ] **Step 2: Write TIFF parser tests**

Test with synthetic TIFF data (construct minimal valid TIFF headers in-memory):

```go
// internal/raw/tiff_test.go
package raw

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestParseTIFF_LittleEndian(t *testing.T) {
	// Build a minimal TIFF file with one IFD containing a JPEG preview
	f := buildMinimalTIFF(t, binary.LittleEndian)
	defer os.Remove(f)

	data, err := extractTIFFPreview(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		t.Fatal("expected JPEG SOI marker")
	}
}

func TestParseTIFF_BigEndian(t *testing.T) {
	f := buildMinimalTIFF(t, binary.BigEndian)
	defer os.Remove(f)

	data, err := extractTIFFPreview(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		t.Fatal("expected JPEG SOI marker")
	}
}

func TestParseTIFF_BigTIFF_Rejected(t *testing.T) {
	f := buildBigTIFF(t)
	defer os.Remove(f)

	_, err := extractTIFFPreview(f)
	if err == nil {
		t.Fatal("expected error for BigTIFF")
	}
}

func TestParseTIFF_CircularChain(t *testing.T) {
	f := buildCircularTIFF(t)
	defer os.Remove(f)

	// Must not hang — should return error or empty result
	_, err := extractTIFFPreview(f)
	// Error is acceptable, infinite loop is not
	_ = err
}

func TestParseTIFF_OffsetBeyondEOF(t *testing.T) {
	f := buildTIFFWithBadOffset(t)
	defer os.Remove(f)

	_, err := extractTIFFPreview(f)
	if err == nil {
		t.Fatal("expected error for offset beyond EOF")
	}
}

func TestParseTIFF_EmptyFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "empty.cr2")
	os.WriteFile(f, []byte{}, 0o644)

	_, err := extractTIFFPreview(f)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestParseTIFF_TruncatedHeader(t *testing.T) {
	f := filepath.Join(t.TempDir(), "truncated.cr2")
	os.WriteFile(f, []byte{0x49, 0x49, 0x2A, 0x00}, 0o644) // Only 4 bytes

	_, err := extractTIFFPreview(f)
	if err == nil {
		t.Fatal("expected error for truncated header")
	}
}
```

Additionally, add tests for spec-required edge cases:

```go
func TestParseTIFF_ValueInOffsetField(t *testing.T) {
	// TIFF tag where count*typeSize <= 4, value stored directly in offset field
	f := buildTIFFWithValueInOffset(t)
	defer os.Remove(f)

	_, err := extractTIFFPreview(f)
	// Should not error — value-in-offset is a valid TIFF construct
	_ = err
}

func TestParseTIFF_MultipleStrips(t *testing.T) {
	// TIFF IFD with array StripOffsets (multiple strips forming one JPEG)
	f := buildTIFFWithMultipleStrips(t)
	defer os.Remove(f)

	data, err := extractTIFFPreview(f)
	if err != nil {
		t.Fatalf("multiple strips should be handled: %v", err)
	}
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		t.Fatal("expected valid JPEG from concatenated strips")
	}
}

func TestParseTIFF_SubIFDArrays(t *testing.T) {
	// TIFF with tag 0x014a (SubIFDs) pointing to child IFDs containing JPEG preview
	f := buildTIFFWithSubIFDs(t)
	defer os.Remove(f)

	data, err := extractTIFFPreview(f)
	if err != nil {
		t.Fatalf("SubIFD traversal should find preview: %v", err)
	}
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		t.Fatal("expected valid JPEG from SubIFD")
	}
}

func TestParseTIFF_JPEGInterchangeFormat(t *testing.T) {
	// TIFF IFD using JPEGInterchangeFormat/Length tags instead of StripOffsets
	f := buildTIFFWithJPEGInterchange(t)
	defer os.Remove(f)

	data, err := extractTIFFPreview(f)
	if err != nil {
		t.Fatalf("JPEGInterchangeFormat should be handled: %v", err)
	}
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		t.Fatal("expected valid JPEG")
	}
}
```

Note: All `build*` helper functions construct synthetic binary data. Implement them in a `tiff_test_helpers_test.go` file.

- [ ] **Step 3: Run TIFF parser tests**

Run: `go test ./internal/raw/ -v -run 'TestParseTIFF'`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/raw/tiff.go internal/raw/tiff_test.go
git commit -m "feat(raw): implement TIFF IFD parser with safety guards"
```

---

### Task 3: Preview Extraction (Path A + B combined)

**Files:**
- Create: `internal/raw/preview.go`
- Create: `internal/raw/preview_test.go`

- [ ] **Step 1: Add imagemeta dependency**

Run: `go get github.com/evanoberholster/imagemeta`

- [ ] **Step 2: Create `internal/raw/preview.go`**

This is the main entry point: `ExtractPreview(path string) ([]byte, error)`

Flow:
1. Detect format via file extension (fast) — use `raw.IsRAWExt()`
2. For CR3: use imagemeta to extract PRVW box preview
3. For CR2/NEF/ARW/DNG: use TIFF IFD parser from Task 2
4. Validate result: check JPEG SOI, check dimensions >= 400px wide
5. If Path A fails or preview too small: fall back to dcraw (Path B)
6. Return JPEG bytes

```go
package raw

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cullsnap/internal/logger"
)

// ExtractPreview returns the embedded JPEG preview bytes from a RAW file.
// Uses Path A (Pure Go) first, falls back to Path B (dcraw) on failure.
func ExtractPreview(path string) ([]byte, error) {
	start := time.Now()
	ext := strings.ToLower(filepath.Ext(path))

	if !IsRAWExt(ext) {
		return nil, fmt.Errorf("not a RAW file: %s", ext)
	}

	format := FormatName(ext)
	logger.Log.Debug("raw: extracting preview", "path", path, "format", format)

	// Path A: Pure Go extraction
	var data []byte
	var err error

	switch ext {
	case ".cr3":
		data, err = extractCR3Preview(path)
	case ".cr2", ".nef", ".arw", ".dng":
		data, err = extractTIFFPreview(path)
	default:
		// Formats without Path A (RAF, RW2, ORF, NRW, PEF, SRW) go directly to dcraw
		logger.Log.Debug("raw: no Path A for format, trying dcraw", "format", format)
		data, err = ExtractPreviewDcraw(path)
		if err != nil {
			return nil, fmt.Errorf("dcraw extraction failed for %s: %w", format, err)
		}
		logger.Log.Debug("raw: dcraw extraction succeeded", "path", path, "bytes", len(data), "duration", time.Since(start))
		return data, nil
	}

	// Check Path A result
	if err != nil || len(data) == 0 {
		logger.Log.Warn("raw: Path A failed, falling back to dcraw", "path", path, "format", format, "error", err)
		data, err = ExtractPreviewDcraw(path)
		if err != nil {
			return nil, fmt.Errorf("both Path A and dcraw failed for %s: %w", format, err)
		}
		logger.Log.Debug("raw: dcraw fallback succeeded", "path", path, "bytes", len(data))
		return data, nil
	}

	// Validate preview dimensions (must be >= 400px wide)
	if !isPreviewLargeEnough(data, 400) {
		logger.Log.Warn("raw: Path A preview too small, falling back to dcraw", "path", path, "format", format)
		dcrawData, dcrawErr := ExtractPreviewDcraw(path)
		if dcrawErr == nil && len(dcrawData) > 0 {
			data = dcrawData
		}
		// If dcraw also fails, use the small preview rather than nothing
	}

	logger.Log.Debug("raw: preview extracted", "path", path, "format", format, "bytes", len(data), "duration", time.Since(start))
	return data, nil
}

// isPreviewLargeEnough decodes JPEG headers to check if width >= minWidth.
func isPreviewLargeEnough(jpegData []byte, minWidth int) bool {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpegData))
	if err != nil {
		return false
	}
	return cfg.Width >= minWidth
}

// extractCR3Preview extracts the JPEG preview from a CR3 (BMFF) file using imagemeta.
func extractCR3Preview(path string) ([]byte, error) {
	// Implementation uses imagemeta library
	// Will be filled in during implementation
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Use imagemeta to extract CR3 preview
	// TODO: implement with imagemeta API
	return nil, fmt.Errorf("CR3 extraction not yet implemented")
}
```

- [ ] **Step 3: Write preview extraction tests**

Tests should cover:
- ExtractPreview with each TIFF-based format (using synthetic TIFF from Task 2 helpers)
- ExtractPreview returns error for non-RAW file
- ExtractPreview falls back to dcraw when Path A fails
- isPreviewLargeEnough with valid and invalid JPEG data

- [ ] **Step 4: Run tests**

Run: `go test ./internal/raw/ -v -run 'TestExtractPreview'`
Expected: PASS (dcraw fallback tests may skip if dcraw not installed)

- [ ] **Step 5: Commit**

```bash
git add internal/raw/preview.go internal/raw/preview_test.go go.mod go.sum
git commit -m "feat(raw): implement preview extraction with Path A + B fallback"
```

---

### Task 4: dcraw Binary Provisioning

**Files:**
- Create: `internal/raw/dcraw.go`
- Create: `internal/raw/dcraw_test.go`

Follow the exact pattern from `internal/video/ffmpeg.go:26-54` for Init() and download.

- [ ] **Step 1: Create `internal/raw/dcraw.go`**

```go
package raw

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"cullsnap/internal/logger"
)

var (
	dcrawPath string
	dcrawAvailable bool
)

const dcrawTimeout = 30 * time.Second

// Init initializes the dcraw binary. Call during app startup.
// If dcraw is not found, Path B is disabled but Path A still works.
func Init() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}

	binDir := filepath.Join(home, ".cullsnap", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("cannot create bin directory: %w", err)
	}

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}

	dcrawPath = filepath.Join(binDir, "dcraw"+ext)

	// Check if already installed
	if _, err := os.Stat(dcrawPath); err == nil {
		dcrawAvailable = true
		logger.Log.Info("raw: dcraw binary found", "path", dcrawPath)
		return nil
	}

	logger.Log.Info("raw: dcraw binary not found, attempting download", "targetPath", dcrawPath)
	if err := downloadDcraw(); err != nil {
		logger.Log.Warn("raw: dcraw download failed, Path B disabled", "error", err)
		return nil // Graceful degradation — Path A only
	}

	dcrawAvailable = true
	logger.Log.Info("raw: dcraw binary installed", "path", dcrawPath)
	return nil
}

// ExtractPreviewDcraw extracts the embedded JPEG preview using dcraw.
// Returns JPEG bytes or error. Has a 30-second timeout.
func ExtractPreviewDcraw(path string) ([]byte, error) {
	if !dcrawAvailable {
		return nil, fmt.Errorf("dcraw not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), dcrawTimeout)
	defer cancel()

	logger.Log.Debug("raw: invoking dcraw", "path", path, "timeout", dcrawTimeout)

	// dcraw -e -c <file> outputs JPEG preview to stdout
	cmd := exec.CommandContext(ctx, dcrawPath, "-e", "-c", path)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logger.Log.Error("raw: dcraw timed out", "path", path, "timeout", dcrawTimeout)
			return nil, fmt.Errorf("dcraw timed out after %v", dcrawTimeout)
		}
		logger.Log.Error("raw: dcraw failed", "path", path, "error", err)
		return nil, fmt.Errorf("dcraw failed: %w", err)
	}

	if len(out) < 2 || out[0] != 0xFF || out[1] != 0xD8 {
		logger.Log.Warn("raw: dcraw output is not JPEG", "path", path, "bytes", len(out))
		return nil, fmt.Errorf("dcraw output is not valid JPEG")
	}

	logger.Log.Debug("raw: dcraw extraction succeeded", "path", path, "bytes", len(out))
	return out, nil
}

func downloadDcraw() error {
	// Platform-specific dcraw download
	// Follow same pattern as internal/video/ffmpeg.go downloadFFmpeg()
	// TODO: implement with actual dcraw download URLs
	return fmt.Errorf("dcraw auto-download not yet implemented")
}
```

- [ ] **Step 2: Write dcraw tests**

```go
// internal/raw/dcraw_test.go
package raw

import "testing"

func TestExtractPreviewDcraw_NotAvailable(t *testing.T) {
	// Ensure dcraw is marked unavailable
	origAvailable := dcrawAvailable
	dcrawAvailable = false
	defer func() { dcrawAvailable = origAvailable }()

	_, err := ExtractPreviewDcraw("/some/file.raf")
	if err == nil {
		t.Fatal("expected error when dcraw not available")
	}
}

func TestInit_GracefulWhenDownloadFails(t *testing.T) {
	// Init should not return error even if download fails
	// It should log a warning and set dcrawAvailable = false
	// This test verifies graceful degradation
	origPath := dcrawPath
	origAvailable := dcrawAvailable
	defer func() {
		dcrawPath = origPath
		dcrawAvailable = origAvailable
	}()

	dcrawPath = "/nonexistent/path/dcraw"
	dcrawAvailable = false

	err := Init()
	if err != nil {
		t.Fatalf("Init should not error on download failure: %v", err)
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/raw/ -v -run 'TestExtractPreviewDcraw|TestInit'`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/raw/dcraw.go internal/raw/dcraw_test.go
git commit -m "feat(raw): add dcraw binary provisioning with 30s timeout"
```

---

### Task 5: RAW+JPEG Companion Pairing

**Files:**
- Create: `internal/raw/pair.go`
- Create: `internal/raw/pair_test.go`

- [ ] **Step 1: Create `internal/raw/pair.go`**

```go
package raw

import (
	"path/filepath"
	"strings"

	"cullsnap/internal/logger"
	"cullsnap/internal/model"
)

var jpegExtensions = map[string]bool{
	".jpg": true, ".jpeg": true,
}

// PairRAWJPEG identifies RAW+JPEG companion pairs in a photo list.
// Same base filename + same directory = pair.
// RAW is primary, JPEG is marked as companion.
// Modifies photos in-place and returns the same slice.
func PairRAWJPEG(photos []model.Photo) []model.Photo {
	// Build lookup: dir+baseName → indices
	type fileKey struct {
		dir  string
		base string // lowercase, no extension
	}

	rawFiles := make(map[fileKey]int)  // key → index of RAW file
	jpegFiles := make(map[fileKey]int) // key → index of JPEG file

	for i, p := range photos {
		ext := strings.ToLower(filepath.Ext(p.Path))
		dir := filepath.Dir(p.Path)
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(p.Path), filepath.Ext(p.Path)))
		key := fileKey{dir: dir, base: base}

		if IsRAWExt(ext) {
			rawFiles[key] = i
		} else if jpegExtensions[ext] {
			jpegFiles[key] = i
		}
	}

	// Find pairs
	pairCount := 0
	for key, rawIdx := range rawFiles {
		jpegIdx, hasCompanion := jpegFiles[key]
		if !hasCompanion {
			continue
		}

		// RAW is primary, JPEG is companion
		photos[rawIdx].CompanionPath = photos[jpegIdx].Path
		photos[jpegIdx].CompanionPath = photos[rawIdx].Path
		photos[jpegIdx].IsRAWCompanion = true

		pairCount++
		logger.Log.Debug("raw: paired RAW+JPEG", "raw", photos[rawIdx].Path, "jpeg", photos[jpegIdx].Path)
	}

	unpaired := len(rawFiles) - pairCount
	logger.Log.Info("raw: pairing complete",
		"totalFiles", len(photos),
		"pairs", pairCount,
		"unpairedRAW", unpaired,
		"unpairedJPEG", len(jpegFiles)-pairCount,
	)

	return photos
}
```

- [ ] **Step 2: Write comprehensive pairing tests**

```go
// internal/raw/pair_test.go
package raw

import (
	"testing"

	"cullsnap/internal/model"
)

func TestPairRAWJPEG_SameBaseNameSameDir(t *testing.T) {
	photos := []model.Photo{
		{Path: "/photos/IMG_0042.CR3", IsRAW: true, RAWFormat: "CR3"},
		{Path: "/photos/IMG_0042.JPG"},
	}
	result := PairRAWJPEG(photos)

	if result[0].CompanionPath != "/photos/IMG_0042.JPG" {
		t.Errorf("RAW should have JPEG companion, got %q", result[0].CompanionPath)
	}
	if !result[1].IsRAWCompanion {
		t.Error("JPEG should be marked as RAW companion")
	}
	if result[1].CompanionPath != "/photos/IMG_0042.CR3" {
		t.Errorf("JPEG should point to RAW companion, got %q", result[1].CompanionPath)
	}
}

func TestPairRAWJPEG_SameBaseNameDifferentDir(t *testing.T) {
	photos := []model.Photo{
		{Path: "/raw/IMG_0042.CR3", IsRAW: true, RAWFormat: "CR3"},
		{Path: "/jpg/IMG_0042.JPG"},
	}
	result := PairRAWJPEG(photos)

	if result[0].CompanionPath != "" {
		t.Error("different directories should NOT be paired")
	}
	if result[1].IsRAWCompanion {
		t.Error("JPEG in different dir should NOT be marked as companion")
	}
}

func TestPairRAWJPEG_RAWWithoutCompanion(t *testing.T) {
	photos := []model.Photo{
		{Path: "/photos/IMG_0043.CR3", IsRAW: true, RAWFormat: "CR3"},
	}
	result := PairRAWJPEG(photos)

	if result[0].CompanionPath != "" {
		t.Error("RAW without JPEG should have empty CompanionPath")
	}
}

func TestPairRAWJPEG_JPEGWithoutCompanion(t *testing.T) {
	photos := []model.Photo{
		{Path: "/photos/IMG_0044.JPG"},
	}
	result := PairRAWJPEG(photos)

	if result[0].IsRAWCompanion {
		t.Error("JPEG without RAW should NOT be marked as companion")
	}
}

func TestPairRAWJPEG_CaseInsensitive(t *testing.T) {
	photos := []model.Photo{
		{Path: "/photos/img_0042.cr3", IsRAW: true, RAWFormat: "CR3"},
		{Path: "/photos/IMG_0042.JPG"},
	}
	result := PairRAWJPEG(photos)

	if result[0].CompanionPath == "" {
		t.Error("case-insensitive matching should find pair")
	}
}

func TestPairRAWJPEG_MultipleFormats(t *testing.T) {
	photos := []model.Photo{
		{Path: "/photos/A.CR3", IsRAW: true, RAWFormat: "CR3"},
		{Path: "/photos/A.JPG"},
		{Path: "/photos/B.ARW", IsRAW: true, RAWFormat: "ARW"},
		{Path: "/photos/B.jpeg"},
		{Path: "/photos/C.NEF", IsRAW: true, RAWFormat: "NEF"},
	}
	result := PairRAWJPEG(photos)

	if result[0].CompanionPath == "" {
		t.Error("CR3+JPG should be paired")
	}
	if result[2].CompanionPath == "" {
		t.Error("ARW+jpeg should be paired")
	}
	if result[4].CompanionPath != "" {
		t.Error("NEF without companion should not be paired")
	}
}

func TestPairRAWJPEG_JpegExtensions(t *testing.T) {
	photos := []model.Photo{
		{Path: "/photos/test.ARW", IsRAW: true, RAWFormat: "ARW"},
		{Path: "/photos/test.jpeg"},
	}
	result := PairRAWJPEG(photos)

	if result[0].CompanionPath == "" {
		t.Error(".jpeg extension should pair with RAW")
	}
}
```

- [ ] **Step 3: Run pairing tests**

Run: `go test ./internal/raw/ -v -run 'TestPairRAWJPEG'`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/raw/pair.go internal/raw/pair_test.go
git commit -m "feat(raw): implement RAW+JPEG companion pairing"
```

---

### Task 6: Scanner Integration

**Files:**
- Modify: `internal/scanner/scanner.go:10-21` (allowedExtensions), `:65-71` (isVideoExt)

- [ ] **Step 1: Update scanner to use shared extension registry**

In `internal/scanner/scanner.go`:

1. Add RAW extensions to `allowedExtensions` map by importing `raw.Extensions` and merging at init time, OR add them statically. Using the shared registry is preferred:

```go
import (
	"cullsnap/internal/raw"
)

// In init() or at declaration, merge raw.Extensions into allowedExtensions
func init() {
	for ext := range raw.Extensions {
		allowedExtensions[ext] = true
	}
}
```

2. Add `isRAWExt` usage: In `ScanDirectory`, when building `model.Photo`, set `IsRAW` and `RAWFormat`:

```go
photo := model.Photo{
	Path:    filePath,
	Size:    info.Size(),
	IsVideo: isVideoExt(ext),
}
if raw.IsRAWExt(ext) {
	photo.IsRAW = true
	photo.RAWFormat = raw.FormatName(ext)
}
```

- [ ] **Step 2: Run scanner tests**

Run: `go test ./internal/scanner/ -v`
Expected: PASS (existing tests should pass, RAW files now included in scans)

- [ ] **Step 3: Commit**

```bash
git add internal/scanner/scanner.go
git commit -m "feat(raw): add RAW extension support to scanner"
```

---

### Task 7: Thumbnail Pipeline Integration

**Files:**
- Modify: `internal/image/thumbnail.go:14-57` (GetThumbnail)
- Modify: `internal/image/thumbcache.go:49-114` (GenerateThumbnail)

- [ ] **Step 1: Update `GenerateThumbnail` in thumbcache.go**

After the video detection switch at lines 60-65, add RAW detection:

```go
isRAW := raw.IsRAWExt(ext)
```

Add a new block before the existing photo handling (before line 80):

```go
if isRAW {
	logger.Log.Debug("thumbcache: generating RAW thumbnail", "path", path, "format", raw.FormatName(ext))
	previewBytes, err := raw.ExtractPreview(path)
	if err != nil {
		return "", fmt.Errorf("RAW preview extraction failed for %s: %w", filepath.Base(path), err)
	}

	// Decode JPEG preview bytes
	img, err := jpeg.Decode(bytes.NewReader(previewBytes))
	if err != nil {
		return "", fmt.Errorf("failed to decode RAW preview JPEG: %w", err)
	}

	// Resize to thumbnail dimensions
	thumb := imaging.Resize(img, 300, 0, imaging.Box)

	// Atomic write: temp file → rename (same pattern as existing photo thumbnails)
	tmpPath := thumbPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", fmt.Errorf("failed to create thumbnail file: %w", err)
	}

	if err := jpeg.Encode(f, thumb, &jpeg.Options{Quality: 80}); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("failed to encode thumbnail: %w", err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("failed to close thumbnail file: %w", err)
	}

	if err := os.Rename(tmpPath, thumbPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}

	logger.Log.Debug("thumbcache: RAW thumbnail generated", "path", path)
	return thumbPath, nil
}
```

Add necessary imports: `"bytes"`, `"image/jpeg"`, `"cullsnap/internal/raw"`, `"github.com/disintegration/imaging"`

- [ ] **Step 2: Run thumbnail tests**

Run: `go test ./internal/image/ -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/image/thumbnail.go internal/image/thumbcache.go
git commit -m "feat(raw): route RAW files through preview extraction in thumbnail pipeline"
```

---

### Task 8: Dedup Hash + Quality Integration

**Files:**
- Modify: `internal/dedupe/hash.go:33-63` (hashImage), `:70-72` (extensions filter)
- Modify: `internal/dedupe/quality.go:13-86` (CalculateLaplacianVariance)

- [ ] **Step 1: Update `hashImage` in hash.go for RAW support**

At the start of `hashImage()`, detect RAW and extract preview:

```go
func hashImage(path string) (*goimagehash.ImageHash, error) {
	ext := strings.ToLower(filepath.Ext(path))

	var img image.Image
	var err error

	if raw.IsRAWExt(ext) {
		// Extract JPEG preview from RAW, decode to image.Image
		previewBytes, extractErr := raw.ExtractPreview(path)
		if extractErr != nil {
			return nil, fmt.Errorf("RAW preview extraction failed: %w", extractErr)
		}
		img, err = jpeg.Decode(bytes.NewReader(previewBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to decode RAW preview: %w", err)
		}
	} else {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		img, _, err = image.Decode(file)
		if err != nil {
			return nil, err
		}
	}

	thumb := imaging.Resize(img, 256, 0, imaging.NearestNeighbor)
	hash, err := goimagehash.DifferenceHash(thumb)
	return hash, err
}
```

- [ ] **Step 2: Update `FindDuplicates` extension filter**

Replace the hardcoded extensions map at line 70-72 with:

```go
extensions := raw.ImageExtensions()
```

- [ ] **Step 3: Update `CalculateLaplacianVariance` in quality.go**

Same pattern — detect RAW at the start, extract preview, decode from bytes:

```go
func CalculateLaplacianVariance(imgPath string) (float64, error) {
	ext := strings.ToLower(filepath.Ext(imgPath))

	var img image.Image

	if raw.IsRAWExt(ext) {
		previewBytes, err := raw.ExtractPreview(imgPath)
		if err != nil {
			return 0, fmt.Errorf("RAW preview extraction failed: %w", err)
		}
		decoded, err := jpeg.Decode(bytes.NewReader(previewBytes))
		if err != nil {
			return 0, fmt.Errorf("failed to decode RAW preview: %w", err)
		}
		img = decoded
	} else {
		file, err := os.Open(imgPath)
		if err != nil {
			return 0, err
		}
		defer file.Close()
		decoded, _, err := image.Decode(file)
		if err != nil {
			return 0, err
		}
		img = decoded
	}

	// existing resize + Laplacian logic continues unchanged...
```

- [ ] **Step 4: Run dedup tests**

Run: `go test ./internal/dedupe/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/dedupe/hash.go internal/dedupe/quality.go
git commit -m "feat(raw): integrate RAW preview extraction into dedup hash and quality pipelines"
```

---

### Task 9: EXIF Extraction for RAW Files

**Files:**
- Modify: `internal/dedupe/sorter.go:22-42` (ExtractDateTaken), `:54-120` (ExtractFullEXIF)
- Modify: `internal/app/app.go` (GetPhotoEXIF)

- [ ] **Step 1: Update `ExtractDateTaken` for RAW files**

For TIFF-based RAW (CR2, NEF, ARW, DNG), `exif.Decode()` already works on the file stream because these files start with a valid TIFF header. For CR3/RAF/RW2/ORF, extract EXIF from the embedded JPEG preview:

```go
func ExtractDateTaken(path string) (time.Time, bool) {
	ext := strings.ToLower(filepath.Ext(path))

	// For non-TIFF RAW formats, extract EXIF from preview JPEG
	if raw.IsRAWExt(ext) && !isTIFFBasedRAW(ext) {
		previewBytes, err := raw.ExtractPreview(path)
		if err != nil {
			return time.Time{}, false
		}
		e, err := exif.Decode(bytes.NewReader(previewBytes))
		if err != nil {
			return time.Time{}, false
		}
		date, err := e.DateTime()
		if err != nil {
			return time.Time{}, false
		}
		return date, true
	}

	// TIFF-based RAW and regular JPEG/PNG — existing path
	file, err := os.Open(path)
	if err != nil {
		return time.Time{}, false
	}
	defer file.Close()
	// ... existing exif.Decode() logic
}

func isTIFFBasedRAW(ext string) bool {
	switch ext {
	case ".cr2", ".nef", ".arw", ".dng":
		return true
	}
	return false
}
```

- [ ] **Step 2: Update `ExtractFullEXIF` with same pattern**

Apply the same TIFF-based vs non-TIFF-based routing.

- [ ] **Step 3: Update `GetPhotoEXIF` in app.go**

The existing `GetPhotoEXIF` at line 637-650 calls `dedupe.ExtractFullEXIF()`. The changes to `ExtractFullEXIF` should make this work transparently. Add logging:

```go
logger.Log.Debug("app: extracting EXIF", "path", path, "isRAW", raw.IsRAWExt(filepath.Ext(path)))
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/dedupe/ -v -run 'TestExtract'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/dedupe/sorter.go internal/app/app.go
git commit -m "feat(raw): add EXIF extraction support for all RAW formats"
```

---

### Task 10: App Wiring (Startup + ScanAndDeduplicate + CheckDedupStatus)

**Files:**
- Modify: `internal/app/app.go:74-99` (Startup), `:675-717` (CheckDedupStatus), `:719-813` (ScanAndDeduplicate)

- [ ] **Step 1: Add `raw.Init()` to Startup**

In `Startup()` around line 97, after updater initialization:

```go
// Initialize RAW module (dcraw provisioning)
if err := raw.Init(); err != nil {
	logger.Log.Error("Failed to initialize RAW module", "error", err)
	// Graceful degradation — Path A only, no dcraw
}
logger.Log.Info("app: RAW module initialized")
```

- [ ] **Step 2: Update CheckDedupStatus to use shared registry**

At line 694 in `CheckDedupStatus`, replace the hardcoded extensions with:

```go
ext := strings.ToLower(filepath.Ext(entry.Name()))
isImage := ext == ".jpg" || ext == ".jpeg" || ext == ".png" || raw.IsRAWExt(ext)
```

- [ ] **Step 3: Wire PairRAWJPEG into ScanAndDeduplicate**

**Architecture note:** `ScanAndDeduplicate()` delegates file discovery to `dedupe.FindDuplicates()` which does its own `filepath.WalkDir`. There is no `[]model.Photo` before dedup runs. The solution is:

1. Add a pre-scan step using `scanner.ScanDirectory()` before calling `FindDuplicates()`
2. Run `raw.PairRAWJPEG()` on the scanned photos
3. Store pairing metadata in a map keyed by path
4. After `FindDuplicates()` returns groups, propagate RAW fields into the `photoModel` construction

In `ScanAndDeduplicate()`, add before `dedupe.FindDuplicates()` (after line 749):

```go
// Pre-scan to identify RAW files and RAW+JPEG pairs
scannedPhotos, scanErr := scanner.ScanDirectory(path)
if scanErr != nil {
	logger.Log.Warn("raw: pre-scan failed, proceeding without pairing", "error", scanErr)
}
scannedPhotos = raw.PairRAWJPEG(scannedPhotos)

// Build lookup for RAW metadata by path
rawMeta := make(map[string]model.Photo, len(scannedPhotos))
for _, p := range scannedPhotos {
	if p.IsRAW || p.IsRAWCompanion {
		rawMeta[p.Path] = p
	}
}
```

Then in the `photoModel` construction loop (around line 795-799), propagate RAW fields:

```go
photoModel := model.Photo{
	Path:    p.Path,
	Size:    info.Size(),
	TakenAt: date,
}

// Propagate RAW metadata from pre-scan
if meta, ok := rawMeta[p.Path]; ok {
	photoModel.IsRAW = meta.IsRAW
	photoModel.RAWFormat = meta.RAWFormat
	photoModel.CompanionPath = meta.CompanionPath
	photoModel.IsRAWCompanion = meta.IsRAWCompanion
}
```

This ensures the frontend receives `IsRAW`, `RAWFormat`, `CompanionPath`, and `IsRAWCompanion` so badges render correctly.

- [ ] **Step 4: Run app tests**

Run: `go test ./internal/app/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/app.go
git commit -m "feat(raw): wire RAW init, pairing, and extension registry into app"
```

---

### Task 11: Media Server — RAW Preview Serving + Cache

**Files:**
- Modify: `main.go:78-127` (ServeHTTP handler), `:129-131` (video extensions), `:196-200` (MIME types)
- Create: `internal/raw/previewcache.go`

- [ ] **Step 1: Create preview cache**

```go
// internal/raw/previewcache.go
package raw

import (
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"

	"cullsnap/internal/logger"
)

var previewCacheDir string

// InitPreviewCache creates the preview cache directory.
func InitPreviewCache() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	previewCacheDir = filepath.Join(home, ".cullsnap", "previews")
	return os.MkdirAll(previewCacheDir, 0o755)
}

// GetCachedPreview returns cached preview JPEG bytes, or nil if not cached.
func GetCachedPreview(path string) ([]byte, error) {
	cachePath := previewCachePath(path)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err // Cache miss
	}
	logger.Log.Debug("raw: preview cache hit", "path", path)
	return data, nil
}

// CachePreview writes preview JPEG bytes to the cache.
func CachePreview(path string, data []byte) error {
	cachePath := previewCachePath(path)
	return os.WriteFile(cachePath, data, 0o644)
}

func previewCachePath(path string) string {
	info, err := os.Stat(path)
	var modTime string
	if err == nil {
		modTime = info.ModTime().String()
	}
	hash := md5.Sum([]byte(path + modTime))
	return filepath.Join(previewCacheDir, fmt.Sprintf("%x.jpg", hash))
}
```

- [ ] **Step 2: Update media server in main.go**

In the `ServeHTTP` handler, before `http.ServeFile`, add RAW detection:

```go
ext := strings.ToLower(filepath.Ext(cleanPath))
if raw.IsRAWExt(ext) {
	logger.Log.Debug("media: serving RAW preview", "path", cleanPath)

	// Try cache first
	if cached, err := raw.GetCachedPreview(cleanPath); err == nil {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "private, max-age=3600")
		w.Write(cached)
		return
	}

	// Extract and cache
	start := time.Now()
	previewBytes, err := raw.ExtractPreview(cleanPath)
	if err != nil {
		logger.Log.Error("media: RAW preview extraction failed", "path", cleanPath, "error", err)
		http.Error(w, "failed to extract RAW preview", http.StatusInternalServerError)
		return
	}

	raw.CachePreview(cleanPath, previewBytes)
	logger.Log.Debug("media: RAW preview served", "path", cleanPath, "bytes", len(previewBytes), "duration", time.Since(start))

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Write(previewBytes)
	return
}
```

Also add RAW extensions to the media server's allowed extensions if they have their own allowlist.

- [ ] **Step 3: Add `InitPreviewCache()` call to main**

In `main()` around line 222 (near video init):

```go
if err := raw.InitPreviewCache(); err != nil {
	logger.Log.Warn("Failed to initialize preview cache", "error", err)
}
```

- [ ] **Step 4: Commit**

```bash
git add internal/raw/previewcache.go main.go
git commit -m "feat(raw): add media server RAW preview serving with disk cache"
```

---

### Task 12: Frontend — Grid RAW Badge

**Files:**
- Modify: `frontend/src/components/Grid.tsx:130-138` (badge area)
- Modify: `frontend/src/index.css:474-498` (badge styles)

- [ ] **Step 1: Add `.badge-raw` CSS**

In `frontend/src/index.css`, after `.badge-exported` (line 498):

```css
.badge-raw {
  position: absolute;
  bottom: 6px;
  left: 6px;
  background: #D97706;
  color: white;
  border-radius: 4px;
  padding: 1px 5px;
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.5px;
  width: auto;
  height: auto;
  display: flex;
  align-items: center;
  justify-content: center;
}
```

- [ ] **Step 2: Add RAW badge to Grid.tsx**

In `Grid.tsx`, after the video badge block (line 138), add:

```tsx
{photo.IsRAW && photo.RAWFormat && (
    <div className="badge-raw">
        {photo.RAWFormat}
    </div>
)}
```

- [ ] **Step 3: Verify TypeScript types**

After adding fields to the Go model, regenerate Wails TS bindings:

Run: `cd /Users/abhishekmitra/Local_Documents/Local_Developments/photo_sorter/CullSnap && wails generate module`

Verify that `frontend/wailsjs/go/models.ts` has `isRAW`, `rawFormat`, `companionPath`, `isRAWCompanion` fields.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/Grid.tsx frontend/src/index.css
git commit -m "feat(raw): add RAW format badge to grid thumbnails"
```

---

### Task 13: Frontend — Viewer RAW Badge

**Files:**
- Modify: `frontend/src/components/Viewer.tsx:126-142` (image container)
- Modify: `frontend/src/index.css` (viewer badge style)

- [ ] **Step 1: Add viewer badge CSS**

```css
.viewer-raw-badge {
  position: absolute;
  top: 12px;
  left: 12px;
  background: rgba(217, 119, 6, 0.85);
  backdrop-filter: blur(8px);
  color: white;
  border-radius: 6px;
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.5px;
  z-index: 10;
  pointer-events: none;
}
```

- [ ] **Step 2: Add RAW badge to Viewer.tsx**

Inside the `viewer-image-container` div (after line 126), before the `<img>` tag:

```tsx
{photo.IsRAW && photo.RAWFormat && (
    <div className="viewer-raw-badge">
        {photo.RAWFormat}
        {photo.CompanionPath && photo.CompanionPath.endsWith('.JPG') || photo.CompanionPath?.toLowerCase().endsWith('.jpg') || photo.CompanionPath?.toLowerCase().endsWith('.jpeg')
            ? ' + JPG'
            : ''}
    </div>
)}
```

Simplify the companion check:

```tsx
{photo.IsRAW && photo.RAWFormat && (
    <div className="viewer-raw-badge">
        {photo.RAWFormat}{photo.CompanionPath ? ' + JPG' : ''}
    </div>
)}
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/Viewer.tsx frontend/src/index.css
git commit -m "feat(raw): add RAW format overlay badge to viewer"
```

---

### Task 14: Frontend — HelpModal Update

**Files:**
- Modify: `frontend/src/components/HelpModal.tsx:190`

- [ ] **Step 1: Update supported formats text**

At line 190, change:
```
Images: JPG, JPEG, PNG. Videos: MP4, MOV, AVI, MKV, WEBM (when FFmpeg is available).
```

To:
```
Images: JPG, JPEG, PNG, CR2, CR3, ARW, NEF, DNG, RAF, RW2, ORF, NRW, PEF, SRW. Videos: MP4, MOV, AVI, MKV, WEBM (when FFmpeg is available).
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/HelpModal.tsx
git commit -m "feat(raw): update supported formats list in help modal"
```

---

### Task 15: Test Fixtures + Real-World RAW Tests

**Files:**
- Create: `internal/raw/testdata/.gitignore`
- Create: `internal/raw/testdata/download.sh`
- Modify: `internal/raw/preview_test.go` (add real-file tests)

- [ ] **Step 1: Create testdata directory and download script**

```bash
# internal/raw/testdata/.gitignore
*.cr2
*.cr3
*.arw
*.nef
*.dng
*.raf
*.rw2
*.orf
*.nrw
*.pef
*.srw
```

Create `internal/raw/testdata/download.sh`:
```bash
#!/bin/bash
# Downloads real camera RAW samples for testing.
# Source: raw.pixls.us and other public domain sources.
# Run once before testing: bash internal/raw/testdata/download.sh

set -e
DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

echo "Downloading RAW test samples..."

# CR2 - Canon EOS 5D Mark IV
[ -f sample.cr2 ] || curl -L -o sample.cr2 "https://raw.pixls.us/getfile.php/2267/nice/Canon%20-%20Canon%20EOS%205D%20Mark%20IV%20-%20IMG_0004.CR2"

# NEF - Nikon D850
[ -f sample.nef ] || curl -L -o sample.nef "https://raw.pixls.us/getfile.php/3102/nice/Nikon%20-%20D850.NEF"

# ARW - Sony A7R III
[ -f sample.arw ] || curl -L -o sample.arw "https://raw.pixls.us/getfile.php/2803/nice/Sony%20-%20ILCE-7RM3.ARW"

# DNG - Leica Q2
[ -f sample.dng ] || curl -L -o sample.dng "https://raw.pixls.us/getfile.php/3090/nice/Leica%20-%20Q2.DNG"

# RAF - Fujifilm X-T3
[ -f sample.raf ] || curl -L -o sample.raf "https://raw.pixls.us/getfile.php/2793/nice/Fujifilm%20-%20X-T3.RAF"

# RW2 - Panasonic GH5
[ -f sample.rw2 ] || curl -L -o sample.rw2 "https://raw.pixls.us/getfile.php/1878/nice/Panasonic%20-%20DC-GH5.RW2"

# ORF - Olympus E-M1 Mark II
[ -f sample.orf ] || curl -L -o sample.orf "https://raw.pixls.us/getfile.php/2122/nice/Olympus%20-%20E-M1MarkII.ORF"

# CR3 - Canon EOS R5 (check raw.pixls.us for availability, may need alternative source)
[ -f sample.cr3 ] || curl -L -o sample.cr3 "https://raw.pixls.us/getfile.php/3239/nice/Canon%20-%20Canon%20EOS%20R5.CR3" || echo "CR3 sample not available from raw.pixls.us — find an alternative"

# NRW - Nikon Coolpix
[ -f sample.nrw ] || curl -L -o sample.nrw "https://raw.pixls.us/getfile.php/774/nice/Nikon%20-%20COOLPIX%20P7800.NRW" || echo "NRW sample not available — find an alternative"

# PEF - Pentax K-1
[ -f sample.pef ] || curl -L -o sample.pef "https://raw.pixls.us/getfile.php/1884/nice/Pentax%20-%20K-1.PEF" || echo "PEF sample not available — find an alternative"

# SRW - Samsung NX1
[ -f sample.srw ] || curl -L -o sample.srw "https://raw.pixls.us/getfile.php/1362/nice/Samsung%20-%20NX1.SRW" || echo "SRW sample not available — find an alternative"

echo "Done! Downloaded $(ls -1 *.{cr2,cr3,nef,arw,dng,raf,rw2,orf,nrw,pef,srw} 2>/dev/null | wc -l) files."
```

Note: URLs are best-effort from raw.pixls.us. The implementer should verify each URL works and find alternatives from camera sample databases if any fail. All 11 formats should have test fixtures.

- [ ] **Step 2: Add real-file integration tests to preview_test.go**

```go
func TestExtractPreview_RealFiles(t *testing.T) {
	testdata := filepath.Join("testdata")

	tests := []struct {
		file   string
		format string
	}{
		{"sample.cr2", "CR2"},
		{"sample.cr3", "CR3"},
		{"sample.nef", "NEF"},
		{"sample.arw", "ARW"},
		{"sample.dng", "DNG"},
		{"sample.raf", "RAF"},
		{"sample.rw2", "RW2"},
		{"sample.orf", "ORF"},
		{"sample.nrw", "NRW"},
		{"sample.pef", "PEF"},
		{"sample.srw", "SRW"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			path := filepath.Join(testdata, tt.file)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Skipf("test fixture %s not found — run testdata/download.sh first", tt.file)
			}

			data, err := ExtractPreview(path)
			if err != nil {
				t.Fatalf("ExtractPreview(%s) error: %v", tt.format, err)
			}

			if len(data) < 2 {
				t.Fatal("preview too short")
			}
			if data[0] != 0xFF || data[1] != 0xD8 {
				t.Fatal("preview does not start with JPEG SOI marker")
			}

			// Verify dimensions
			cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("failed to decode preview config: %v", err)
			}
			if cfg.Width < 200 {
				t.Errorf("preview too small: %dx%d", cfg.Width, cfg.Height)
			}

			t.Logf("%s preview: %dx%d, %d bytes", tt.format, cfg.Width, cfg.Height, len(data))
		})
	}
}
```

- [ ] **Step 3: Run real-file tests (requires download.sh first)**

Run: `bash internal/raw/testdata/download.sh && go test ./internal/raw/ -v -run 'TestExtractPreview_RealFiles'`
Expected: PASS for all downloaded formats, SKIP for missing ones

- [ ] **Step 4: Commit**

```bash
git add internal/raw/testdata/.gitignore internal/raw/testdata/download.sh internal/raw/preview_test.go
git commit -m "test(raw): add real camera RAW sample tests with download script"
```

---

### Task 16: License Compliance

**Files:**
- Create: `THIRD_PARTY_LICENSES` (project root)

- [ ] **Step 1: Create THIRD_PARTY_LICENSES file**

Include dcraw's GPL-2.0 license notice:

```
This file lists third-party components bundled with CullSnap.

================================================================================
dcraw — Raw Image Decoder
================================================================================
Author: Dave Coffin
License: GPL-2.0
Source: https://www.dechifro.org/dcraw/

dcraw is distributed under the terms of the GNU General Public License
version 2. A copy of the license is available at:
https://www.gnu.org/licenses/old-licenses/gpl-2.0.html

dcraw is used as a fallback binary for extracting embedded JPEG previews
from camera RAW files (RAF, RW2, ORF, NRW, PEF, SRW formats).
```

- [ ] **Step 2: Commit**

```bash
git add THIRD_PARTY_LICENSES
git commit -m "docs: add THIRD_PARTY_LICENSES for bundled dcraw binary"
```

---

### Task 17: Wails Bindings Regeneration + Full Integration Test

**Files:**
- Regenerate: `frontend/wailsjs/go/models.ts`
- Run: Full test suite

- [ ] **Step 1: Regenerate Wails TypeScript bindings**

Run: `cd /Users/abhishekmitra/Local_Documents/Local_Developments/photo_sorter/CullSnap && wails generate module`

Verify the generated `frontend/wailsjs/go/models.ts` includes `isRAW`, `rawFormat`, `companionPath`, `isRAWCompanion` fields.

- [ ] **Step 2: Run full Go test suite**

Run: `go test ./... -count=1`
Expected: All tests PASS

- [ ] **Step 3: Run linter**

Run: `golangci-lint run ./...`
Expected: No errors (may have warnings)

- [ ] **Step 4: Run formatter**

Run: `golangci-lint fmt ./...`
Expected: No formatting changes needed

- [ ] **Step 5: Commit any binding changes**

```bash
git add frontend/wailsjs/
git commit -m "chore: regenerate Wails TypeScript bindings for RAW support"
```

---

### Task 18: Final Verification + Build

- [ ] **Step 1: Build the application**

Run: `wails build`
Expected: Build succeeds for current platform

- [ ] **Step 2: Manual smoke test**

1. Open CullSnap
2. Select a directory with mixed RAW+JPEG files
3. Verify: RAW files appear in grid with format badges
4. Verify: Clicking a RAW file shows full-res preview in Viewer with format badge
5. Verify: RAW+JPEG pairs are correctly identified
6. Verify: EXIF data displays for RAW files
7. Verify: Dedup correctly handles RAW files
8. Verify: Help modal shows all RAW formats

- [ ] **Step 3: Final commit if any fixes needed**

```bash
git add -A
git commit -m "fix(raw): address issues found during smoke testing"
```

---

## Summary of Files

### New files (14):
1. `internal/raw/extensions.go` — Shared extension registry
2. `internal/raw/extensions_test.go` — Extension registry tests
3. `internal/raw/tiff.go` — TIFF IFD parser
4. `internal/raw/tiff_test.go` — TIFF parser tests
5. `internal/raw/preview.go` — Main preview extraction (Path A + B)
6. `internal/raw/preview_test.go` — Preview extraction tests
7. `internal/raw/dcraw.go` — dcraw provisioning
8. `internal/raw/dcraw_test.go` — dcraw tests
9. `internal/raw/pair.go` — RAW+JPEG pairing
10. `internal/raw/pair_test.go` — Pairing tests
11. `internal/raw/previewcache.go` — Full-res preview disk cache
12. `internal/raw/testdata/download.sh` — Test fixture download script
13. `internal/raw/testdata/.gitignore` — Ignore large RAW test files
14. `THIRD_PARTY_LICENSES` — dcraw GPL notice

### Modified files (14):
1. `internal/logger/logger.go` — Configurable log level via CULLSNAP_LOG_LEVEL
2. `internal/model/photo.go` — Add RAW fields
2. `internal/scanner/scanner.go` — RAW extension support
3. `internal/image/thumbnail.go` — RAW thumbnail routing
4. `internal/image/thumbcache.go` — RAW thumbnail generation
5. `internal/dedupe/hash.go` — RAW-aware hashing
6. `internal/dedupe/quality.go` — RAW-aware quality scoring
7. `internal/dedupe/sorter.go` — RAW EXIF extraction
8. `internal/app/app.go` — RAW init, pairing, CheckDedupStatus
9. `main.go` — Media server RAW preview serving
10. `frontend/src/components/Grid.tsx` — RAW format badge
11. `frontend/src/components/Viewer.tsx` — RAW format overlay badge
12. `frontend/src/index.css` — Badge styles
13. `frontend/src/components/HelpModal.tsx` — Formats list
