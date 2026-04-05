package scoring

import (
	"context"
	"cullsnap/internal/logger"
	"sync"
)

// Engine orchestrates AI scoring across registered providers.
// It tries providers in registration order, using the first available one.
type Engine struct {
	mu        sync.RWMutex
	providers []ScoringProvider
}

// NewEngine creates a new scoring engine with no providers.
func NewEngine() *Engine {
	return &Engine{}
}

// Register adds a scoring provider. Providers are tried in registration order.
func (e *Engine) Register(p ScoringProvider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.providers = append(e.providers, p)
	logger.Log.Info("scoring: registered provider", "name", p.Name())
}

// Enabled reports whether at least one provider is available.
func (e *Engine) Enabled() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, p := range e.providers {
		if p.Available() {
			return true
		}
	}
	return false
}

// Score runs AI analysis on the image using the first available provider.
// Returns nil, nil if no provider is available (scoring is optional).
func (e *Engine) Score(ctx context.Context, imgData []byte) (*ScoreResult, error) {
	e.mu.RLock()
	providers := make([]ScoringProvider, len(e.providers))
	copy(providers, e.providers)
	e.mu.RUnlock()

	for _, p := range providers {
		if !p.Available() {
			continue
		}

		result, err := p.Score(ctx, imgData)
		if err != nil {
			logger.Log.Warn("scoring: provider failed, trying next",
				"provider", p.Name(),
				"error", err,
			)
			continue
		}

		return result, nil
	}

	return nil, nil
}

// Providers returns the names of all registered providers and their availability.
func (e *Engine) Providers() []ProviderStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()
	statuses := make([]ProviderStatus, len(e.providers))
	for i, p := range e.providers {
		statuses[i] = ProviderStatus{
			Name:             p.Name(),
			Available:        p.Available(),
			RequiresAPIKey:   p.RequiresAPIKey(),
			RequiresDownload: p.RequiresDownload(),
		}
	}
	return statuses
}

// ProviderStatus describes a registered provider's current state.
type ProviderStatus struct {
	Name             string `json:"name"`
	Available        bool   `json:"available"`
	RequiresAPIKey   bool   `json:"requiresApiKey"`
	RequiresDownload bool   `json:"requiresDownload"`
}
