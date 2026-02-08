// Package prompts provides prompt management for Mote.
package prompts

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// PromptType represents the type of prompt.
type PromptType string

const (
	PromptTypeSystem    PromptType = "system"
	PromptTypeUser      PromptType = "user"
	PromptTypeAssistant PromptType = "assistant"
)

// Prompt represents a prompt entry.
type Prompt struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Type      PromptType `json:"type"`
	Content   string     `json:"content"`
	Priority  int        `json:"priority"`
	Enabled   bool       `json:"enabled"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// PromptConfig holds configuration for creating a prompt.
type PromptConfig struct {
	Name     string
	Type     PromptType
	Content  string
	Priority int
	Enabled  bool
}

// Manager manages prompts.
type Manager struct {
	mu      sync.RWMutex
	prompts map[string]*Prompt
}

// NewManager creates a new prompt manager.
func NewManager() *Manager {
	return &Manager{
		prompts: make(map[string]*Prompt),
	}
}

// AddPrompt adds a new prompt.
func (m *Manager) AddPrompt(cfg PromptConfig) (*Prompt, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("prompt name cannot be empty")
	}
	if cfg.Content == "" {
		return nil, fmt.Errorf("prompt content cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicate name
	for _, p := range m.prompts {
		if p.Name == cfg.Name {
			return nil, fmt.Errorf("prompt with name '%s' already exists", cfg.Name)
		}
	}

	promptType := cfg.Type
	if promptType == "" {
		promptType = PromptTypeSystem
	}

	now := time.Now()
	prompt := &Prompt{
		ID:        uuid.New().String(),
		Name:      cfg.Name,
		Type:      promptType,
		Content:   cfg.Content,
		Priority:  cfg.Priority,
		Enabled:   cfg.Enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}

	m.prompts[prompt.ID] = prompt
	return prompt, nil
}

// GetPrompt returns a prompt by ID.
func (m *Manager) GetPrompt(id string) (*Prompt, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	prompt, exists := m.prompts[id]
	if !exists {
		return nil, fmt.Errorf("prompt not found: %s", id)
	}
	return prompt, nil
}

// RemovePrompt removes a prompt by ID.
func (m *Manager) RemovePrompt(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.prompts[id]; !exists {
		return fmt.Errorf("prompt not found: %s", id)
	}

	delete(m.prompts, id)
	return nil
}

// ListPrompts returns all prompts.
func (m *Manager) ListPrompts() []*Prompt {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Prompt, 0, len(m.prompts))
	for _, p := range m.prompts {
		result = append(result, p)
	}
	return result
}

// EnablePrompt enables a prompt.
func (m *Manager) EnablePrompt(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	prompt, exists := m.prompts[id]
	if !exists {
		return fmt.Errorf("prompt not found: %s", id)
	}

	prompt.Enabled = true
	prompt.UpdatedAt = time.Now()
	return nil
}

// DisablePrompt disables a prompt.
func (m *Manager) DisablePrompt(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	prompt, exists := m.prompts[id]
	if !exists {
		return fmt.Errorf("prompt not found: %s", id)
	}

	prompt.Enabled = false
	prompt.UpdatedAt = time.Now()
	return nil
}

// GetEnabledPrompts returns all enabled prompts sorted by priority.
func (m *Manager) GetEnabledPrompts() []*Prompt {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Prompt, 0)
	for _, p := range m.prompts {
		if p.Enabled {
			result = append(result, p)
		}
	}

	// Sort by priority (lower = earlier)
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Priority > result[j].Priority {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

// UpdatePrompt updates a prompt's content.
func (m *Manager) UpdatePrompt(id, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	prompt, exists := m.prompts[id]
	if !exists {
		return fmt.Errorf("prompt not found: %s", id)
	}

	prompt.Content = content
	prompt.UpdatedAt = time.Now()
	return nil
}

// SetPriority sets a prompt's priority.
func (m *Manager) SetPriority(id string, priority int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	prompt, exists := m.prompts[id]
	if !exists {
		return fmt.Errorf("prompt not found: %s", id)
	}

	prompt.Priority = priority
	prompt.UpdatedAt = time.Now()
	return nil
}
