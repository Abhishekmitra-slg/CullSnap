# Dedup Pipeline Performance & Quality Scoring — Design Specification

**Date:** 2026-03-21
**Author:** Abhishek Mitra

## 1. Context

CullSnap's dedup pipeline (hash → quality score → sort → relocate) takes 5+ minutes on 2,618 files (1,219 ARW + 1,399 JPG) from an external USB drive. Three root causes identified:

1. **Redundant I/O:** Each file opened 2-3 times from USB (thumbnail, hash, quality)
2. **Wasted computation:** 256x256 pre-resize in hashing is thrown away (dHash resizes to 9x8 internally)
3. **No parallelism in quality scoring:** FindBestPhotos() is single-threaded

Additionally, the current quality scoring (Laplacian Variance only) is noise-sensitive and misses exposure problems.

## 2. Speed Optimization

### Phase 1: Immediate Wins

**A. Remove redundant 256x256 resize in hashImage()**
- `goimagehash.DifferenceHash()` internally resizes to 9x8 pixels
- The 256x256 intermediate step is pure overhead
- Fix: one-line deletion

**B. Parallelize FindBestPhotos()**
- Currently single-threaded, processes groups sequentially
- Fix: `errgroup.Group` with `SetLimit(runtime.NumCPU())`, same pattern as FindDuplicates()

### Phase 2: Eliminate USB I/O from Dedup

**Use cached 300px thumbnails for hashing AND quality scoring.**

The thumbnail cache (`~/.cullsnap/thumbs/`) already has 300px JPEGs for every file by the time dedup runs.

- dHash works on 9x8 pixels — 300px vs 6000px input produces identical hashes
- Laplacian Variance at 300px preserves relative rankings (90,000 pixels, 3x3 kernel)
- Thumbnails are on local SSD (~1ms) vs USB originals (~50-200ms per file)
- Eliminates all USB I/O from dedup entirely

### Phase 3: Optional

- Cache EXIF dates during thumbnail generation
- Producer-consumer pipeline for cold-cache case (sequential USB reader → parallel CPU workers)

### Expected Speedup: 5+ min → ~60-90s (4-5x faster)

## 3. Multi-Factor Quality Scoring

### Current: Laplacian Variance Only
- Noise-sensitive (high-ISO bursts can rank noisier frame higher)
- Misses exposure problems entirely

### Proposed: Weighted Multi-Factor Formula

```
QualityScore = 0.50 * SharpnessScore
             + 0.25 * ExposureScore
             + 0.15 * NoiseScore
             + 0.10 * ContrastScore
```

**SharpnessScore (weight: 0.50):**
- Add Gaussian pre-blur (sigma=1.0) before Laplacian → "Laplacian of Gaussian" (LoG)
- Fixes noise sensitivity at ~2ms additional cost
- Normalize within burst group via min-max

**ExposureScore (weight: 0.25):**
- Single histogram pass: highlight clipping %, shadow clipping %, mean luminance
- `score = 1.0 - (3.0 * highlight_clip) - (1.0 * shadow_clip) - (0.5 * exposure_balance)`
- Highlights penalized 3x (unrecoverable blown highlights)
- ~2ms per image

**NoiseScore (weight: 0.15):**
- Immerkaer noise estimation method
- `sigma = sqrt(pi/2) / (6*(W-2)*(H-2)) * sum(|conv(I, noise_kernel)|)`
- Prevents noisy frames from ranking as "sharp"
- ~3ms per image

**ContrastScore (weight: 0.10):**
- RMS contrast: `std_dev(luminance) / mean(luminance)`
- Tiebreaker signal, derived from same histogram pass as exposure
- ~0.5ms

### Total additional cost: ~8ms per image (well within 100ms budget)

### What NOT to pursue:
- Face/eye detection — requires ML, defer to future phase
- Composition analysis — irrelevant for bursts (framing doesn't change)
- FFT-based sharpness — 8-10x more expensive, marginal benefit
- BRISQUE — too expensive

## 4. Normalization Strategy

All scores normalized **within burst group** using min-max:
```
normalized = (value - group_min) / (group_max - group_min)
```
Handle degenerate case (all values equal) → score = 0.5 for all.

## 5. Files to Modify

| File | Change |
|------|--------|
| `internal/dedupe/hash.go` | Remove 256x256 pre-resize, use thumbnail cache path |
| `internal/dedupe/quality.go` | Add LoG, histogram metrics, noise estimation, weighted scoring, parallelize |
| `internal/dedupe/sorter.go` | Optional: accept pre-cached EXIF dates |
| `internal/app/app.go` | Pass thumbnail cache reference to dedup pipeline |
