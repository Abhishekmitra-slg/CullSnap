package app

import (
	"math"
	"testing"
)

// TestMaxInt verifies maxInt returns the larger of two ints.
func TestMaxInt(t *testing.T) {
	if got := maxInt(3, 7); got != 7 {
		t.Errorf("maxInt(3,7) = %d, want 7", got)
	}
	if got := maxInt(7, 3); got != 7 {
		t.Errorf("maxInt(7,3) = %d, want 7", got)
	}
	if got := maxInt(5, 5); got != 5 {
		t.Errorf("maxInt(5,5) = %d, want 5", got)
	}
}

// TestMinInt verifies minInt returns the smaller of two ints.
func TestMinInt(t *testing.T) {
	if got := minInt(3, 7); got != 3 {
		t.Errorf("minInt(3,7) = %d, want 3", got)
	}
	if got := minInt(7, 3); got != 3 {
		t.Errorf("minInt(7,3) = %d, want 3", got)
	}
	if got := minInt(5, 5); got != 5 {
		t.Errorf("minInt(5,5) = %d, want 5", got)
	}
}

// TestWorkerCount verifies workerCount returns a value in [1, 8].
func TestWorkerCount(t *testing.T) {
	n := workerCount()
	if n < 1 || n > 8 {
		t.Errorf("workerCount() = %d, want in [1, 8]", n)
	}
}

// TestFloat32SliceToBytes_RoundTrip verifies that encoding then decoding returns
// the original slice.
func TestFloat32SliceToBytes_RoundTrip(t *testing.T) {
	original := []float32{0.0, 1.0, -1.0, 3.14, math.MaxFloat32}

	encoded := float32SliceToBytes(original)
	if len(encoded) != len(original)*4 {
		t.Fatalf("encoded length = %d, want %d", len(encoded), len(original)*4)
	}

	decoded := bytesToFloat32Slice(encoded)
	if len(decoded) != len(original) {
		t.Fatalf("decoded length = %d, want %d", len(decoded), len(original))
	}

	for i, v := range original {
		if decoded[i] != v {
			t.Errorf("decoded[%d] = %v, want %v", i, decoded[i], v)
		}
	}
}

// TestFloat32SliceToBytes_Empty verifies empty slice produces empty byte slice.
func TestFloat32SliceToBytes_Empty(t *testing.T) {
	if got := float32SliceToBytes(nil); len(got) != 0 {
		t.Errorf("float32SliceToBytes(nil) len = %d, want 0", len(got))
	}
}

// TestBytesToFloat32Slice_BadLength verifies nil is returned for non-multiple-of-4 input.
func TestBytesToFloat32Slice_BadLength(t *testing.T) {
	if got := bytesToFloat32Slice([]byte{1, 2, 3}); got != nil {
		t.Errorf("bytesToFloat32Slice(3 bytes) = %v, want nil", got)
	}
}

// TestBytesToFloat32Slice_Empty verifies empty bytes produce empty float slice.
func TestBytesToFloat32Slice_Empty(t *testing.T) {
	got := bytesToFloat32Slice([]byte{})
	if len(got) != 0 {
		t.Errorf("bytesToFloat32Slice(empty) len = %d, want 0", len(got))
	}
}
