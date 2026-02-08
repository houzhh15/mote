// Package workspace provides workspace binding and file management.
package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// WorkspaceBinding represents a bound workspace for a session.
type WorkspaceBinding struct {
	SessionID  string    `json:"session_id"`
	Path       string    `json:"path"`
	Alias      string    `json:"alias,omitempty"`
	ReadOnly   bool      `json:"read_only"`
	BoundAt    time.Time `json:"bound_at"`
	LastAccess time.Time `json:"last_access"`
}

// FileInfo represents file metadata.
type FileInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// WorkspaceManager manages workspace bindings.
type WorkspaceManager struct {
	mu           sync.RWMutex
	bindings     map[string]*WorkspaceBinding
	bindingsPath string
}

// NewWorkspaceManager creates a new WorkspaceManager.
func NewWorkspaceManager() *WorkspaceManager {
	homeDir, _ := os.UserHomeDir()
	bindingsPath := filepath.Join(homeDir, ".mote", "workspace_bindings.json")

	m := &WorkspaceManager{
		bindings:     make(map[string]*WorkspaceBinding),
		bindingsPath: bindingsPath,
	}

	// Load persisted bindings
	m.load()

	return m
}

// load reads bindings from disk.
func (m *WorkspaceManager) load() {
	data, err := os.ReadFile(m.bindingsPath)
	if err != nil {
		// File doesn't exist or can't be read, start with empty bindings
		return
	}

	var bindings map[string]*WorkspaceBinding
	if err := json.Unmarshal(data, &bindings); err != nil {
		// Invalid JSON, start with empty bindings
		return
	}

	m.bindings = bindings
}

// save writes bindings to disk.
func (m *WorkspaceManager) save() error {
	// Ensure directory exists
	dir := filepath.Dir(m.bindingsPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(m.bindings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal bindings: %w", err)
	}

	if err := os.WriteFile(m.bindingsPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write bindings file: %w", err)
	}

	return nil
}

// Bind binds a workspace path to a session.
func (m *WorkspaceManager) Bind(sessionID, path string, readOnly bool) error {
	return m.BindWithAlias(sessionID, path, "", readOnly)
}

// BindWithAlias binds a workspace path to a session with an alias.
func (m *WorkspaceManager) BindWithAlias(sessionID, path, alias string, readOnly bool) error {
	if sessionID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}
	if path == "" {
		return fmt.Errorf("workspace path cannot be empty")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("workspace path does not exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace path is not a directory")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.bindings[sessionID] = &WorkspaceBinding{
		SessionID:  sessionID,
		Path:       absPath,
		Alias:      alias,
		ReadOnly:   readOnly,
		BoundAt:    time.Now(),
		LastAccess: time.Now(),
	}

	// Persist to disk
	if err := m.save(); err != nil {
		// Log error but don't fail the bind operation
		fmt.Fprintf(os.Stderr, "Warning: failed to persist workspace binding: %v\n", err)
	}

	return nil
}

// Unbind removes a workspace binding for a session.
func (m *WorkspaceManager) Unbind(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.bindings[sessionID]; !exists {
		return fmt.Errorf("session %s not bound", sessionID)
	}

	delete(m.bindings, sessionID)

	// Persist to disk
	if err := m.save(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to persist workspace unbinding: %v\n", err)
	}

	return nil
}

// Get returns the workspace binding for a session.
func (m *WorkspaceManager) Get(sessionID string) (*WorkspaceBinding, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	binding, exists := m.bindings[sessionID]
	if exists {
		binding.LastAccess = time.Now()
	}
	return binding, exists
}

// IsBound returns true if a session has a workspace binding.
func (m *WorkspaceManager) IsBound(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.bindings[sessionID]
	return exists
}

// List returns all workspace bindings.
func (m *WorkspaceManager) List() []*WorkspaceBinding {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*WorkspaceBinding, 0, len(m.bindings))
	for _, b := range m.bindings {
		result = append(result, b)
	}
	return result
}

// CleanupOrphanBindings removes workspace bindings for sessions that no longer exist.
// The sessionExists function should return true if the session exists.
func (m *WorkspaceManager) CleanupOrphanBindings(sessionExists func(sessionID string) bool) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	toDelete := []string{}
	for sessionID := range m.bindings {
		if !sessionExists(sessionID) {
			toDelete = append(toDelete, sessionID)
		}
	}

	for _, sessionID := range toDelete {
		delete(m.bindings, sessionID)
	}

	if len(toDelete) > 0 {
		if err := m.save(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to persist workspace cleanup: %v\n", err)
		}
	}

	return len(toDelete)
}

// Len returns the number of bindings.
func (m *WorkspaceManager) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.bindings)
}

// ResolvePath resolves a relative path within a workspace.
// Prevents path traversal attacks.
func (m *WorkspaceManager) ResolvePath(sessionID, relativePath string) (string, error) {
	binding, exists := m.Get(sessionID)
	if !exists {
		return "", fmt.Errorf("session %s not bound", sessionID)
	}

	if filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("absolute paths not allowed")
	}

	absPath := filepath.Join(binding.Path, relativePath)
	absPath = filepath.Clean(absPath)

	if !strings.HasPrefix(absPath, binding.Path) {
		return "", fmt.Errorf("path traversal not allowed")
	}

	return absPath, nil
}

// ListFiles lists files in a directory within the workspace.
func (m *WorkspaceManager) ListFiles(sessionID, relativePath string) ([]FileInfo, error) {
	absPath, err := m.ResolvePath(sessionID, relativePath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, FileInfo{
			Name:    entry.Name(),
			Path:    filepath.Join(relativePath, entry.Name()),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	return files, nil
}

// ReadFile reads a file from the workspace.
func (m *WorkspaceManager) ReadFile(sessionID, relativePath string) ([]byte, error) {
	absPath, err := m.ResolvePath(sessionID, relativePath)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return data, nil
}

// WriteFile writes a file to the workspace.
func (m *WorkspaceManager) WriteFile(sessionID, relativePath string, content []byte) error {
	binding, exists := m.Get(sessionID)
	if !exists {
		return fmt.Errorf("session %s not bound", sessionID)
	}

	if binding.ReadOnly {
		return fmt.Errorf("workspace is read-only")
	}

	absPath, err := m.ResolvePath(sessionID, relativePath)
	if err != nil {
		return err
	}

	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(absPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
