// Package ui provides UI component management for the Mote gateway.
package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Registry manages UI components.
type Registry struct {
	components map[string]Component
	uiDir      string
	mu         sync.RWMutex
}

// NewRegistry creates a new Registry with the given UI directory path.
func NewRegistry(uiDir string) *Registry {
	return &Registry{
		components: make(map[string]Component),
		uiDir:      uiDir,
	}
}

// Scan scans the UI directory for components and loads the manifest.
// Returns nil if the directory or manifest does not exist.
func (r *Registry) Scan() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing components
	r.components = make(map[string]Component)

	// Check if UI directory exists
	if r.uiDir == "" {
		return nil
	}

	info, err := os.Stat(r.uiDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return nil
	}

	// Try to load manifest.json
	manifestPath := filepath.Join(r.uiDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if os.IsNotExist(err) {
		// No manifest, try to scan components directory
		return r.scanComponentsDir()
	}
	if err != nil {
		return err
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}

	// Load components from manifest
	for _, comp := range manifest.Components {
		r.components[comp.Name] = comp
	}

	return nil
}

// scanComponentsDir scans the components directory for individual component files.
func (r *Registry) scanComponentsDir() error {
	componentsDir := filepath.Join(r.uiDir, "components")
	info, err := os.Stat(componentsDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return nil
	}

	entries, err := os.ReadDir(componentsDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if ext == ".js" || ext == ".jsx" || ext == ".ts" || ext == ".tsx" {
			compName := name[:len(name)-len(ext)]
			r.components[compName] = Component{
				Name: compName,
				File: filepath.Join("components", name),
			}
		}
	}

	return nil
}

// List returns all registered components.
func (r *Registry) List() []Component {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Component, 0, len(r.components))
	for _, comp := range r.components {
		result = append(result, comp)
	}
	return result
}

// Get returns a component by name and whether it exists.
func (r *Registry) Get(name string) (*Component, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	comp, ok := r.components[name]
	if !ok {
		return nil, false
	}
	return &comp, true
}

// Refresh clears the registry and rescans the UI directory.
func (r *Registry) Refresh() error {
	return r.Scan()
}

// Count returns the number of registered components.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.components)
}
