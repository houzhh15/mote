package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ConfigStore defines the interface for skill configuration persistence.
type ConfigStore interface {
	// Get retrieves the configuration for a skill by ID.
	Get(skillID string) (ConfigMap, error)
	// Set stores the configuration for a skill.
	Set(skillID string, cfg ConfigMap) error
	// Delete removes the configuration for a skill.
	Delete(skillID string) error
	// List returns all stored skill configurations.
	List() (map[string]ConfigMap, error)
}

// FileConfigStore implements ConfigStore using a JSON file.
type FileConfigStore struct {
	path string
	mu   sync.RWMutex
}

// NewFileConfigStore creates a new FileConfigStore.
// Default path is ~/.mote/skill-configs.json.
func NewFileConfigStore(path string) *FileConfigStore {
	if path == "" {
		homeDir, _ := os.UserHomeDir()
		path = filepath.Join(homeDir, ".mote", "skill-configs.json")
	}
	return &FileConfigStore{
		path: path,
	}
}

// Get retrieves the configuration for a skill by ID.
func (s *FileConfigStore) Get(skillID string) (ConfigMap, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	configs, err := s.loadAll()
	if err != nil {
		return nil, err
	}

	cfg, exists := configs[skillID]
	if !exists {
		return nil, nil
	}
	return cfg, nil
}

// Set stores the configuration for a skill.
func (s *FileConfigStore) Set(skillID string, cfg ConfigMap) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	configs, err := s.loadAll()
	if err != nil {
		// If file doesn't exist, start with empty map
		configs = make(map[string]ConfigMap)
	}

	configs[skillID] = cfg
	return s.saveAll(configs)
}

// Delete removes the configuration for a skill.
func (s *FileConfigStore) Delete(skillID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	configs, err := s.loadAll()
	if err != nil {
		return nil
	}

	delete(configs, skillID)
	return s.saveAll(configs)
}

// List returns all stored skill configurations.
func (s *FileConfigStore) List() (map[string]ConfigMap, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.loadAll()
}

// loadAll reads all configurations from the file.
func (s *FileConfigStore) loadAll() (map[string]ConfigMap, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]ConfigMap), nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var configs map[string]ConfigMap
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return configs, nil
}

// saveAll writes all configurations to the file.
func (s *FileConfigStore) saveAll(configs map[string]ConfigMap) error {
	// Ensure directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configs: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
