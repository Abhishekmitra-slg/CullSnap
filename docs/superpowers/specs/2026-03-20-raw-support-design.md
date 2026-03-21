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

- `dcraw -e <file>` extracts embedded JPEG preview from 700+ camera models
- Same bundling pattern as FFmpeg (`internal/video/ffmpeg.go`)
- Triggered when Path A returns no preview, preview too small (<400px), or unsupported format
- Covers: RAF, RW2, ORF, NRW, PEF, SRW, and edge cases in TIFF-based formats
- dcraw is a single ~400KB C binary, compiles on all platforms

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

## 5. Files to Create/Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/raw/preview.go` | New | TIFF IFD parser + CR3 preview via imagemeta. Exports `ExtractPreview(path) (image.Image, error)` |
| `internal/raw/dcraw.go` | New | dcraw binary provisioning + `dcraw -e` fallback |
| `internal/raw/pair.go` | New | `PairRAWJPEG()` companion detection by base filename |
| `internal/scanner/scanner.go` | Modify | Add RAW extensions to walk filter, set IsRAW/RAWFormat |
| `internal/model/photo.go` | Modify | Add IsRAW, RAWFormat, CompanionPath, IsRAWCompanion fields |
| `internal/image/thumbnail.go` | Modify | Route RAW files through raw.ExtractPreview() |
| `internal/image/thumbcache.go` | Modify | Handle RAW format in GenerateThumbnail() |
| `internal/dedupe/hash.go` | Modify | Add RAW extensions, use extracted preview for hashing |
| `internal/dedupe/quality.go` | Modify | Use extracted preview for Laplacian Variance on RAW |
| `internal/app/app.go` | Modify | Wire RAW+JPEG pairing into ScanAndDeduplicate() |
| `main.go` | Modify | Add RAW extensions to media server allowlist, serve extracted previews for RAW files |
| `frontend/src/components/Grid.tsx` | Modify | Add RAW format badge on thumbnails |
| `frontend/src/components/Viewer.tsx` | Modify | Add RAW format badge overlay on viewer |
| `frontend/src/index.css` | Modify | Add `.badge-raw` styles |
| `frontend/src/components/HelpModal.tsx` | Modify | Update supported formats list |

## 6. RAW+JPEG Companion Pairing

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
  - JPEG companion marked as redundant but NOT auto-deleted
  - User decides what to do with companions
For singles:
  - Normal dHash + Laplacian Variance flow
```

### Pairing rules:
- Same base name + same directory → pair
- Same base name + different directories → NOT paired
- RAW without JPEG companion → single, normal dedup
- Case-insensitive matching (IMG_0042.cr3 == IMG_0042.JPG)

## 7. Model Changes

```go
type Photo struct {
    // existing fields...

    IsRAW          bool   `json:"isRAW"`
    RAWFormat      string `json:"rawFormat"`      // "CR3", "ARW", "NEF", etc.
    CompanionPath  string `json:"companionPath"`  // path to RAW+JPEG pair companion
    IsRAWCompanion bool   `json:"isRAWCompanion"` // true if this JPEG has a RAW companion
}
```

## 8. TIFF IFD Parser Algorithm

```
1. Read 8-byte TIFF header:
   - Bytes 0-1: byte order ("II"=little-endian, "MM"=big-endian)
   - Bytes 2-3: magic number (42)
   - Bytes 4-7: offset to IFD0

2. Walk IFD chain starting at IFD0:
   - Read 2-byte entry count
   - For each 12-byte entry: tag ID, type, count, value/offset
   - Read 4-byte next-IFD offset (0 = end of chain)

3. For each IFD, collect:
   - Compression (tag 0x0103): if 6 or 7, it's JPEG
   - StripOffsets (tag 0x0111) + StripByteCounts (tag 0x0117)
   - OR: JPEGInterchangeFormat (0x0201) + JPEGInterchangeFormatLength (0x0202)
   - NewSubFileType (tag 0x00FE): value 1 = preview image
   - SubIFDs (tag 0x014a): recurse into child IFDs

4. Select the largest JPEG preview found across all IFDs

5. Seek to offset, read bytes, validate JPEG SOI marker (0xFFD8)
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

## 9. Memory Efficiency

- TIFF IFD parser reads ~500 bytes of headers/tags per IFD
- Seeks directly to JPEG preview offset, reads only preview bytes
- Never loads full RAW sensor data (25-80 MB per file)
- For burst of 10 files (~700 MB total), reads ~10 MB of preview data
- imagemeta reads only EXIF/preview blocks for CR3

## 10. Updated Dedup Flow

```
1. Scan directory → collect all photos (JPEG + PNG + RAW)
2. PairRAWJPEG() → identify companion pairs by base filename
3. For each pair: mark JPEG as companion, RAW as primary
4. For unpaired files (singles and RAW-only):
   a. Extract embedded JPEG preview (Path A → Path B fallback)
   b. Compute dHash on preview image.Image
   c. Group by Hamming distance (existing logic, unchanged)
   d. Within each group: rank by Laplacian Variance
   e. Surface highest-scoring as keeper
5. Present paired groups and duplicate groups to UI
```

### Laplacian Variance on RAW previews

The embedded JPEG preview has camera-applied sharpening/NR/tone mapping. Laplacian Variance measures the camera's JPEG rendering, not raw sensor sharpness. This is acceptable because within a burst group (same camera, same session = same processing pipeline), scores are comparable as relative rankings.

## 11. EXIF Orientation Handling

1. Extract JPEG preview bytes from RAW
2. Read EXIF Orientation from the preview JPEG (or parent RAW's IFD0)
3. Apply orientation transform using `imaging.Decode()` with AutoOrientation
4. Then resize for thumbnail cache

## 12. Media Server — RAW Preview Serving

The media server at `127.0.0.1:34342` must serve extracted RAW previews to the frontend Viewer.

**Current pattern:** For JPEG/PNG, the server reads the file and serves it directly with the appropriate Content-Type.

**RAW integration:** When the requested file has a RAW extension:
1. Extract JPEG preview via `raw.ExtractPreview()` (returns `image.Image`)
2. Encode to JPEG bytes
3. Serve with `Content-Type: image/jpeg`
4. Cache the extracted preview bytes in memory or on disk to avoid re-extraction on subsequent requests (the thumbnail cache already handles this for grid thumbnails, but the Viewer needs full-resolution previews)

**Performance consideration:** RAW preview extraction takes ~10-50ms per file. For the Viewer (single file at a time), this is imperceptible. For the Grid, thumbnails are pre-cached by `GenerateBatch()`.

## 13. UI — RAW Format Badges

### Grid Thumbnails — RAW Format Badge

Add a format badge to RAW file thumbnails in the grid, using the existing badge pattern:

```
┌──────────────┐
│           [✓]│  ← selected badge (top-right, existing)
│              │
│              │
│[ARW]         │  ← RAW format badge (bottom-left, new)
└──────────────┘
```

- **Position:** Bottom-left corner of the thumbnail card
- **Content:** Dynamic format text from `photo.RAWFormat` — shows "CR3", "ARW", "NEF", "DNG", "RAF", etc.
- **Style:** `.badge-raw` — warm amber/orange background to visually distinguish from selected (blue) and exported (green) badges
- **Size:** Auto-width pill shape to accommodate varying format name lengths (e.g., "CR3" vs "DNG")

### Viewer — RAW Format Overlay Badge

Add a format indicator overlay on the Viewer image for RAW files:

- **Position:** Top-left corner of the image container
- **Content:** Format name from `photo.RAWFormat` (e.g., "NEF", "CR3", "ARW")
- **For paired files:** Show "CR3 + JPG" to indicate the companion relationship
- **Style:** Semi-transparent glass-panel pill, consistent with existing UI aesthetic
- **Visibility:** Only shown when `photo.IsRAW` is true

### RAW+JPEG Companion Indicator

When a JPEG is paired with a RAW companion:
- Grid badge shows nothing extra (the RAW file's badge is sufficient)
- Viewer shows companion info in the EXIF bar or as a subtle indicator

## 14. Testing Strategy

### Test Fixtures — Real Camera RAW Samples

Download real-world RAW sample files from public sources for comprehensive testing:
- **Source:** raw.pixls.us, raw.samples archive, camera manufacturer sample galleries
- **One file per format minimum:** CR2, CR3, ARW, NEF, DNG, RAF, RW2, ORF
- **Store in:** `internal/raw/testdata/` (gitignored if >5MB per file, or use small crops)
- **Validation:** Each test file must have a verifiable embedded JPEG preview

### internal/raw/preview_test.go
- Table-driven tests with real RAW samples per format
- Assert: preview not nil, dimensions >= 200px, no error
- Assert: JPEG SOI marker validation works
- Assert: graceful error on corrupt file, truncated file, file with no preview
- Assert: EXIF orientation is correctly read and applied
- Assert: fallback to dcraw when Pure Go path fails

### internal/raw/tiff_test.go
- Edge cases: big-endian/little-endian byte order
- Value-in-offset-field (count * type_size <= 4)
- Multiple strips (array StripOffsets)
- SubIFD arrays (multiple child IFDs)
- Both StripOffsets and JPEGInterchangeFormat paths
- Corrupt TIFF header handling

### internal/raw/dcraw_test.go
- Graceful degradation when dcraw binary is absent
- Extraction from each dcraw-primary format (RAF, RW2, ORF) when binary available
- Timeout handling for hung dcraw processes
- Error handling for corrupt files

### internal/raw/pair_test.go
- Same base name same dir → paired
- Same base name different dirs → NOT paired
- RAW without JPEG companion → single
- JPEG without RAW companion → single
- Case-insensitive matching (IMG_0042.cr3 == IMG_0042.JPG)
- Multiple RAW formats in same directory
- Mixed case extensions

### Integration Tests
- Full scan → thumbnail → dedup flow with mixed RAW+JPEG test folder
- RAW+JPEG pairing correctly marks companions in dedup output
- Media server serves extracted RAW previews correctly
- Grid displays RAW format badges correctly

## 15. Library Dependencies

| Library | Import Path | Purpose | License |
|---------|-------------|---------|---------|
| imagemeta | github.com/evanoberholster/imagemeta | CR3 preview + format detection | MIT |
| dcraw | Bundled binary | Fallback preview extraction | GPL (binary only) |

### Libraries NOT used (with reasons):
- `rwcarlsen/goexif` — already in project for JPEG EXIF, but cannot extract full-res RAW previews (only IFD1 ~160px thumbnail)
- `barasher/go-exiftool` — GPL-3.0, requires ExifTool binary (Perl), overkill when dcraw is simpler
- `seppedelanghe/go-libraw` — CGO required, 6 stars, too early stage
- `dcraw` shell-out for core path — development stalled 2018, but fine as fallback

## 16. Cross-Platform Build

- Path A (imagemeta + custom parser): Pure Go, zero CGO, cross-compiles freely
- Path B (dcraw): Bundle platform-specific binaries, same pattern as FFmpeg provisioning
  - macOS: `dcraw` universal binary
  - Windows: `dcraw.exe`
  - Linux: `dcraw` amd64
