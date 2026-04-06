package scoring

import (
	"cullsnap/internal/logger"
	"fmt"
	"sync"
)

// PluginStatus reports the current state of a registered plugin.
type PluginStatus struct {
	Name      string      `json:"name"`
	Category  string      `json:"category"`
	Available bool        `json:"available"`
	Models    []ModelSpec `json:"models"`
}

// Registry holds registered ScoringPlugin instances keyed by name.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]ScoringPlugin
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]ScoringPlugin),
	}
}

// Register stores p in the registry under its name, replacing any existing
// plugin with the same name.
func (r *Registry) Register(p ScoringPlugin) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.plugins[p.Name()] = p
	logger.Log.Debug("scoring: registry: registered plugin",
		"name", p.Name(),
		"category", p.Category().String(),
	)
}

// Get returns the plugin registered under name, or nil if not found.
func (r *Registry) Get(name string) ScoringPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.plugins[name]
}

// GetByCategory returns all plugins whose Category matches cat.
func (r *Registry) GetByCategory(cat PluginCategory) []ScoringPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []ScoringPlugin
	for _, p := range r.plugins {
		if p.Category() == cat {
			out = append(out, p)
		}
	}
	return out
}

// AllModels aggregates the ModelSpec lists from every registered plugin.
func (r *Registry) AllModels() []ModelSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var models []ModelSpec
	for _, p := range r.plugins {
		models = append(models, p.Models()...)
	}
	return models
}

// Available reports true when at least one Detection plugin AND at least one
// Quality plugin are both Available.
func (r *Registry) Available() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var detOK, qualOK bool
	for _, p := range r.plugins {
		if !p.Available() {
			continue
		}
		switch p.Category() {
		case CategoryDetection:
			detOK = true
		case CategoryQuality:
			qualOK = true
		}
	}
	return detOK && qualOK
}

// InitAll calls Init(libPath) on every registered plugin. Errors from
// Detection or Quality plugins are returned immediately. Errors from
// Recognition plugins are logged and skipped.
func (r *Registry) InitAll(libPath string) error {
	r.mu.RLock()
	plugins := make([]ScoringPlugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		plugins = append(plugins, p)
	}
	r.mu.RUnlock()

	for _, p := range plugins {
		if err := p.Init(libPath); err != nil {
			cat := p.Category()
			if cat == CategoryRecognition {
				logger.Log.Info("scoring: registry: recognition plugin init failed, skipping",
					"name", p.Name(),
					"error", err.Error(),
				)
				continue
			}
			return fmt.Errorf("scoring: registry: plugin %q (category=%s) init failed: %w",
				p.Name(), cat.String(), err)
		}
		logger.Log.Debug("scoring: registry: plugin initialised", "name", p.Name())
	}
	return nil
}

// CloseAll calls Close on every registered plugin and logs any errors.
func (r *Registry) CloseAll() {
	r.mu.RLock()
	plugins := make([]ScoringPlugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		plugins = append(plugins, p)
	}
	r.mu.RUnlock()

	for _, p := range plugins {
		if err := p.Close(); err != nil {
			logger.Log.Info("scoring: registry: plugin close error",
				"name", p.Name(),
				"error", err.Error(),
			)
		} else {
			logger.Log.Debug("scoring: registry: plugin closed", "name", p.Name())
		}
	}
}

// PluginStatuses returns a snapshot of the status of every registered plugin.
func (r *Registry) PluginStatuses() []PluginStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	statuses := make([]PluginStatus, 0, len(r.plugins))
	for _, p := range r.plugins {
		statuses = append(statuses, PluginStatus{
			Name:      p.Name(),
			Category:  p.Category().String(),
			Available: p.Available(),
			Models:    p.Models(),
		})
	}
	return statuses
}
