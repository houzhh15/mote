package ui

import (
	"encoding/json"
	"sync"
	"testing"
)

// mockBroadcaster is a mock implementation of Broadcaster for testing.
type mockBroadcaster struct {
	mu        sync.Mutex
	messages  [][]byte
	callCount int
}

func newMockBroadcaster() *mockBroadcaster {
	return &mockBroadcaster{
		messages: make([][]byte, 0),
	}
}

func (m *mockBroadcaster) BroadcastAll(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, data)
	m.callCount++
}

func (m *mockBroadcaster) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func (m *mockBroadcaster) getLastMessage() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.messages) == 0 {
		return nil
	}
	return m.messages[len(m.messages)-1]
}

func TestNewStateManager(t *testing.T) {
	hub := newMockBroadcaster()
	sm := NewStateManager(hub)

	if sm == nil {
		t.Fatal("NewStateManager returned nil")
	}

	state := sm.Get()
	if state.Theme != "system" {
		t.Errorf("expected default theme 'system', got %q", state.Theme)
	}
	if !state.SidebarOpen {
		t.Error("expected default SidebarOpen to be true")
	}
	if state.CurrentPage != "chat" {
		t.Errorf("expected default CurrentPage 'chat', got %q", state.CurrentPage)
	}
}

func TestNewStateManager_NilHub(t *testing.T) {
	sm := NewStateManager(nil)

	if sm == nil {
		t.Fatal("NewStateManager returned nil with nil hub")
	}

	// Should not panic when updating
	sm.Update(UIState{Theme: "dark"})
}

func TestStateManager_Get_DeepCopy(t *testing.T) {
	hub := newMockBroadcaster()
	sm := NewStateManager(hub)

	// Modify state
	sm.Update(UIState{
		ActiveSession: "sess-1",
		Custom:        map[string]any{"key": "value"},
	})

	// Get a copy
	state1 := sm.Get()
	state2 := sm.Get()

	// Modify the copy's custom map
	state1.Custom["key"] = "modified"

	// Original should be unchanged
	state3 := sm.Get()
	if state3.Custom["key"] != "value" {
		t.Errorf("Get() should return deep copy, but modification affected original")
	}

	// Second copy should also be unchanged
	if state2.Custom["key"] != "value" {
		t.Errorf("Get() should return independent copies")
	}
}

func TestStateManager_Update_MergesFields(t *testing.T) {
	hub := newMockBroadcaster()
	sm := NewStateManager(hub)

	// Set initial state
	sm.Update(UIState{
		ActiveSession: "sess-1",
		Theme:         "dark",
		CurrentPage:   "settings",
	})

	// Partial update
	sm.Update(UIState{
		Theme: "light",
	})

	state := sm.Get()

	// Theme should be updated
	if state.Theme != "light" {
		t.Errorf("Theme should be 'light', got %q", state.Theme)
	}

	// Other fields should be preserved
	if state.ActiveSession != "sess-1" {
		t.Errorf("ActiveSession should be preserved, got %q", state.ActiveSession)
	}
	if state.CurrentPage != "settings" {
		t.Errorf("CurrentPage should be preserved, got %q", state.CurrentPage)
	}
}

func TestStateManager_Update_CustomFields(t *testing.T) {
	hub := newMockBroadcaster()
	sm := NewStateManager(hub)

	// Set custom fields
	sm.Update(UIState{
		Custom: map[string]any{
			"key1": "value1",
			"key2": 42,
		},
	})

	// Add more custom fields
	sm.Update(UIState{
		Custom: map[string]any{
			"key3": true,
		},
	})

	state := sm.Get()

	// All custom fields should be present
	if state.Custom["key1"] != "value1" {
		t.Errorf("Custom key1 mismatch")
	}
	if state.Custom["key2"] != 42 {
		t.Errorf("Custom key2 mismatch")
	}
	if state.Custom["key3"] != true {
		t.Errorf("Custom key3 mismatch")
	}
}

func TestStateManager_Update_BroadcastsCalled(t *testing.T) {
	hub := newMockBroadcaster()
	sm := NewStateManager(hub)

	sm.Update(UIState{Theme: "dark"})

	if hub.getCallCount() != 1 {
		t.Errorf("expected 1 broadcast call, got %d", hub.getCallCount())
	}

	// Verify broadcast message format
	msg := hub.getLastMessage()
	if msg == nil {
		t.Fatal("no message was broadcast")
	}

	var parsed struct {
		Type  string          `json:"type"`
		State json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(msg, &parsed); err != nil {
		t.Fatalf("failed to parse broadcast message: %v", err)
	}

	if parsed.Type != "ui_state" {
		t.Errorf("expected type 'ui_state', got %q", parsed.Type)
	}

	var state UIState
	if err := json.Unmarshal(parsed.State, &state); err != nil {
		t.Fatalf("failed to parse state: %v", err)
	}

	if state.Theme != "dark" {
		t.Errorf("broadcast state theme mismatch: got %q", state.Theme)
	}
}

func TestStateManager_UpdateSidebarOpen(t *testing.T) {
	hub := newMockBroadcaster()
	sm := NewStateManager(hub)

	// Default is true
	state := sm.Get()
	if !state.SidebarOpen {
		t.Error("default SidebarOpen should be true")
	}

	// Close sidebar
	sm.UpdateSidebarOpen(false)

	state = sm.Get()
	if state.SidebarOpen {
		t.Error("SidebarOpen should be false after UpdateSidebarOpen(false)")
	}

	// Reopen sidebar
	sm.UpdateSidebarOpen(true)

	state = sm.Get()
	if !state.SidebarOpen {
		t.Error("SidebarOpen should be true after UpdateSidebarOpen(true)")
	}
}

func TestStateManager_SetActiveSession(t *testing.T) {
	hub := newMockBroadcaster()
	sm := NewStateManager(hub)

	sm.SetActiveSession("session-123")

	state := sm.Get()
	if state.ActiveSession != "session-123" {
		t.Errorf("ActiveSession mismatch: got %q", state.ActiveSession)
	}
}

func TestStateManager_SetTheme(t *testing.T) {
	hub := newMockBroadcaster()
	sm := NewStateManager(hub)

	sm.SetTheme("dark")

	state := sm.Get()
	if state.Theme != "dark" {
		t.Errorf("Theme mismatch: got %q", state.Theme)
	}
}

func TestStateManager_SetCurrentPage(t *testing.T) {
	hub := newMockBroadcaster()
	sm := NewStateManager(hub)

	sm.SetCurrentPage("settings")

	state := sm.Get()
	if state.CurrentPage != "settings" {
		t.Errorf("CurrentPage mismatch: got %q", state.CurrentPage)
	}
}

func TestStateManager_SetCustom(t *testing.T) {
	hub := newMockBroadcaster()
	sm := NewStateManager(hub)

	sm.SetCustom("myKey", "myValue")

	state := sm.Get()
	if state.Custom["myKey"] != "myValue" {
		t.Errorf("Custom field mismatch: got %v", state.Custom["myKey"])
	}
}

func TestStateManager_ConcurrentAccess(t *testing.T) {
	hub := newMockBroadcaster()
	sm := NewStateManager(hub)

	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	// Concurrent updates
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sm.Update(UIState{
				Custom: map[string]any{
					"key": i,
				},
			})
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sm.Get()
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent error: %v", err)
	}

	// Should complete without panics or race conditions
}
