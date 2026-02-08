// Package provider defines the LLM provider interface and types.
package provider

import (
	"fmt"
	"sync"
)

// ProviderFactory is a function that creates a Provider with the given model.
type ProviderFactory func(model string) (Provider, error)

// Pool manages a pool of Providers, one per model.
// It lazily creates Providers on first access and caches them for reuse.
type Pool struct {
	factory  ProviderFactory
	cache    map[string]Provider
	mu       sync.RWMutex
	defaults map[string]string // scenario -> default model
}

// NewPool creates a new Provider pool with the given factory function.
// The factory is called lazily when a provider for a specific model is first requested.
func NewPool(factory ProviderFactory) *Pool {
	return &Pool{
		factory:  factory,
		cache:    make(map[string]Provider),
		defaults: make(map[string]string),
	}
}

// SetDefault sets the default model for a given scenario.
// Supported scenarios: "chat", "cron", "channel"
func (p *Pool) SetDefault(scenario, model string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.defaults[scenario] = model
}

// GetDefault returns the default model for a given scenario.
// Returns empty string if no default is set.
func (p *Pool) GetDefault(scenario string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.defaults[scenario]
}

// Get retrieves or creates a Provider for the given model.
// If model is empty, returns an error.
// The provider is cached for subsequent calls with the same model.
func (p *Pool) Get(model string) (Provider, error) {
	if model == "" {
		return nil, fmt.Errorf("model name cannot be empty")
	}

	// Fast path: check cache with read lock
	p.mu.RLock()
	if provider, ok := p.cache[model]; ok {
		p.mu.RUnlock()
		return provider, nil
	}
	p.mu.RUnlock()

	// Slow path: create provider with write lock
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if provider, ok := p.cache[model]; ok {
		return provider, nil
	}

	// Create new provider
	provider, err := p.factory(model)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider for model %q: %w", model, err)
	}

	p.cache[model] = provider
	return provider, nil
}

// GetForScenario retrieves a Provider for the given scenario using its default model.
// If the scenario has no default model set, returns an error.
func (p *Pool) GetForScenario(scenario string) (Provider, error) {
	model := p.GetDefault(scenario)
	if model == "" {
		return nil, fmt.Errorf("no default model configured for scenario %q", scenario)
	}
	return p.Get(model)
}

// GetOrDefault retrieves a Provider for the given model, or falls back to the
// scenario's default model if the model is empty.
func (p *Pool) GetOrDefault(model, scenario string) (Provider, error) {
	if model == "" {
		model = p.GetDefault(scenario)
	}
	if model == "" {
		return nil, fmt.Errorf("model is empty and no default configured for scenario %q", scenario)
	}
	return p.Get(model)
}

// Models returns a list of all cached model names.
func (p *Pool) Models() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	models := make([]string, 0, len(p.cache))
	for model := range p.cache {
		models = append(models, model)
	}
	return models
}

// Defaults returns a copy of the default model mappings.
func (p *Pool) Defaults() map[string]string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[string]string, len(p.defaults))
	for k, v := range p.defaults {
		result[k] = v
	}
	return result
}

// Clear removes all cached providers.
// This is useful for testing or when configuration changes.
func (p *Pool) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[string]Provider)
}

// Count returns the number of cached providers.
func (p *Pool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.cache)
}
