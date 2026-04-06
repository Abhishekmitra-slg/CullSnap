//go:build !windows

package scoring

import (
	"image"
	"math"
	"testing"
)

// TestAestheticPlugin_Interface verifies AestheticPlugin implements ScoringPlugin.
func TestAestheticPlugin_Interface(t *testing.T) {
	var _ ScoringPlugin = (*AestheticPlugin)(nil)
}

// TestAestheticPlugin_Metadata verifies name, category, and models count.
func TestAestheticPlugin_Metadata(t *testing.T) {
	p := &AestheticPlugin{}

	if got := p.Name(); got != "aesthetic" {
		t.Errorf("Name() = %q, want %q", got, "aesthetic")
	}

	if got := p.Category(); got != CategoryQuality {
		t.Errorf("Category() = %v, want CategoryQuality", got)
	}

	models := p.Models()
	if len(models) != 1 {
		t.Fatalf("Models() returned %d models, want 1", len(models))
	}
	if models[0].Name != aestheticModelName {
		t.Errorf("Models()[0].Name = %q, want %q", models[0].Name, aestheticModelName)
	}
	if models[0].Filename != aestheticModelFile {
		t.Errorf("Models()[0].Filename = %q, want %q", models[0].Filename, aestheticModelFile)
	}
}

// TestSoftmax verifies that softmax produces monotonically increasing probabilities
// for monotonically increasing logits and that the output sums to 1.
func TestSoftmax(t *testing.T) {
	logits := []float32{1, 2, 3}
	probs := softmax(logits)

	if len(probs) != len(logits) {
		t.Fatalf("softmax output length = %d, want %d", len(probs), len(logits))
	}

	// Sum must be approximately 1.
	var sum float64
	for _, p := range probs {
		sum += float64(p)
	}
	if math.Abs(sum-1.0) > 1e-6 {
		t.Errorf("softmax sum = %f, want 1.0", sum)
	}

	// Monotonically increasing logits → monotonically increasing probabilities.
	for i := 1; i < len(probs); i++ {
		if probs[i] <= probs[i-1] {
			t.Errorf("softmax not monotonically increasing: probs[%d]=%f <= probs[%d]=%f",
				i, probs[i], i-1, probs[i-1])
		}
	}
}

// TestSoftmax_LargeValues verifies that softmax does not produce NaN or Inf
// for large logit values (numerical stability check).
func TestSoftmax_LargeValues(t *testing.T) {
	logits := []float32{1000, 1001, 1002}
	probs := softmax(logits)

	for i, p := range probs {
		if math.IsNaN(float64(p)) {
			t.Errorf("probs[%d] is NaN", i)
		}
		if math.IsInf(float64(p), 0) {
			t.Errorf("probs[%d] is Inf", i)
		}
	}

	// Sum must still be approximately 1.
	var sum float64
	for _, p := range probs {
		sum += float64(p)
	}
	if math.Abs(sum-1.0) > 1e-6 {
		t.Errorf("softmax sum = %f, want 1.0 for large values", sum)
	}
}

// TestPreprocessForNIMA_OutputShape verifies that a 300×200 input image produces
// a tensor with exactly 3*224*224 elements.
func TestPreprocessForNIMA_OutputShape(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 300, 200))
	tensor := preprocessForNIMA(img, aestheticInputSize)

	want := 3 * aestheticInputSize * aestheticInputSize
	if len(tensor) != want {
		t.Errorf("tensor length = %d, want %d", len(tensor), want)
	}
}
