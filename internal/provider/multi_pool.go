// Package provider defines the LLM provider interface and types.
package provider

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
)

// ModelInfo represents information about a model from a provider.
type ModelInfo struct {
	ID          string // Model ID (may include provider prefix for Ollama)
	Provider    string // Provider name: "copilot" or "ollama"
	DisplayName string // Human-readable name
	Available   bool   // Whether the model is currently available
	OriginalID  string // Original model ID without provider prefix
}

// MultiProviderPool manages multiple provider pools for different providers.
// It allows simultaneous access to models from Copilot and Ollama.
type MultiProviderPool struct {
	pools    map[string]*Pool  // provider name -> pool
	models   map[string]string // model id -> provider name
	defaults map[string]string // scenario -> default model
	mu       sync.RWMutex
}

// NewMultiProviderPool creates a new MultiProviderPool.
func NewMultiProviderPool() *MultiProviderPool {
	return &MultiProviderPool{
		pools:    make(map[string]*Pool),
		models:   make(map[string]string),
		defaults: make(map[string]string),
	}
}

// AddProvider adds a provider pool with its models.
// For Ollama models, the ID will be prefixed with "ollama:" to avoid conflicts.
// For Copilot models, the original ID is preserved.
func (m *MultiProviderPool) AddProvider(name string, pool *Pool, modelIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.pools[name]; exists {
		return fmt.Errorf("provider %q already registered", name)
	}

	m.pools[name] = pool

	// Register models with provider prefix for Ollama
	for _, modelID := range modelIDs {
		var finalID string
		if name == "ollama" {
			finalID = "ollama:" + modelID
		} else {
			finalID = modelID
		}
		m.models[finalID] = name
	}

	return nil
}

// GetProvider returns the Provider and provider name for the given model ID.
// For Ollama models, the model ID should include the "ollama:" prefix.
func (m *MultiProviderPool) GetProvider(modelID string) (Provider, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	providerName, ok := m.models[modelID]
	if !ok {
		// Try to infer provider from prefix
		if strings.HasPrefix(modelID, "ollama:") {
			providerName = "ollama"
		} else {
			// Try copilot first, then copilot-acp as fallback.
			// This avoids hardcoding "copilot" when only copilot-acp is enabled.
			if _, exists := m.pools["copilot"]; exists {
				providerName = "copilot"
			} else if _, exists := m.pools["copilot-acp"]; exists {
				providerName = "copilot-acp"
			} else {
				providerName = "copilot" // will trigger "not registered" error below
			}
		}
	}

	pool, ok := m.pools[providerName]
	if !ok {
		return nil, "", fmt.Errorf("provider %q not registered", providerName)
	}

	// Extract original model ID for Ollama
	originalID := modelID
	if providerName == "ollama" && strings.HasPrefix(modelID, "ollama:") {
		originalID = strings.TrimPrefix(modelID, "ollama:")
	}

	provider, err := pool.Get(originalID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get provider for model %q: %w", modelID, err)
	}

	return provider, providerName, nil
}

// ResetProviderSession resets session state for a conversationID across all providers
// that implement the SessionResettable interface. This is used when model/workspace
// changes require a full session rebuild.
func (m *MultiProviderPool) ResetProviderSession(conversationID string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, pool := range m.pools {
		// Try all cached providers in this pool
		for _, modelID := range pool.Models() {
			if prov, err := pool.Get(modelID); err == nil {
				if resettable, ok := prov.(SessionResettable); ok {
					slog.Info("Resetting provider session",
						"provider", name,
						"conversationID", conversationID)
					resettable.ResetSession(conversationID)
				}
			}
		}
	}
}

// GetPool returns the Pool for a specific provider.
func (m *MultiProviderPool) GetPool(providerName string) (*Pool, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	pool, ok := m.pools[providerName]
	return pool, ok
}

// ListAllModels returns information about all registered models from all providers.
func (m *MultiProviderPool) ListAllModels() []ModelInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var models []ModelInfo
	for modelID, providerName := range m.models {
		originalID := modelID
		if providerName == "ollama" && strings.HasPrefix(modelID, "ollama:") {
			originalID = strings.TrimPrefix(modelID, "ollama:")
		}
		models = append(models, ModelInfo{
			ID:          modelID,
			Provider:    providerName,
			DisplayName: originalID, // Can be enhanced with actual display names
			Available:   true,       // Assume available; can be enhanced with health checks
			OriginalID:  originalID,
		})
	}

	// Sort by provider, then by ID
	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		return models[i].ID < models[j].ID
	})

	return models
}

// ListProviders returns the names of all registered providers.
func (m *MultiProviderPool) ListProviders() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	providers := make([]string, 0, len(m.pools))
	for name := range m.pools {
		providers = append(providers, name)
	}
	sort.Strings(providers)
	return providers
}

// HasProvider checks if a provider is registered.
func (m *MultiProviderPool) HasProvider(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.pools[name]
	return ok
}

// GetAnyProvider returns any Provider instance from the specified provider pool.
// This is useful for health checks where we don't need a specific model.
// Returns nil if the provider is not registered or has no cached instances.
func (m *MultiProviderPool) GetAnyProvider(providerName string) Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pool, ok := m.pools[providerName]
	if !ok {
		return nil
	}

	// Try to get any cached provider from the pool
	models := pool.Models()
	if len(models) == 0 {
		// No cached providers, try to create one with any model from the registry
		for modelID, pName := range m.models {
			if pName == providerName {
				// Extract original model ID
				originalID := modelID
				if providerName == "ollama" && strings.HasPrefix(modelID, "ollama:") {
					originalID = strings.TrimPrefix(modelID, "ollama:")
				}
				// Need to release the read lock before calling Get
				m.mu.RUnlock()
				if prov, err := pool.Get(originalID); err == nil {
					m.mu.RLock()
					return prov
				}
				m.mu.RLock()
				break
			}
		}
		return nil
	}

	// Return first cached provider
	if prov, err := pool.Get(models[0]); err == nil {
		return prov
	}
	return nil
}

// SetDefault sets the default model for a given scenario.
func (m *MultiProviderPool) SetDefault(scenario, model string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaults[scenario] = model
}

// GetDefault returns the default model for a given scenario.
func (m *MultiProviderPool) GetDefault(scenario string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaults[scenario]
}

// GetOrDefault retrieves a Provider for the given model, or falls back to
// the scenario's default model if the model is empty.
func (m *MultiProviderPool) GetOrDefault(model, scenario string) (Provider, string, error) {
	if model == "" {
		model = m.GetDefault(scenario)
	}
	if model == "" {
		return nil, "", fmt.Errorf("model is empty and no default configured for scenario %q", scenario)
	}
	return m.GetProvider(model)
}

// RefreshModels updates the model list for a specific provider.
// This is useful for Ollama where models can be added/removed dynamically.
func (m *MultiProviderPool) RefreshModels(providerName string, modelIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.pools[providerName]; !ok {
		return fmt.Errorf("provider %q not registered", providerName)
	}

	// Remove old models for this provider
	for modelID, pName := range m.models {
		if pName == providerName {
			delete(m.models, modelID)
		}
	}

	// Add new models
	for _, modelID := range modelIDs {
		var finalID string
		if providerName == "ollama" {
			finalID = "ollama:" + modelID
		} else {
			finalID = modelID
		}
		m.models[finalID] = providerName
	}

	return nil
}

// RemoveProvider removes a provider and all its models.
func (m *MultiProviderPool) RemoveProvider(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.pools[name]; !ok {
		return fmt.Errorf("provider %q not registered", name)
	}

	// Remove the pool
	delete(m.pools, name)

	// Remove all models for this provider
	for modelID, pName := range m.models {
		if pName == name {
			delete(m.models, modelID)
		}
	}

	// Remove default if it belongs to this provider
	for scenario, defaultModel := range m.defaults {
		if pName, ok := m.models[defaultModel]; ok && pName == name {
			delete(m.defaults, scenario)
		}
	}

	return nil
}

// UpdateProvider updates an existing provider with a new pool and models.
// If the provider doesn't exist, it will be added.
func (m *MultiProviderPool) UpdateProvider(name string, pool *Pool, modelIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove old pool if exists
	if _, exists := m.pools[name]; exists {
		// Remove old models for this provider
		for modelID, pName := range m.models {
			if pName == name {
				delete(m.models, modelID)
			}
		}
	}

	// Add new pool
	m.pools[name] = pool

	// Add new models
	for _, modelID := range modelIDs {
		var finalID string
		if name == "ollama" {
			finalID = "ollama:" + modelID
		} else {
			finalID = modelID
		}
		m.models[finalID] = name
	}

	return nil
}

// Count returns the number of registered providers.
func (m *MultiProviderPool) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.pools)
}

// ModelCount returns the total number of registered models.
func (m *MultiProviderPool) ModelCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.models)
}

// ModelCountByProvider returns the number of models for a specific provider.
func (m *MultiProviderPool) ModelCountByProvider(providerName string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, pName := range m.models {
		if pName == providerName {
			count++
		}
	}
	return count
}
