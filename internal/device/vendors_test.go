package device

import (
	"testing"
)

func TestClassifyVendor(t *testing.T) {
	tests := []struct {
		vendorID string
		wantType string
	}{
		{"05ac", "iphone"},
		{"05AC", "iphone"},
		{"04e8", "android"},
		{"18d1", "android"},
		{"22b8", "android"},
		{"2717", "android"},
		{"12d1", "android"},
		{"2a70", "android"},
		{"0bb4", "android"},
		{"1004", "android"},
		{"2ae5", "android"},
		{"29a9", "android"},
		{"0fce", "android"},
		{"22d9", "android"},
		{"2d95", "android"},
		{"19d2", "android"},
		{"17ef", "android"},
		{"0b05", "android"},
		{"04a9", "camera"},
		{"04b0", "camera"},
		{"054c", "camera"},
		{"04cb", "camera"},
		{"07b4", "camera"},
		{"04da", "camera"},
		{"0a17", "camera"},
		{"1199", "camera"},
		{"0000", ""},
		{"ffff", ""},
		{"", ""},
		{"1d6b", ""},
	}
	for _, tt := range tests {
		t.Run(tt.vendorID, func(t *testing.T) {
			got := classifyVendor(tt.vendorID)
			if got != tt.wantType {
				t.Errorf("classifyVendor(%q) = %q, want %q", tt.vendorID, got, tt.wantType)
			}
		})
	}
}
