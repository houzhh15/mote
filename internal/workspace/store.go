package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Workspace represents a registered workspace.
type Workspace struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Path      string         `json:"path"`
	Active    bool           `json:"active"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// WorkspaceStore manages persistent workspace storage.
type WorkspaceStore struct {
	mu         sync.RWMutex
	workspaces map[string]*Workspace
	configPath string
}

// NewWorkspaceStore creates a new workspace store.
func NewWorkspaceStore() *WorkspaceStore {
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, ".mote", "workspaces.json")

	store := &WorkspaceStore{
		workspaces: make(map[string]*Workspace),
		configPath: configPath,
	}

	// Load existing workspaces
	store.load()

	return store
}

// ListWorkspaces returns all registered workspaces.
func (m *WorkspaceManager) ListWorkspaces() ([]*Workspace, error) {
	store := NewWorkspaceStore()
	return store.List(), nil
}

// BindWorkspace binds a new workspace path.
func (m *WorkspaceManager) BindWorkspace(name, path string) (*Workspace, error) {
	store := NewWorkspaceStore()

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("workspace path does not exist: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace path is not a directory")
	}

	ws := store.Add(name, absPath)
	return ws, nil
}

// UnbindWorkspace removes a workspace binding.
func (m *WorkspaceManager) UnbindWorkspace(id string) error {
	store := NewWorkspaceStore()
	return store.Remove(id)
}

// GetWorkspace returns a workspace by ID.
func (m *WorkspaceManager) GetWorkspace(id string) (*Workspace, error) {
	store := NewWorkspaceStore()
	ws, exists := store.Get(id)
	if !exists {
		return nil, fmt.Errorf("workspace not found: %s", id)
	}
	return ws, nil
}

// Add adds a new workspace.
func (s *WorkspaceStore) Add(name, path string) *Workspace {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	ws := &Workspace{
		ID:        uuid.New().String()[:8],
		Name:      name,
		Path:      path,
		Active:    true,
		Metadata:  make(map[string]any),
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.workspaces[ws.ID] = ws
	s.save()

	return ws
}

// Get returns a workspace by ID.
func (s *WorkspaceStore) Get(id string) (*Workspace, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ws, exists := s.workspaces[id]
	return ws, exists
}

// Remove removes a workspace by ID.
func (s *WorkspaceStore) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.workspaces[id]; !exists {
		return fmt.Errorf("workspace not found: %s", id)
	}

	delete(s.workspaces, id)
	s.save()

	return nil
}

// List returns all workspaces.
func (s *WorkspaceStore) List() []*Workspace {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Workspace, 0, len(s.workspaces))
	for _, ws := range s.workspaces {
		result = append(result, ws)
	}
	return result
}

// load loads workspaces from disk.
func (s *WorkspaceStore) load() {
	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return // File may not exist yet
	}

	var workspaces []*Workspace
	if err := json.Unmarshal(data, &workspaces); err != nil {
		return
	}

	for _, ws := range workspaces {
		s.workspaces[ws.ID] = ws
	}
}

// save saves workspaces to disk.
func (s *WorkspaceStore) save() {
	workspaces := make([]*Workspace, 0, len(s.workspaces))
	for _, ws := range s.workspaces {
		workspaces = append(workspaces, ws)
	}

	data, err := json.MarshalIndent(workspaces, "", "  ")
	if err != nil {
		return
	}

	// Ensure directory exists
	dir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	os.WriteFile(s.configPath, data, 0644)
}
