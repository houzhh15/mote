package provider

import (
	"sort"
	"sync"
)

var (
	providers       = make(map[string]Provider)
	defaultProvider Provider
	mu              sync.RWMutex
)

// Register registers a provider.
// The first registered provider becomes the default.
func Register(p Provider) {
	mu.Lock()
	defer mu.Unlock()

	name := p.Name()
	providers[name] = p

	if defaultProvider == nil {
		defaultProvider = p
	}
}

// Get returns a provider by name.
func Get(name string) (Provider, bool) {
	mu.RLock()
	defer mu.RUnlock()

	p, ok := providers[name]
	return p, ok
}

// Default returns the default provider.
func Default() Provider {
	mu.RLock()
	defer mu.RUnlock()

	return defaultProvider
}

// SetDefault sets the default provider by name.
func SetDefault(name string) bool {
	mu.Lock()
	defer mu.Unlock()

	if p, ok := providers[name]; ok {
		defaultProvider = p
		return true
	}
	return false
}

// List returns the names of all registered providers.
func List() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Reset clears all registered providers (for testing).
func Reset() {
	mu.Lock()
	defer mu.Unlock()

	providers = make(map[string]Provider)
	defaultProvider = nil
}
