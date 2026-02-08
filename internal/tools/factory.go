// Package tools provides tool factory pattern for dynamic tool creation.
package tools

import (
	"fmt"
	"sort"
	"sync"
)

// ToolFactory is a function type that creates a Tool instance from configuration.
// It takes a config map and returns a configured Tool or an error.
type ToolFactory func(config map[string]any) (Tool, error)

// FactoryRegistry manages a collection of tool factories.
// It is safe for concurrent use.
type FactoryRegistry struct {
	mu        sync.RWMutex
	factories map[string]ToolFactory
}

// NewFactoryRegistry creates a new empty factory registry.
func NewFactoryRegistry() *FactoryRegistry {
	return &FactoryRegistry{
		factories: make(map[string]ToolFactory),
	}
}

// Register adds a tool factory to the registry.
// Returns an error if a factory with the same name is already registered.
func (r *FactoryRegistry) Register(name string, factory ToolFactory) error {
	if name == "" {
		return NewInvalidArgsError("factory_registry", "factory name cannot be empty", nil)
	}
	if factory == nil {
		return NewInvalidArgsError("factory_registry", "factory cannot be nil", nil)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("factory '%s' already registered", name)
	}

	r.factories[name] = factory
	return nil
}

// MustRegister adds a tool factory to the registry and panics on error.
// Useful for registering built-in factories during initialization.
func (r *FactoryRegistry) MustRegister(name string, factory ToolFactory) {
	if err := r.Register(name, factory); err != nil {
		panic(err)
	}
}

// Get retrieves a factory by name.
// Returns the factory and true if found, nil and false otherwise.
func (r *FactoryRegistry) Get(name string) (ToolFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, ok := r.factories[name]
	return factory, ok
}

// Create creates a Tool instance using the named factory and configuration.
// Returns an error if the factory is not found or if creation fails.
func (r *FactoryRegistry) Create(name string, config map[string]any) (Tool, error) {
	factory, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("factory '%s' not found", name)
	}

	if config == nil {
		config = make(map[string]any)
	}

	return factory(config)
}

// List returns the names of all registered factories in sorted order.
func (r *FactoryRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Unregister removes a factory from the registry.
// Returns an error if the factory is not registered.
func (r *FactoryRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; !exists {
		return fmt.Errorf("factory '%s' not found", name)
	}

	delete(r.factories, name)
	return nil
}

// Len returns the number of registered factories.
func (r *FactoryRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.factories)
}

// Has checks if a factory with the given name exists.
func (r *FactoryRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.factories[name]
	return exists
}

// Clear removes all factories from the registry.
func (r *FactoryRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories = make(map[string]ToolFactory)
}
