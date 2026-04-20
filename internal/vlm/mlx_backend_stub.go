//go:build !(darwin && arm64)

package vlm

import (
	"context"
	"errors"
)

var errMLXNotAvailable = errors.New("MLX not available on this platform")

// MLXBackend is a no-op stub on non-Apple-Silicon platforms.
type MLXBackend struct{}

// NewMLXBackend returns a stub MLXBackend; arguments are intentionally ignored.
func NewMLXBackend(_, _ string, _ ModelManifest) *MLXBackend {
	return &MLXBackend{}
}

// Name returns the backend identifier.
func (b *MLXBackend) Name() string { return "mlx" }

// Available always returns false on non-Apple-Silicon platforms.
func (b *MLXBackend) Available() bool { return false }

// Start returns a platform-unavailability error.
func (b *MLXBackend) Start(_ context.Context) error { return errMLXNotAvailable }

// Stop is a no-op on stub platforms.
func (b *MLXBackend) Stop(_ context.Context) error { return nil }

// Health returns a platform-unavailability error.
func (b *MLXBackend) Health(_ context.Context) error { return errMLXNotAvailable }

// ModelInfo returns a minimal ModelInfo with the mlx backend tag.
func (b *MLXBackend) ModelInfo() ModelInfo { return ModelInfo{Backend: "mlx"} }

// ScorePhoto returns a platform-unavailability error.
func (b *MLXBackend) ScorePhoto(_ context.Context, _ ScoreRequest) (*VLMScore, error) {
	return nil, errors.New("MLX not available")
}

// RankPhotos returns a platform-unavailability error.
func (b *MLXBackend) RankPhotos(_ context.Context, _ RankRequest) (*RankingResult, error) {
	return nil, errors.New("MLX not available")
}
