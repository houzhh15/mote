package ui

import (
	"encoding/json"
	"sync"

	"mote/pkg/logger"
)

// Broadcaster defines the interface for broadcasting messages.
// This allows for easier testing with mock implementations.
type Broadcaster interface {
	BroadcastAll(data []byte)
}

// StateManager manages UI state and broadcasts changes.
type StateManager struct {
	state *UIState
	hub   Broadcaster
	mu    sync.RWMutex
}

// NewStateManager creates a new StateManager with default state.
func NewStateManager(hub Broadcaster) *StateManager {
	return &StateManager{
		state: &UIState{
			Theme:       "system",
			SidebarOpen: true,
			CurrentPage: "chat",
			Custom:      make(map[string]any),
		},
		hub: hub,
	}
}

// Get returns a copy of the current UI state.
func (sm *StateManager) Get() UIState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.copyStateLocked()
}

// copyStateLocked returns a deep copy of the current state.
// Caller must hold at least a read lock.
func (sm *StateManager) copyStateLocked() UIState {
	stateCopy := UIState{
		ActiveSession: sm.state.ActiveSession,
		Theme:         sm.state.Theme,
		SidebarOpen:   sm.state.SidebarOpen,
		CurrentPage:   sm.state.CurrentPage,
	}

	// Deep copy custom map
	if sm.state.Custom != nil {
		stateCopy.Custom = make(map[string]any, len(sm.state.Custom))
		for k, v := range sm.state.Custom {
			stateCopy.Custom[k] = v
		}
	}

	return stateCopy
}

// Update merges partial state updates and broadcasts the change.
func (sm *StateManager) Update(partial UIState) {
	sm.mu.Lock()

	// Merge non-zero/non-empty values
	if partial.ActiveSession != "" {
		sm.state.ActiveSession = partial.ActiveSession
	}
	if partial.Theme != "" {
		sm.state.Theme = partial.Theme
	}
	if partial.CurrentPage != "" {
		sm.state.CurrentPage = partial.CurrentPage
	}
	// SidebarOpen is a bool, always update if explicitly set
	// We use a special marker to detect intentional updates
	sm.state.SidebarOpen = partial.SidebarOpen

	// Merge custom fields
	if partial.Custom != nil {
		if sm.state.Custom == nil {
			sm.state.Custom = make(map[string]any)
		}
		for k, v := range partial.Custom {
			sm.state.Custom[k] = v
		}
	}

	// Copy state for broadcast (while still holding lock)
	stateCopy := sm.copyStateLocked()
	sm.mu.Unlock()

	// Broadcast outside of lock
	sm.broadcastState(stateCopy)
}

// UpdateSidebarOpen explicitly updates the sidebar state.
func (sm *StateManager) UpdateSidebarOpen(open bool) {
	sm.mu.Lock()
	sm.state.SidebarOpen = open
	stateCopy := *sm.state
	if sm.state.Custom != nil {
		stateCopy.Custom = make(map[string]any, len(sm.state.Custom))
		for k, v := range sm.state.Custom {
			stateCopy.Custom[k] = v
		}
	}
	sm.mu.Unlock()

	sm.broadcastState(stateCopy)
}

// broadcastState sends the current state to all connected clients.
func (sm *StateManager) broadcastState(state UIState) {
	if sm.hub == nil {
		return
	}

	stateJSON, err := json.Marshal(state)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to marshal UI state for broadcast")
		return
	}

	msg := struct {
		Type  string          `json:"type"`
		State json.RawMessage `json:"state"`
	}{
		Type:  "ui_state",
		State: stateJSON,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to marshal UI state message")
		return
	}

	sm.hub.BroadcastAll(data)
}

// SetActiveSession updates the active session.
func (sm *StateManager) SetActiveSession(sessionID string) {
	sm.Update(UIState{ActiveSession: sessionID})
}

// SetTheme updates the theme.
func (sm *StateManager) SetTheme(theme string) {
	sm.Update(UIState{Theme: theme})
}

// SetCurrentPage updates the current page.
func (sm *StateManager) SetCurrentPage(page string) {
	sm.Update(UIState{CurrentPage: page})
}

// SetCustom updates a custom state field.
func (sm *StateManager) SetCustom(key string, value any) {
	sm.Update(UIState{Custom: map[string]any{key: value}})
}
