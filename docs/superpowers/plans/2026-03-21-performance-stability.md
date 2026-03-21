# Dedup Pipeline Performance & Quality Scoring — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Speed up the dedup pipeline from 5+ minutes to ~60-90 seconds on 2,618 files by eliminating redundant I/O and decoding, and improve photo quality scoring with multi-factor analysis (sharpness + exposure + noise + contrast).

**Architecture:** Replace original-file reads in hashing and quality scoring with cached 300px thumbnail reads from local SSD. Remove the wasteful 256x256 pre-resize in hashing (dHash resizes to 9x8 internally). Parallelize quality scoring. Add Gaussian pre-blur for noise-robust sharpness, plus histogram-based exposure/noise/contrast scoring with weighted combination.

**Tech Stack:** Go 1.25, existing goimagehash + imaging libraries, no new dependencies.

**Spec:** `docs/superpowers/specs/2026-03-21-performance-stability-design.md`

---

### Task 1: Remove Redundant 256x256 Pre-Resize in Hashing

**Files:**
- Modify: `internal/dedupe/hash.go:68-70`

The `goimagehash.DifferenceHash()` internally resizes to 9x8 pixels. The current 256x256 intermediate resize is pure overhead.

- [ ] **Step 1: Remove the resize line**

In `internal/dedupe/hash.go`, delete lines 68-70:
```go
	// Downscale heavily before hashing to save matrix overhead inside hashing func
	// a simple 256x256 is plenty for perceptual hashes.
	thumb := imaging.Resize(img, 256, 0, imaging.NearestNeighbor)
```

And change line 72 from:
```go
	hash, err := goimagehash.DifferenceHash(thumb)
```
to:
```go
	hash, err := goimagehash.DifferenceHash(img)
```

- [ ] **Step 2: Remove unused imaging import if no longer needed**

Check if `imaging` is still imported in hash.go. It was only used for the resize — remove it if unused.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/dedupe/ -v -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/dedupe/hash.go
git commit -m "perf: remove redundant 256x256 pre-resize in dHash (library resizes to 9x8 internally)"
```

---

### Task 2: Use Cached Thumbnails for Hashing

**Files:**
- Modify: `internal/dedupe/hash.go:36-77` (hashImage function)
- Modify: `internal/dedupe/hash.go:82-204` (FindDuplicates — pass thumbnailDir)
- Modify: `internal/app/app.go:777` (pass thumbnailDir to FindDuplicates)

- [ ] **Step 1: Add thumbnailDir parameter to FindDuplicates**

Change the signature of `FindDuplicates` from:
```go
func FindDuplicates(ctx context.Context, dirPath string, similarityThreshold int, progressCallback func(current, total int, message string)) ([]*DuplicateGroup, error) {
```
to:
```go
func FindDuplicates(ctx context.Context, dirPath string, similarityThreshold int, thumbnailDir string, progressCallback func(current, total int, message string)) ([]*DuplicateGroup, error) {
```

- [ ] **Step 2: Add hashImageFromThumbnail function**

Add a new function that loads from the thumbnail cache:
```go
// hashImageFromThumbnail computes dHash from a cached 300px thumbnail (fast, local SSD read).
// Falls back to hashImage() if thumbnail not found.
func hashImageFromThumbnail(originalPath string, thumbnailDir string) (*goimagehash.ImageHash, error) {
	if thumbnailDir == "" {
		return hashImage(originalPath)
	}

	// Compute thumbnail cache path (same logic as ThumbCache.cacheKey)
	info, err := os.Stat(originalPath)
	if err != nil {
		return hashImage(originalPath)
	}
	h := md5.Sum([]byte(fmt.Sprintf("%s_%d", originalPath, info.ModTime().UnixNano())))
	thumbPath := filepath.Join(thumbnailDir, fmt.Sprintf("%x.jpg", h))

	// Try loading cached thumbnail
	thumbFile, err := os.Open(thumbPath)
	if err != nil {
		// Cache miss — fall back to original file
		return hashImage(originalPath)
	}
	defer func() { _ = thumbFile.Close() }()

	img, err := jpeg.Decode(thumbFile)
	if err != nil {
		return hashImage(originalPath)
	}

	// dHash directly on thumbnail — no pre-resize needed
	hash, err := goimagehash.DifferenceHash(img)
	if err != nil {
		return nil, fmt.Errorf("failed to compute dHash for %s: %w", originalPath, err)
	}

	return hash, nil
}
```

Add imports: `"crypto/md5"`, `"image/jpeg"`

- [ ] **Step 3: Update FindDuplicates to use thumbnails**

In the goroutine at line 121, change:
```go
hash, err := hashImage(path)
```
to:
```go
hash, err := hashImageFromThumbnail(path, thumbnailDir)
```

- [ ] **Step 4: Update app.go caller**

In `internal/app/app.go:777`, change:
```go
groups, err := dedupe.FindDuplicates(appCtx, path, similarityThreshold, emitProgress)
```
to:
```go
groups, err := dedupe.FindDuplicates(appCtx, path, similarityThreshold, a.thumbCache.CacheDir(), emitProgress)
```

Also add a `CacheDir()` accessor to ThumbCache if it doesn't exist:
```go
// In internal/image/thumbcache.go
func (tc *ThumbCache) CacheDir() string {
	return tc.cacheDir
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/... -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/dedupe/hash.go internal/app/app.go internal/image/thumbcache.go
git commit -m "perf: hash from cached thumbnails instead of re-reading originals from disk"
```

---

### Task 3: Use Cached Thumbnails for Quality Scoring

**Files:**
- Modify: `internal/dedupe/quality.go:21-51` (CalculateLaplacianVariance)
- Modify: `internal/dedupe/quality.go:119-177` (FindBestPhotos — pass thumbnailDir)
- Modify: `internal/app/app.go:783` (pass thumbnailDir to FindBestPhotos)

- [ ] **Step 1: Add thumbnailDir parameter to FindBestPhotos and CalculateLaplacianVariance**

Change `CalculateLaplacianVariance` signature to:
```go
func CalculateLaplacianVariance(imgPath string, thumbnailDir string) (float64, error) {
```

Change `FindBestPhotos` signature to:
```go
func FindBestPhotos(ctx context.Context, groups []*DuplicateGroup, thumbnailDir string, progressCallback func(current, total int, message string)) error {
```

- [ ] **Step 2: Add thumbnail loading to CalculateLaplacianVariance**

Replace the current image loading block (lines 22-51) with:
```go
func CalculateLaplacianVariance(imgPath string, thumbnailDir string) (variance float64, err error) {
	var img image.Image

	// Try cached thumbnail first (local SSD, fast)
	if thumbnailDir != "" {
		info, statErr := os.Stat(imgPath)
		if statErr == nil {
			h := md5.Sum([]byte(fmt.Sprintf("%s_%d", imgPath, info.ModTime().UnixNano())))
			thumbPath := filepath.Join(thumbnailDir, fmt.Sprintf("%x.jpg", h))
			if thumbFile, openErr := os.Open(thumbPath); openErr == nil {
				defer func() { _ = thumbFile.Close() }()
				if decoded, decErr := jpeg.Decode(thumbFile); decErr == nil {
					img = decoded
				}
			}
		}
	}

	// Fallback: read original file
	if img == nil {
		ext := strings.ToLower(filepath.Ext(imgPath))
		if raw.IsRAWExt(ext) {
			previewBytes, extractErr := raw.ExtractPreview(imgPath)
			if extractErr != nil {
				return 0, fmt.Errorf("RAW preview extraction failed: %w", extractErr)
			}
			decoded, decErr := jpeg.Decode(bytes.NewReader(previewBytes))
			if decErr != nil {
				return 0, fmt.Errorf("failed to decode RAW preview: %w", decErr)
			}
			img = decoded
		} else {
			file, openErr := os.Open(imgPath)
			if openErr != nil {
				return 0, fmt.Errorf("failed to open image: %w", openErr)
			}
			defer func() {
				if cerr := file.Close(); cerr != nil && err == nil {
					err = cerr
				}
			}()
			decoded, _, decErr := image.Decode(file)
			if decErr != nil {
				return 0, fmt.Errorf("failed to decode image: %w", decErr)
			}
			img = decoded
		}
	}

	// Rest of function unchanged (resize to 500 → grayscale → laplacian)
	// But when loaded from 300px thumbnail, the resize to 500 is a no-op upscale.
	// Change resize target to 300 to match thumbnail size:
```

Change the resize from `500` to `300` since thumbnails are already 300px:
```go
	thumb := imaging.Resize(img, 300, 0, imaging.NearestNeighbor)
```

Add import: `"crypto/md5"`

- [ ] **Step 3: Pass thumbnailDir through FindBestPhotos**

In `FindBestPhotos`, pass `thumbnailDir` to `CalculateLaplacianVariance`:
```go
variance, err := CalculateLaplacianVariance(photo.Path, thumbnailDir)
```

- [ ] **Step 4: Update app.go caller**

In `internal/app/app.go:783`, change:
```go
err = dedupe.FindBestPhotos(appCtx, groups, emitProgress)
```
to:
```go
err = dedupe.FindBestPhotos(appCtx, groups, a.thumbCache.CacheDir(), emitProgress)
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/... -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/dedupe/quality.go internal/app/app.go
git commit -m "perf: use cached thumbnails for quality scoring, resize target 300px"
```

---

### Task 4: Parallelize Quality Scoring

**Files:**
- Modify: `internal/dedupe/quality.go:119-177` (FindBestPhotos)

- [ ] **Step 1: Rewrite FindBestPhotos with errgroup parallelism**

Replace the sequential loop with parallel processing across groups:

```go
func FindBestPhotos(ctx context.Context, groups []*DuplicateGroup, thumbnailDir string, progressCallback func(current, total int, message string)) error {
	totalGroups := len(groups)
	var processedCount int32

	g := new(errgroup.Group)
	g.SetLimit(runtime.NumCPU())

	for _, group := range groups {
		group := group // capture for goroutine

		if len(group.Photos) <= 1 {
			if len(group.Photos) == 1 {
				group.Photos[0].IsUnique = true
			}
			atomic.AddInt32(&processedCount, 1)
			if progressCallback != nil {
				progressCallback(int(atomic.LoadInt32(&processedCount)), totalGroups, "Selecting best quality photos...")
			}
			continue
		}

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			var bestPhoto *PhotoInfo
			var maxVariance float64 = -1

			for _, photo := range group.Photos {
				if photo.Hash == nil {
					continue
				}

				variance, err := CalculateLaplacianVariance(photo.Path, thumbnailDir)
				if err != nil {
					continue
				}

				if variance > maxVariance {
					maxVariance = variance
					bestPhoto = photo
				}
			}

			for _, photo := range group.Photos {
				if photo == bestPhoto && bestPhoto != nil {
					photo.IsUnique = true
				} else {
					photo.IsUnique = false
				}
			}

			if bestPhoto == nil && len(group.Photos) > 0 {
				group.Photos[0].IsUnique = true
				for i := 1; i < len(group.Photos); i++ {
					group.Photos[i].IsUnique = false
				}
			}

			count := atomic.AddInt32(&processedCount, 1)
			if progressCallback != nil {
				progressCallback(int(count), totalGroups, "Selecting best quality photos...")
			}
			return nil
		})
	}

	return g.Wait()
}
```

Add imports: `"runtime"`, `"sync/atomic"`, `"golang.org/x/sync/errgroup"`

- [ ] **Step 2: Run tests**

Run: `go test ./internal/dedupe/ -v -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/dedupe/quality.go
git commit -m "perf: parallelize quality scoring across CPU cores via errgroup"
```

---

### Task 5: Multi-Factor Quality Scoring

**Files:**
- Create: `internal/dedupe/histogram.go`
- Create: `internal/dedupe/histogram_test.go`
- Modify: `internal/dedupe/quality.go` (add Gaussian pre-blur, integrate multi-factor scoring)

- [ ] **Step 1: Create histogram.go with exposure + noise + contrast metrics**

```go
package dedupe

import (
	"image"
	"image/color"
	"math"
)

// ImageMetrics holds histogram-derived quality metrics for an image.
type ImageMetrics struct {
	HighlightClip float64 // fraction of pixels with luminance >= 250
	ShadowClip    float64 // fraction of pixels with luminance <= 5
	MeanLuminance float64 // average luminance 0-255
	RMSContrast   float64 // std_dev(luminance) / mean(luminance)
	NoiseSigma    float64 // estimated noise level (Immerkaer method)
}

// ComputeHistogramMetrics computes exposure, contrast, and noise metrics in a single pass.
func ComputeHistogramMetrics(img image.Image) ImageMetrics {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	totalPixels := float64(width * height)

	if totalPixels == 0 {
		return ImageMetrics{}
	}

	var sumLum, sumLumSq float64
	var highlightCount, shadowCount float64

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			lum := luminance(img.At(x, y))
			sumLum += lum
			sumLumSq += lum * lum
			if lum >= 250 {
				highlightCount++
			}
			if lum <= 5 {
				shadowCount++
			}
		}
	}

	meanLum := sumLum / totalPixels
	variance := (sumLumSq / totalPixels) - (meanLum * meanLum)
	stdDev := math.Sqrt(math.Max(0, variance))

	rmsContrast := 0.0
	if meanLum > 0 {
		rmsContrast = stdDev / meanLum
	}

	return ImageMetrics{
		HighlightClip: highlightCount / totalPixels,
		ShadowClip:    shadowCount / totalPixels,
		MeanLuminance: meanLum,
		RMSContrast:   rmsContrast,
	}
}

// EstimateNoise computes noise sigma using the Immerkaer method.
// Uses a specialized 3x3 kernel designed for noise estimation.
func EstimateNoise(img image.Image) float64 {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width < 3 || height < 3 {
		return 0
	}

	// Immerkaer noise estimation kernel:
	// [  1 -2  1 ]
	// [ -2  4 -2 ]
	// [  1 -2  1 ]
	var sum float64
	for y := bounds.Min.Y + 1; y < bounds.Max.Y-1; y++ {
		for x := bounds.Min.X + 1; x < bounds.Max.X-1; x++ {
			val := luminance(img.At(x-1, y-1)) - 2*luminance(img.At(x, y-1)) + luminance(img.At(x+1, y-1)) -
				2*luminance(img.At(x-1, y)) + 4*luminance(img.At(x, y)) - 2*luminance(img.At(x+1, y)) +
				luminance(img.At(x-1, y+1)) - 2*luminance(img.At(x, y+1)) + luminance(img.At(x+1, y+1))
			sum += math.Abs(val)
		}
	}

	// Immerkaer formula
	sigma := sum * math.Sqrt(math.Pi/2.0) / (6.0 * float64(width-2) * float64(height-2))
	return sigma
}

func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
}

// ExposureScore computes a 0-1 score penalizing clipped highlights/shadows.
// groupMedianLuminance should be the median mean-luminance across the burst group.
func ExposureScore(m ImageMetrics, groupMedianLuminance float64) float64 {
	balance := math.Abs(m.MeanLuminance-groupMedianLuminance) / 128.0
	score := 1.0 - (3.0 * m.HighlightClip) - (1.0 * m.ShadowClip) - (0.5 * balance)
	return math.Max(0, math.Min(1, score))
}
```

- [ ] **Step 2: Write histogram tests**

```go
// internal/dedupe/histogram_test.go
package dedupe

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestComputeHistogramMetrics_NormalImage(t *testing.T) {
	// Create a 10x10 image with mid-range luminance
	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.NRGBA{R: 128, G: 128, B: 128, A: 255})
		}
	}

	m := ComputeHistogramMetrics(img)
	if m.HighlightClip > 0 {
		t.Errorf("expected no highlight clipping, got %f", m.HighlightClip)
	}
	if m.ShadowClip > 0 {
		t.Errorf("expected no shadow clipping, got %f", m.ShadowClip)
	}
	if math.Abs(m.MeanLuminance-128) > 1 {
		t.Errorf("expected mean ~128, got %f", m.MeanLuminance)
	}
}

func TestComputeHistogramMetrics_Overexposed(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	m := ComputeHistogramMetrics(img)
	if m.HighlightClip < 0.9 {
		t.Errorf("expected high highlight clipping, got %f", m.HighlightClip)
	}
}

func TestEstimateNoise_UniformImage(t *testing.T) {
	// Uniform image should have near-zero noise
	img := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			img.Set(x, y, color.NRGBA{R: 100, G: 100, B: 100, A: 255})
		}
	}

	sigma := EstimateNoise(img)
	if sigma > 1 {
		t.Errorf("expected low noise for uniform image, got %f", sigma)
	}
}

func TestExposureScore(t *testing.T) {
	// Well-exposed image at group median
	m := ImageMetrics{HighlightClip: 0, ShadowClip: 0, MeanLuminance: 128}
	score := ExposureScore(m, 128)
	if score < 0.9 {
		t.Errorf("expected high score for well-exposed image, got %f", score)
	}

	// Overexposed
	m2 := ImageMetrics{HighlightClip: 0.1, ShadowClip: 0, MeanLuminance: 200}
	score2 := ExposureScore(m2, 128)
	if score2 >= score {
		t.Errorf("overexposed should score lower than well-exposed")
	}
}
```

- [ ] **Step 3: Run histogram tests**

Run: `go test ./internal/dedupe/ -v -run 'TestComputeHistogram|TestEstimateNoise|TestExposureScore'`
Expected: PASS

- [ ] **Step 4: Add Gaussian pre-blur to CalculateLaplacianVariance**

In `quality.go`, after the resize step and before the grayscale conversion, add a Gaussian blur:

```go
	thumb := imaging.Resize(img, 300, 0, imaging.NearestNeighbor)

	// Gaussian pre-blur to suppress noise (Laplacian of Gaussian)
	thumb = imaging.Blur(thumb, 1.0)

	grayImg := imaging.Grayscale(thumb)
```

- [ ] **Step 5: Create ComputeQualityScore that combines all metrics**

Add to `quality.go`:
```go
// ComputeQualityScore computes a weighted multi-factor quality score.
// All component scores are raw values — normalization happens at the group level in FindBestPhotos.
type QualityResult struct {
	Sharpness float64
	Exposure  ImageMetrics
	Noise     float64
}

func ComputeQualityScore(imgPath string, thumbnailDir string) (QualityResult, error) {
	// Load image (thumbnail or original)
	img, err := loadImageForScoring(imgPath, thumbnailDir)
	if err != nil {
		return QualityResult{}, err
	}

	thumb := imaging.Resize(img, 300, 0, imaging.NearestNeighbor)

	// Sharpness: Laplacian of Gaussian variance
	blurred := imaging.Blur(thumb, 1.0)
	grayImg := imaging.Grayscale(blurred)
	sharpness := computeLaplacianVariance(grayImg)

	// Histogram metrics (exposure + contrast)
	exposure := ComputeHistogramMetrics(thumb)

	// Noise estimation
	noise := EstimateNoise(imaging.Grayscale(thumb))

	return QualityResult{
		Sharpness: sharpness,
		Exposure:  exposure,
		Noise:     noise,
	}, nil
}
```

Extract the Laplacian kernel computation into a helper `computeLaplacianVariance(grayImg)` so it can be reused.

Extract image loading into `loadImageForScoring(imgPath, thumbnailDir) (image.Image, error)` to share between hash and quality.

- [ ] **Step 6: Update FindBestPhotos to use multi-factor scoring**

Replace the single `CalculateLaplacianVariance` call with `ComputeQualityScore`, then normalize within the group and apply weights:

```go
const (
	weightSharpness = 0.50
	weightExposure  = 0.25
	weightNoise     = 0.15
	weightContrast  = 0.10
)
```

Within each group:
1. Compute `QualityResult` for each photo
2. Find min/max for each metric across the group
3. Normalize each metric to 0-1 via `(val - min) / (max - min)`
4. Compute weighted sum
5. Select photo with highest weighted sum as best

- [ ] **Step 7: Run all tests**

Run: `go test ./internal/... -count=1`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/dedupe/histogram.go internal/dedupe/histogram_test.go internal/dedupe/quality.go
git commit -m "feat: multi-factor quality scoring with sharpness, exposure, noise, and contrast"
```

---

### Task 6: Add Debug Logging

**Files:**
- Modify: `internal/dedupe/hash.go` (log thumbnail hit/miss, hash duration)
- Modify: `internal/dedupe/quality.go` (log scoring duration, component scores)

- [ ] **Step 1: Add logging to hashImageFromThumbnail**

```go
logger.Log.Debug("dedupe: hashing from thumbnail", "path", originalPath, "thumbHit", true)
// or on fallback:
logger.Log.Debug("dedupe: hashing from original (cache miss)", "path", originalPath)
```

- [ ] **Step 2: Add logging to FindBestPhotos**

Log total duration and per-group stats:
```go
logger.Log.Info("dedupe: quality scoring complete", "groups", totalGroups, "duration", time.Since(start))
```

- [ ] **Step 3: Run tests and commit**

Run: `go test ./internal/... -count=1`
Expected: PASS

```bash
git add internal/dedupe/hash.go internal/dedupe/quality.go
git commit -m "feat: add debug logging to dedup hashing and quality scoring"
```

---

### Task 7: Update Tests for New Signatures

**Files:**
- Modify: `internal/dedupe/hash_test.go` (if exists — update FindDuplicates calls)
- Modify: `internal/dedupe/quality_test.go` (if exists — update FindBestPhotos calls)
- Modify: `internal/app/app_test.go` (if affected by signature changes)

- [ ] **Step 1: Find and update all callers of changed functions**

Search for all calls to `FindDuplicates` and `FindBestPhotos` across the codebase and update them to pass the new `thumbnailDir` parameter (use `""` in tests to trigger fallback behavior).

- [ ] **Step 2: Run full test suite**

Run: `go test ./internal/... -count=1`
Expected: PASS

- [ ] **Step 3: Run linter**

Run: `go vet ./...`
Expected: Clean

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: update test callers for new dedup function signatures"
```

---

## Summary of Files

### New files (2):
1. `internal/dedupe/histogram.go` — Exposure, noise, contrast metrics
2. `internal/dedupe/histogram_test.go` — Histogram metric tests

### Modified files (4):
1. `internal/dedupe/hash.go` — Remove pre-resize, thumbnail-based hashing, logging
2. `internal/dedupe/quality.go` — Thumbnail loading, Gaussian pre-blur, multi-factor scoring, parallelization
3. `internal/image/thumbcache.go` — CacheDir() accessor
4. `internal/app/app.go` — Pass thumbnailDir to dedup functions
