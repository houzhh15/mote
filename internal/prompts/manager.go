// Package prompts provides prompt management for Mote.
package prompts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// PromptType represents the type of prompt.
type PromptType string

const (
	PromptTypeSystem    PromptType = "system"
	PromptTypeUser      PromptType = "user"
	PromptTypeAssistant PromptType = "assistant"
)

// PromptArgument represents a prompt argument definition (MCP compatible).
type PromptArgument struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
}

// PromptSource indicates where the prompt comes from.
type PromptSource string

const (
	PromptSourceMemory    PromptSource = "memory"    // Created via API, in-memory only
	PromptSourceFile      PromptSource = "file"      // Loaded from .mote/prompts/
	PromptSourceUserDir   PromptSource = "user_dir"  // Loaded from ~/.mote/prompts/
	PromptSourceWorkspace PromptSource = "workspace" // Loaded from ./.mote/prompts/
)

// Prompt represents a prompt entry.
type Prompt struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Type        PromptType       `json:"type"`
	Content     string           `json:"content"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
	Priority    int              `json:"priority"`
	Enabled     bool             `json:"enabled"`
	Source      PromptSource     `json:"source"`
	FilePath    string           `json:"file_path,omitempty"` // If loaded from file
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// PromptFrontmatter represents the YAML frontmatter in a markdown prompt file.
type PromptFrontmatter struct {
	Name        string           `yaml:"name"`
	Description string           `yaml:"description,omitempty"`
	Type        string           `yaml:"type,omitempty"`
	Arguments   []PromptArgument `yaml:"arguments,omitempty"`
	Priority    int              `yaml:"priority,omitempty"`
	Enabled     *bool            `yaml:"enabled,omitempty"`
}

// PromptConfig holds configuration for creating a prompt.
type PromptConfig struct {
	Name        string
	Description string
	Type        PromptType
	Content     string
	Arguments   []PromptArgument
	Priority    int
	Enabled     bool
}

// Manager manages prompts.
type Manager struct {
	mu             sync.RWMutex
	prompts        map[string]*Prompt
	promptsDirs    []string // Directories to load prompts from
	enableAutoSave bool     // Auto-save new prompts to file
}

// ManagerConfig holds configuration for the prompt manager.
type ManagerConfig struct {
	PromptsDirs    []string // Directories to load prompts from
	EnableAutoSave bool     // Auto-save new prompts to first directory
}

// NewManager creates a new prompt manager.
func NewManager() *Manager {
	return &Manager{
		prompts:        make(map[string]*Prompt),
		promptsDirs:    []string{},
		enableAutoSave: false,
	}
}

// NewManagerWithConfig creates a new prompt manager with configuration.
func NewManagerWithConfig(cfg ManagerConfig) *Manager {
	m := &Manager{
		prompts:        make(map[string]*Prompt),
		promptsDirs:    cfg.PromptsDirs,
		enableAutoSave: cfg.EnableAutoSave,
	}

	// Load prompts from all configured directories
	for _, dir := range cfg.PromptsDirs {
		_ = m.LoadFromDirectory(dir)
	}

	return m
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
		ID:          uuid.New().String(),
		Name:        cfg.Name,
		Description: cfg.Description,
		Type:        promptType,
		Content:     cfg.Content,
		Arguments:   cfg.Arguments,
		Priority:    cfg.Priority,
		Enabled:     cfg.Enabled,
		Source:      PromptSourceMemory,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	m.prompts[prompt.ID] = prompt

	// Auto-save to file if enabled
	if m.enableAutoSave && len(m.promptsDirs) > 0 {
		_ = m.savePromptToFile(prompt, m.promptsDirs[0])
	}

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

	prompt, exists := m.prompts[id]
	if !exists {
		return fmt.Errorf("prompt not found: %s", id)
	}

	// Delete the file if it exists and was loaded from file
	if prompt.FilePath != "" && (prompt.Source == PromptSourceFile || prompt.Source == PromptSourceUserDir || prompt.Source == PromptSourceWorkspace) {
		if err := os.Remove(prompt.FilePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to delete prompt file %s: %w", prompt.FilePath, err)
		}
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

	// Persist to file if the prompt was loaded from a file
	if prompt.FilePath != "" {
		if err := m.writePromptFile(prompt); err != nil {
			return fmt.Errorf("failed to persist prompt to file: %w", err)
		}
	}

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

// LoadFromDirectory loads prompts from a directory containing markdown files.
func (m *Manager) LoadFromDirectory(dir string) error {
	// Ensure directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil // Silently skip non-existent directories
	}

	// Read all .md files
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		if err := m.loadPromptFromFile(filePath); err != nil {
			// Log error but continue loading other files
			continue
		}
	}

	return nil
}

// loadPromptFromFile loads a single prompt from a markdown file.
func (m *Manager) loadPromptFromFile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Parse frontmatter and content
	frontmatter, content, err := parseFrontmatter(string(data))
	if err != nil {
		return fmt.Errorf("failed to parse frontmatter in %s: %w", filePath, err)
	}

	// Validate required fields
	if frontmatter.Name == "" {
		return fmt.Errorf("prompt name is required in %s", filePath)
	}

	// Determine source based on file path
	source := PromptSourceFile
	if strings.Contains(filePath, "/.mote/prompts") {
		source = PromptSourceWorkspace
	}
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" && strings.HasPrefix(filePath, filepath.Join(homeDir, ".mote/prompts")) {
		source = PromptSourceUserDir
	}

	// Set defaults
	promptType := PromptType(frontmatter.Type)
	if promptType == "" {
		promptType = PromptTypeSystem
	}

	enabled := true
	if frontmatter.Enabled != nil {
		enabled = *frontmatter.Enabled
	}

	now := time.Now()
	prompt := &Prompt{
		ID:          uuid.New().String(),
		Name:        frontmatter.Name,
		Description: frontmatter.Description,
		Type:        promptType,
		Content:     content,
		Arguments:   frontmatter.Arguments,
		Priority:    frontmatter.Priority,
		Enabled:     enabled,
		Source:      source,
		FilePath:    filePath,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicate name
	for _, p := range m.prompts {
		if p.Name == frontmatter.Name {
			// Update existing prompt if it's from the same or lower priority source
			if p.Source == PromptSourceMemory || p.FilePath == filePath {
				p.Description = prompt.Description
				p.Type = prompt.Type
				p.Content = prompt.Content
				p.Arguments = prompt.Arguments
				p.Priority = prompt.Priority
				p.Enabled = prompt.Enabled
				p.UpdatedAt = now
				return nil
			}
			return fmt.Errorf("prompt with name '%s' already exists", frontmatter.Name)
		}
	}

	m.prompts[prompt.ID] = prompt
	return nil
}

// parseFrontmatter extracts YAML frontmatter and content from markdown.
func parseFrontmatter(data string) (*PromptFrontmatter, string, error) {
	// Check for frontmatter delimiter
	if !strings.HasPrefix(data, "---\n") {
		return &PromptFrontmatter{}, data, nil
	}

	// Find end of frontmatter
	parts := strings.SplitN(data[4:], "\n---\n", 2)
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("invalid frontmatter format")
	}

	// Parse YAML frontmatter
	var fm PromptFrontmatter
	if err := yaml.Unmarshal([]byte(parts[0]), &fm); err != nil {
		return nil, "", fmt.Errorf("failed to parse YAML frontmatter: %w", err)
	}

	// Content is everything after frontmatter.
	// Use TrimRight to only strip trailing whitespace (newlines),
	// preserving all meaningful content characters.
	content := strings.TrimRight(parts[1], " \t\n\r")
	// Also trim leading whitespace (the blank line after ---)
	content = strings.TrimLeft(content, " \t\n\r")

	return &fm, content, nil
}

// writePromptFile writes a prompt to its FilePath as a markdown file with YAML frontmatter.
func (m *Manager) writePromptFile(prompt *Prompt) error {
	// Build frontmatter
	fm := PromptFrontmatter{
		Name:        prompt.Name,
		Description: prompt.Description,
		Type:        string(prompt.Type),
		Arguments:   prompt.Arguments,
		Priority:    prompt.Priority,
		Enabled:     &prompt.Enabled,
	}

	fmData, err := yaml.Marshal(&fm)
	if err != nil {
		return fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	// Build markdown content with trailing newline for POSIX compliance
	content := fmt.Sprintf("---\n%s---\n\n%s\n", string(fmData), prompt.Content)

	// Write file
	if err := os.WriteFile(prompt.FilePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", prompt.FilePath, err)
	}

	return nil
}

// savePromptToFile saves a prompt to a new markdown file in the given directory.
func (m *Manager) savePromptToFile(prompt *Prompt, dir string) error {
	// Ensure directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Generate filename from name
	filename := sanitizeFilename(prompt.Name) + ".md"
	prompt.FilePath = filepath.Join(dir, filename)
	prompt.Source = PromptSourceFile

	return m.writePromptFile(prompt)
}

// sanitizeFilename converts a prompt name to a safe filename.
// Supports Unicode characters (Chinese, Japanese, etc.) while removing
// dangerous characters like path separators and control characters.
func sanitizeFilename(name string) string {
	// Replace spaces with dashes
	name = strings.ReplaceAll(name, " ", "-")

	// Keep Unicode letters, digits, dash and underscore; remove everything else
	var result strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			result.WriteRune(r)
		}
	}

	s := result.String()
	if s == "" {
		// Fallback: use a UUID fragment when the name produces an empty string
		s = uuid.New().String()[:8]
	}

	return s
}

// ReloadFromFiles reloads all prompts from configured directories.
func (m *Manager) ReloadFromFiles() error {
	m.mu.Lock()

	// Remove file-based prompts
	for id, p := range m.prompts {
		if p.Source != PromptSourceMemory {
			delete(m.prompts, id)
		}
	}
	m.mu.Unlock()

	// Reload from directories
	for _, dir := range m.promptsDirs {
		if err := m.LoadFromDirectory(dir); err != nil {
			return err
		}
	}

	return nil
}
