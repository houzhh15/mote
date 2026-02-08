// Package prompt provides user prompt management.
package prompt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// UserPrompt represents a user-defined prompt template.
type UserPrompt struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Content     string     `json:"content"`
	Slot        PromptSlot `json:"slot,omitempty"` // Target injection slot
	Tags        []string   `json:"tags,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// UserPromptStore is the interface for persisting user prompts.
type UserPromptStore interface {
	Get(name string) (*UserPrompt, error)
	Set(prompt *UserPrompt) error
	Delete(name string) error
	List() ([]*UserPrompt, error)
}

// FileUserPromptStore implements UserPromptStore using a JSON file.
type FileUserPromptStore struct {
	path string
	mu   sync.RWMutex
}

// NewFileUserPromptStore creates a new file-based user prompt store.
// If path is empty, uses default ~/.mote/user-prompts.json.
func NewFileUserPromptStore(path string) *FileUserPromptStore {
	if path == "" {
		homeDir, _ := os.UserHomeDir()
		path = filepath.Join(homeDir, ".mote", "user-prompts.json")
	}
	return &FileUserPromptStore{path: path}
}

// Get retrieves a user prompt by name.
func (s *FileUserPromptStore) Get(name string) (*UserPrompt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prompts, err := s.loadPrompts()
	if err != nil {
		return nil, err
	}

	for _, p := range prompts {
		if p.Name == name {
			return p, nil
		}
	}
	return nil, nil
}

// Set stores or updates a user prompt.
func (s *FileUserPromptStore) Set(prompt *UserPrompt) error {
	if prompt == nil || prompt.Name == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	prompts, err := s.loadPrompts()
	if err != nil {
		prompts = make([]*UserPrompt, 0)
	}

	// Check if prompt exists and update, or append new
	found := false
	for i, p := range prompts {
		if p.Name == prompt.Name {
			prompt.UpdatedAt = time.Now()
			prompts[i] = prompt
			found = true
			break
		}
	}

	if !found {
		prompt.CreatedAt = time.Now()
		prompt.UpdatedAt = prompt.CreatedAt
		prompts = append(prompts, prompt)
	}

	return s.savePrompts(prompts)
}

// Delete removes a user prompt by name.
func (s *FileUserPromptStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prompts, err := s.loadPrompts()
	if err != nil {
		return nil
	}

	filtered := make([]*UserPrompt, 0)
	for _, p := range prompts {
		if p.Name != name {
			filtered = append(filtered, p)
		}
	}

	return s.savePrompts(filtered)
}

// List returns all user prompts.
func (s *FileUserPromptStore) List() ([]*UserPrompt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadPrompts()
}

// loadPrompts reads prompts from the JSON file.
func (s *FileUserPromptStore) loadPrompts() ([]*UserPrompt, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return make([]*UserPrompt, 0), nil
		}
		return nil, err
	}

	var prompts []*UserPrompt
	if err := json.Unmarshal(data, &prompts); err != nil {
		return nil, err
	}
	return prompts, nil
}

// savePrompts writes prompts to the JSON file.
func (s *FileUserPromptStore) savePrompts(prompts []*UserPrompt) error {
	// Ensure directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(prompts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}
