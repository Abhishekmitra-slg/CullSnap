package scoring

import (
	"image"
)

// 3x3 Laplacian kernel for edge detection.
// This is the standard discrete Laplacian operator.
var laplacianKernel = [3][3]float64{
	{0, 1, 0},
	{1, -4, 1},
	{0, 1, 0},
}

// LaplacianVariance computes the variance of the Laplacian-filtered image region.
// Higher values indicate sharper/more in-focus areas. Returns 0 for regions
// smaller than the 3x3 kernel.
func LaplacianVariance(gray *image.Gray, region image.Rectangle) float64 {
	// Need at least 3x3 for the kernel.
	if region.Dx() < 3 || region.Dy() < 3 {
		return 0
	}

	// Clamp region to image bounds.
	region = region.Intersect(gray.Bounds())
	if region.Empty() {
		return 0
	}

	// Convolution produces output for pixels with full kernel coverage.
	outW := region.Dx() - 2
	outH := region.Dy() - 2
	if outW <= 0 || outH <= 0 {
		return 0
	}

	n := outW * outH
	var sum, sumSq float64

	for y := range outH {
		for x := range outW {
			// Apply 3x3 Laplacian kernel centered at (region.Min.X+x+1, region.Min.Y+y+1).
			var val float64
			for ky := range 3 {
				for kx := range 3 {
					px := region.Min.X + x + kx
					py := region.Min.Y + y + ky
					val += float64(gray.GrayAt(px, py).Y) * laplacianKernel[ky][kx]
				}
			}
			sum += val
			sumSq += val * val
		}
	}

	// Variance = E[X²] - E[X]²
	mean := sum / float64(n)
	variance := sumSq/float64(n) - mean*mean

	return variance
}

// EyeSharpnessFromFace computes the Laplacian variance of the eye region within a face.
// The eye region is estimated as the top 40% of the face bounding box,
// which typically contains the forehead and eyes in a frontal face.
func EyeSharpnessFromFace(gray *image.Gray, face FaceRegion) float64 {
	bb := face.BoundingBox

	// Eye region: top 40% of face height.
	eyeHeight := bb.Dy() * 40 / 100
	eyeRegion := image.Rect(
		bb.Min.X,
		bb.Min.Y,
		bb.Max.X,
		bb.Min.Y+eyeHeight,
	)

	return LaplacianVariance(gray, eyeRegion)
}
