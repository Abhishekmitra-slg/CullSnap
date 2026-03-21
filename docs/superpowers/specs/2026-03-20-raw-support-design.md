# RAW Image Format Support — Design Specification

**Issue:** #13 — Add support for RAW image formats (CR2, CR3, ARW, NEF, DNG)
**Date:** 2026-03-20 (updated 2026-03-21)
**Author:** Abhishek Mitra

## 1. Context

CullSnap is a photo culling tool for photographers. The goal is to extend the scanner, thumbnail, deduplication, media serving, and UI pipelines to support camera RAW formats. Current support: JPEG, PNG, video.

**Core insight:** We are a culling tool, not a RAW editor. We extract the embedded JPEG preview that cameras bake into every RAW file — no debayering needed.

## 2. Verified RAW Format Data

### Embedded JPEG Preview Locations (per format)

| Format | Container | Preview Location | Typical Size |
|--------|-----------|-----------------|--------------|
| CR2 | TIFF | IFD0, `StripOffsets`/`StripByteCounts`, Compression=6 | ~3000px wide |
| CR3 | BMFF (ISO Base Media) | `PRVW` box (not TIFF) | Full resolution |
| NEF | TIFF | SubIFD 0 (via tag 0x014a), Compression=6 | Full resolution |
| ARW | TIFF | IFD0 `JPEGInterchangeFormat`/`JPEGInterchangeFormatLength` | ~1616-3000px |
| DNG | TIFF | IFD with `NewSubFileType=1`, Compression=7 | Varies (camera-native=full, Adobe-converted=configurable) |
| RAF | Proprietary | Fujifilm-specific structure | Requires dcraw |
| RW2 | Variant TIFF | Panasonic-specific | Requires dcraw |
| ORF | Variant TIFF | Olympus-specific | Requires dcraw |

### Typical File Sizes

| Camera | Format | Single File | 10-file Burst |
|--------|--------|------------|---------------|
| Canon R5 | CR3 | 50-60 MB | 500-600 MB |
| Sony A7R V | ARW | 60-80 MB | 600-800 MB |
| Nikon D850 | NEF | 50-58 MB | 500-580 MB |
| Leica Q3 | DNG | 60-75 MB | 600-750 MB |
| Canon 5D IV | CR2 | 35-42 MB | 350-420 MB |

### EXIF Orientation

Embedded preview pixels are NOT pre-rotated in any major camera brand. The EXIF Orientation tag must be read and applied, or portrait shots appear sideways.

### RAW+JPEG Shooting

~15-30% of photographers shoot RAW+JPEG simultaneously. Every major culling tool (Photo Mechanic, FastRawViewer, Capture One) supports pairing. Must-have feature.

## 3. Architecture: Two Complementary Paths

### Path A — Pure Go (primary, ~200 lines)

1. **imagemeta** library for:
   - CR3 preview extraction (`PreviewCR3()`) — BMFF container, cannot use TIFF parsing
   - Format detection via magic bytes (`imagetype.Scan()`) — detects all RAW formats
   - Library: `github.com/evanoberholster/imagemeta` (MIT, 135 stars, last push 2026-03-11)

2. **Custom TIFF IFD parser** for CR2/NEF/ARW/DNG:
   - Reads 8-byte TIFF header (byte order + magic + IFD0 offset)
   - Walks IFD chains + SubIFDs (tag 0x014a)
   - Finds largest embedded JPEG (Compression=6 or 7)
   - Reads via StripOffsets/StripByteCounts or JPEGInterchangeFormat/JPEGInterchangeFormatLength
   - Validates JPEG SOI marker (0xFFD8) before returning
   - Pure Go, zero CGO, zero external dependencies

### Path B — dcraw fallback (~400KB bundled binary)

- `dcraw -e -c <file>` extracts embedded JPEG preview to stdout from 700+ camera models
- Same bundling pattern as FFmpeg (`internal/video/ffmpeg.go`)
- Triggered when Path A returns no preview, preview too small (<400px), or unsupported format
- Covers: RAF, RW2, ORF, NRW, PEF, SRW, and edge cases in TIFF-based formats
- dcraw is a single ~400KB C binary, compiles on all platforms
- Subprocess timeout: 30 seconds via `context.WithTimeout` to prevent zombie processes on corrupt files

### Why both paths

- Path A covers 90%+ of real-world RAW files with zero dependencies
- Path B handles the long tail and serves as a safety net
- No shortcuts — every format works on day one

## 4. Supported Formats (Complete)

| Format | Ext | Brand | Path A (Pure Go) | Path B (dcraw) |
|--------|-----|-------|-----------------|----------------|
| CR2 | .cr2 | Canon DSLR | TIFF IFD parser | Fallback |
| CR3 | .cr3 | Canon mirrorless | imagemeta PreviewCR3() | Fallback |
| ARW | .arw | Sony | TIFF IFD parser | Fallback |
| NEF | .nef | Nikon | TIFF IFD parser | Fallback |
| DNG | .dng | Adobe/Leica/Ricoh | TIFF IFD parser | Fallback |
| RAF | .raf | Fujifilm | — | Primary |
| RW2 | .rw2 | Panasonic | — | Primary |
| ORF | .orf | Olympus/OM System | — | Primary |
| NRW | .nrw | Nikon compact | — | Primary |
| PEF | .pef | Pentax | — | Primary |
| SRW | .srw | Samsung | — | Primary |

## 5. Shared Extension Registry

All RAW extension checks must use a single shared source of truth to prevent drift:

```go
// internal/raw/extensions.go
package raw

var Extensions = map[string]bool{
    ".cr2": true, ".cr3": true, ".arw": true, ".nef": true, ".dng": true,
    ".raf": true, ".rw2": true, ".orf": true, ".nrw": true, ".pef": true, ".srw": true,
}

func IsRAWExt(ext string) bool {
    return Extensions[strings.ToLower(ext)]
}

func FormatName(ext string) string {
    return strings.ToUpper(strings.TrimPrefix(ext, "."))
}
```

All consumers (scanner, dedup hash, CheckDedupStatus, media server, thumbnail cache) import from this single registry.

## 6. Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/raw/extensions.go` | New | Shared RAW extension registry — single source of truth |
| `internal/raw/preview.go` | New | TIFF IFD parser + CR3 preview via imagemeta. Exports `ExtractPreview(path) ([]byte, error)` returning JPEG bytes |
| `internal/raw/dcraw.go` | New | dcraw binary provisioning + `dcraw -e -c` fallback with 30s timeout |
| `internal/raw/pair.go` | New | `PairRAWJPEG()` companion detection by base filename |
| `internal/scanner/scanner.go` | Modify | Use `raw.Extensions` for walk filter, set IsRAW/RAWFormat |
| `internal/model/photo.go` | Modify | Add IsRAW, RAWFormat, CompanionPath, IsRAWCompanion fields |
| `internal/image/thumbnail.go` | Modify | For RAW files: skip EXIF thumbnail path, go directly to `raw.ExtractPreview()` → decode → resize to 300px |
| `internal/image/thumbcache.go` | Modify | Detect RAW ext via `raw.IsRAWExt()`, route to `raw.ExtractPreview()` |
| `internal/dedupe/hash.go` | Modify | Use `raw.IsRAWExt()` in extensions + call `raw.ExtractPreview()` → `jpeg.Decode()` → then hash. Do NOT call `image.Decode()` on RAW files directly |
| `internal/dedupe/quality.go` | Modify | Same pattern: `raw.ExtractPreview()` → `jpeg.Decode()` → Laplacian Variance. Do NOT call `image.Decode()` on RAW files directly |
| `internal/dedupe/sorter.go` | Modify | Handle EXIF extraction for RAW files: for TIFF-based RAW, `exif.Decode()` works on file stream; for CR3/RAF/RW2/ORF, extract from embedded JPEG preview or use imagemeta |
| `internal/app/app.go` | Modify | Wire RAW+JPEG pairing into ScanAndDeduplicate(), call `raw.Init()` in Startup(), update `CheckDedupStatus()` to use `raw.Extensions`, update `GetPhotoEXIF()` for RAW files |
| `main.go` | Modify | Add RAW extensions to media server allowlist via `raw.Extensions`, serve extracted previews for RAW files |
| `frontend/src/components/Grid.tsx` | Modify | Add RAW format badge on thumbnails |
| `frontend/src/components/Viewer.tsx` | Modify | Add RAW format badge overlay on viewer |
| `frontend/src/index.css` | Modify | Add `.badge-raw` styles (explicit CSS, not relying on nonexistent `.badge-video`) |
| `frontend/src/components/HelpModal.tsx` | Modify | Update supported formats list |

## 7. RAW+JPEG Companion Pairing

```
Scan directory → collect all photos (JPEG + RAW)
        ↓
PairRAWJPEG() → group by base filename in same directory
  IMG_0042.CR3 + IMG_0042.JPG → pair (RAW=primary, JPEG=companion)
  IMG_0043.CR3 alone → single (normal dedup flow)
  IMG_0044.JPG alone → single (normal flow)
        ↓
For pairs:
  - RAW is always the keeper (more data)
  - JPEG companion marked as IsRAWCompanion=true, left in place (NOT moved to duplicates/)
  - User decides what to do with companions
For singles:
  - Normal dHash + Laplacian Variance flow
```

### Pairing rules:
- Same base name + same directory → pair
- Same base name + different directories → NOT paired
- RAW without JPEG companion → single, normal dedup
- Case-insensitive matching (IMG_0042.cr3 == IMG_0042.JPG)
- JPEG companions are NOT moved to duplicates/ folder — they stay in place with the `IsRAWCompanion` flag

### Export behavior for pairs:
- When user exports a RAW file, only the RAW file is exported
- The JPEG companion is not auto-exported (user can select it independently if desired)
- This keeps export behavior simple and predictable

## 8. Model Changes

```go
type Photo struct {
    // existing fields...

    IsRAW          bool   `json:"isRAW"`
    RAWFormat      string `json:"rawFormat"`      // "CR3", "ARW", "NEF", etc.
    CompanionPath  string `json:"companionPath"`  // path to RAW+JPEG pair companion
    IsRAWCompanion bool   `json:"isRAWCompanion"` // true if this JPEG has a RAW companion
}
```

## 9. TIFF IFD Parser Algorithm

```
1. Stat file to get fileSize (for bounds validation)

2. Read 8-byte TIFF header:
   - Bytes 0-1: byte order ("II"=little-endian, "MM"=big-endian)
   - Bytes 2-3: magic number (42 for classic TIFF, 43 for BigTIFF)
   - If magic=43: return error "BigTIFF not supported" (known limitation, rare in practice)
   - Bytes 4-7: offset to IFD0

3. Walk IFD chain starting at IFD0:
   - Maintain visited-offsets set to detect circular IFD chains
   - Hard cap: max 20 IFDs to prevent runaway parsing on malformed files
   - Read 2-byte entry count
   - For each 12-byte entry: tag ID, type, count, value/offset
   - Read 4-byte next-IFD offset (0 = end of chain)
   - If next-IFD offset already in visited set → break (circular chain)

4. For each IFD, collect:
   - Compression (tag 0x0103): if 6 or 7, it's JPEG
   - StripOffsets (tag 0x0111) + StripByteCounts (tag 0x0117)
   - OR: JPEGInterchangeFormat (0x0201) + JPEGInterchangeFormatLength (0x0202)
   - NewSubFileType (tag 0x00FE): value 1 = preview image
   - SubIFDs (tag 0x014a): recurse into child IFDs (same visited set + depth cap)

5. Validate all offsets: offset + byteCount <= fileSize

6. Select the largest JPEG preview found across all IFDs

7. Seek to offset, read bytes, validate JPEG SOI marker (0xFFD8)
```

### Edge cases handled:
- Byte order (big/little endian)
- Value-in-offset-field (count * type_size <= 4)
- Multiple strips (array StripOffsets)
- Tag value types (SHORT=2 bytes vs LONG=4 bytes)
- SubIFD arrays (multiple child IFDs)
- OJPEG (Compression=6) vs JPEG (Compression=7)
- Both StripOffsets and JPEGInterchangeFormat paths
- JPEG SOI validation before returning data
- **Circular IFD chain detection** via visited-offsets set
- **Offset bounds validation** against file size
- **BigTIFF detection** (magic=43) with graceful rejection

## 10. `ExtractPreview` Return Type

```go
// ExtractPreview returns the embedded JPEG preview bytes from a RAW file.
// Returns raw JPEG bytes (not decoded image.Image) for efficiency:
// - Media server can serve bytes directly without decode→re-encode round trip
// - Thumbnail pipeline decodes from bytes only when needed for resize
// - Dedup pipeline decodes from bytes for hashing
// - Avoids lossy JPEG re-encoding for full-res Viewer previews
func ExtractPreview(path string) ([]byte, error)
```

Consumers that need `image.Image` call `jpeg.Decode(bytes.NewReader(jpegBytes))` after extraction.

## 11. Memory Efficiency

- TIFF IFD parser reads ~500 bytes of headers/tags per IFD
- Seeks directly to JPEG preview offset, reads only preview bytes
- Never loads full RAW sensor data (25-80 MB per file)
- For burst of 10 files (~700 MB total), reads ~10 MB of preview data
- imagemeta reads only EXIF/preview blocks for CR3

## 12. Updated Dedup Flow

```
1. Scan directory → collect all photos (JPEG + PNG + RAW)
2. PairRAWJPEG() → identify companion pairs by base filename
3. For each pair: mark JPEG as companion (left in place), RAW as primary
4. For unpaired files (singles and RAW-only):
   a. Extract embedded JPEG preview bytes (Path A → Path B fallback)
   b. Decode JPEG bytes to image.Image
   c. Compute dHash on image.Image
   d. Group by Hamming distance (existing logic, unchanged)
   e. Within each group: rank by Laplacian Variance
   f. Surface highest-scoring as keeper
5. Present paired groups and duplicate groups to UI
```

### Laplacian Variance on RAW previews

The embedded JPEG preview has camera-applied sharpening/NR/tone mapping. Laplacian Variance measures the camera's JPEG rendering, not raw sensor sharpness. This is acceptable because within a burst group (same camera, same session = same processing pipeline), scores are comparable as relative rankings.

## 13. EXIF Handling for RAW Files

### EXIF Extraction Strategy

| Format | EXIF Source | Method |
|--------|-----------|--------|
| CR2, NEF, ARW, DNG | RAW file's TIFF IFD0 | `rwcarlsen/goexif` `exif.Decode()` works directly on file stream (these are TIFF-based) |
| CR3 | BMFF container | Use `imagemeta` library to extract EXIF from BMFF structure |
| RAF, RW2, ORF | Embedded JPEG preview | Extract preview bytes, then `exif.Decode()` on the JPEG preview |

### Orientation Handling

1. Extract JPEG preview bytes from RAW
2. Read EXIF Orientation from the preview JPEG (or parent RAW's IFD0 for TIFF-based formats)
3. Apply orientation transform after decoding
4. Then resize for thumbnail cache

### Files affected:
- `internal/dedupe/sorter.go` — `ExtractDateTaken()` must handle CR3/RAF/RW2/ORF via preview JPEG EXIF
- `internal/app/app.go` — `GetPhotoEXIF()` must return EXIF data for all RAW formats
- `internal/app/app.go` — `ExtractFullEXIF()` must route RAW files through appropriate EXIF extraction path

## 14. Media Server — RAW Preview Serving

The media server at `127.0.0.1:34342` must serve extracted RAW previews to the frontend Viewer.

**Current pattern:** For JPEG/PNG, the server reads the file and serves it directly with the appropriate Content-Type.

**RAW integration:** When the requested file has a RAW extension:
1. Check preview cache first (see caching below)
2. If not cached: extract JPEG preview bytes via `raw.ExtractPreview()`
3. Write to cache file
4. Serve with `Content-Type: image/jpeg`

### Full-Resolution Preview Cache

The existing `ThumbCache` generates 300px thumbnails for the Grid. The Viewer needs full-resolution previews (3000px+), which are a different asset.

- **Cache location:** `~/.cullsnap/previews/` (separate from `~/.cullsnap/thumbs/`)
- **Cache key:** Same scheme as thumbnails — MD5(path + modTime) + `.jpg`
- **No eviction during session:** Previews are session-scoped. Cache is cleared on next scan of a different directory, matching how thumbnails work.
- **Size estimate:** ~5MB per preview × 500 RAW files = ~2.5GB max. Acceptable for a session working with a single shoot.

## 15. UI — RAW Format Badges

### Grid Thumbnails — RAW Format Badge

Add a format badge to RAW file thumbnails in the grid:

```
┌──────────────┐
│           [✓]│  ← selected badge (top-right, existing)
│              │
│              │
│[ARW]         │  ← RAW format badge (bottom-left, new)
└──────────────┘
```

- **Position:** Bottom-left corner of the thumbnail card
- **Content:** Dynamic format text from `photo.RAWFormat` — shows the actual format: "CR3", "ARW", "NEF", "DNG", "RAF", "RW2", "ORF", "NRW", "PEF", "SRW"
- **Style:** `.badge-raw` — warm amber/orange background (`#D97706` / amber-600) to visually distinguish from selected (accent blue) and exported (green) badges. White text, rounded pill shape, ~10px font.
- **Size:** Auto-width pill shape to accommodate varying format name lengths

### Viewer — RAW Format Overlay Badge

Add a format indicator overlay on the Viewer image for RAW files:

- **Position:** Top-left corner of the image container
- **Content:** Format name from `photo.RAWFormat` (e.g., "NEF", "ARW", "DNG")
- **For paired files:** Show format + " + JPG" (e.g., "CR3 + JPG") to indicate the companion relationship
- **Style:** Semi-transparent glass-panel pill, consistent with existing UI aesthetic
- **Visibility:** Only shown when `photo.IsRAW` is true

### RAW+JPEG Companion Indicator

When a JPEG is paired with a RAW companion:
- Grid badge shows nothing extra on the JPEG (the RAW file's badge is sufficient)
- Viewer shows companion info in the EXIF bar or as a subtle indicator

## 16. Testing Strategy

### Test Fixtures — Real Camera RAW Samples

Download real-world RAW sample files from public sources for comprehensive testing:
- **Source:** raw.pixls.us, raw.samples archive, camera manufacturer sample galleries
- **One file per format minimum:** CR2, CR3, ARW, NEF, DNG, RAF, RW2, ORF
- **Storage:** `internal/raw/testdata/` directory, gitignored via `.gitignore` entry
- **Download script:** `internal/raw/testdata/download.sh` that fetches samples from known URLs on first test run
- **Validation:** Each test file must have a verifiable embedded JPEG preview

### internal/raw/preview_test.go
- Table-driven tests with real RAW samples per format
- Assert: preview not nil, dimensions >= 200px, no error
- Assert: JPEG SOI marker (0xFFD8) present at start of returned bytes
- Assert: graceful error on corrupt file, truncated file, file with no preview
- Assert: EXIF orientation is correctly read from preview
- Assert: fallback to dcraw when Pure Go path fails
- Assert: returns error (not panic) for zero-length files, non-RAW files

### internal/raw/tiff_test.go
- Edge cases: big-endian/little-endian byte order
- Value-in-offset-field (count * type_size <= 4)
- Multiple strips (array StripOffsets)
- SubIFD arrays (multiple child IFDs)
- Both StripOffsets and JPEGInterchangeFormat paths
- Corrupt TIFF header handling (bad magic, truncated header)
- Circular IFD chain detection (must not infinite loop)
- Offset beyond file size (must return error, not crash)
- BigTIFF detection and graceful rejection

### internal/raw/dcraw_test.go
- Graceful degradation when dcraw binary is absent (returns specific error, not crash)
- Extraction from each dcraw-primary format (RAF, RW2, ORF) when binary available
- Timeout handling: verify 30s timeout kills hung processes
- Error handling for corrupt files
- Proper cleanup of temp files if any

### internal/raw/pair_test.go
- Same base name same dir → paired
- Same base name different dirs → NOT paired
- RAW without JPEG companion → single
- JPEG without RAW companion → single
- Case-insensitive matching (IMG_0042.cr3 == IMG_0042.JPG)
- Multiple RAW formats in same directory
- Mixed case extensions (.CR3, .Cr3, .cr3)
- Multiple JPEG extensions (.jpg, .jpeg) paired with RAW

### Integration Tests
- Full scan → thumbnail → dedup flow with mixed RAW+JPEG test folder
- RAW+JPEG pairing correctly marks companions in dedup output
- JPEG companions are NOT moved to duplicates/ folder
- Media server serves extracted RAW previews with correct Content-Type
- EXIF data is correctly extracted and displayed for all RAW formats
- Grid displays correct dynamic format badge text per photo

## 17. Library Dependencies

| Library | Import Path | Purpose | License |
|---------|-------------|---------|---------|
| imagemeta | github.com/evanoberholster/imagemeta | CR3 preview + format detection | MIT |
| dcraw | Bundled binary | Fallback preview extraction | GPL-2.0 (binary distribution) |

### dcraw License Compliance
dcraw is GPL-2.0. CullSnap is AGPL-3.0 (GPL-compatible). The bundled dcraw binary must be distributed with its GPL license notice. Add dcraw's license to a `THIRD_PARTY_LICENSES` file in the project root.

### Libraries NOT used (with reasons):
- `rwcarlsen/goexif` — already in project for JPEG EXIF, but cannot extract full-res RAW previews (only IFD1 ~160px thumbnail). Still used for EXIF metadata extraction from TIFF-based RAW files.
- `barasher/go-exiftool` — GPL-3.0, requires ExifTool binary (Perl), overkill when dcraw is simpler
- `seppedelanghe/go-libraw` — CGO required, 6 stars, too early stage
- `dcraw` shell-out for core path — development stalled 2018, but fine as fallback

## 18. Cross-Platform Build

- Path A (imagemeta + custom parser): Pure Go, zero CGO, cross-compiles freely
- Path B (dcraw): Bundle platform-specific binaries, same pattern as FFmpeg provisioning
  - macOS: `dcraw` universal binary
  - Windows: `dcraw.exe`
  - Linux: `dcraw` amd64

## 19. Debug Logging

All new code must include comprehensive debug logging so that production issues can be diagnosed without a new release. Use the existing `logger.Log` pattern.

### Required log points:

**`internal/raw/preview.go`:**
- Format detected (path, format name, detection method)
- TIFF header parsed (byte order, magic, IFD0 offset)
- Each IFD visited (IFD index, entry count, compression type, preview size found)
- Preview selected (offset, byte count, dimensions after decode)
- Path A success/failure (path, format, duration, preview dimensions)
- Fallback to Path B triggered (reason: no preview / too small / unsupported format)

**`internal/raw/dcraw.go`:**
- dcraw binary status on Init (found at path / not found / downloading)
- dcraw invocation (path, args, timeout)
- dcraw result (success with byte count / error with stderr output / timeout)

**`internal/raw/pair.go`:**
- Pairing summary (total files, pairs found, unpaired RAW, unpaired JPEG)
- Each pair formed (RAW path + JPEG path)

**`internal/image/thumbnail.go` / `thumbcache.go`:**
- RAW file routed to ExtractPreview (path, format)
- Preview cache hit/miss for full-res previews
- Thumbnail generation duration for RAW files

**`internal/app/app.go`:**
- RAW module initialization result
- EXIF extraction method chosen per file (direct TIFF / imagemeta / preview JPEG)

**Media server (`main.go`):**
- RAW preview request received (path)
- Preview cache hit/miss
- Preview extraction duration

### Logging level:
- Use `Debug` level for per-file operations (won't appear in normal usage)
- Use `Info` level for initialization and summary stats
- Use `Warn` level for fallback triggers and degraded functionality
- Use `Error` level for failures

## 20. Known Limitations

- **BigTIFF:** Some Adobe-converted DNG files use BigTIFF (8-byte offsets, magic=43). The TIFF IFD parser detects this and returns an error. These files fall through to dcraw.
- **dcraw development:** dcraw was last updated in 2018. Very new camera models (post-2018) may not be supported by dcraw. For these, Path A (Pure Go) is the primary path.
- **Lossy preview for dedup:** Perceptual hashing and Laplacian Variance operate on camera-rendered JPEG previews, not raw sensor data. This is acceptable for relative ranking within burst groups but means quality scores reflect camera JPEG processing, not true sensor resolution.
