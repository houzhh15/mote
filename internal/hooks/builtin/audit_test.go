package builtin

import (
	"context"
	"testing"
	"time"

	"mote/internal/hooks"
)

func TestAuditHook_Handler(t *testing.T) {
	store := NewMemoryAuditStore(100)
	hook := NewAuditHook(AuditConfig{
		Store: store,
	})

	handler := hook.Handler("test-audit")
	if handler == nil {
		t.Fatal("expected handler to be created")
	}

	if handler.ID != "test-audit" {
		t.Errorf("expected ID 'test-audit', got '%s'", handler.ID)
	}

	if handler.Priority != 50 {
		t.Errorf("expected priority 50, got %d", handler.Priority)
	}
}

func TestAuditHook_AuditToolCall(t *testing.T) {
	store := NewMemoryAuditStore(100)
	hook := NewAuditHook(AuditConfig{
		Store:         store,
		IncludeParams: true,
	})

	hookCtx := hooks.NewContext(hooks.HookAfterToolCall)
	hookCtx.ToolCall = &hooks.ToolCallContext{
		ID:       "tool-123",
		ToolName: "test_tool",
		Params:   map[string]any{"arg": "value"},
		Duration: 100 * time.Millisecond,
	}

	handler := hook.Handler("test-audit")
	result, rErr := handler.Handler(context.Background(), hookCtx)
	if rErr != nil {
		t.Fatalf("unexpected error: %v", rErr)
	}

	if result == nil {
		t.Fatal("expected result to be returned")
	}

	if !result.Continue {
		t.Error("expected Continue to be true")
	}

	records := store.GetRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	record := records[0]
	if record.ToolName != "test_tool" {
		t.Errorf("expected tool name 'test_tool', got '%s'", record.ToolName)
	}

	if record.ToolID != "tool-123" {
		t.Errorf("expected tool ID 'tool-123', got '%s'", record.ToolID)
	}

	if record.Duration != 100*time.Millisecond {
		t.Errorf("expected duration 100ms, got %v", record.Duration)
	}

	if !record.Success {
		t.Error("expected success to be true")
	}
}

func TestAuditHook_AuditToolCallWithError(t *testing.T) {
	store := NewMemoryAuditStore(100)
	hook := NewAuditHook(AuditConfig{
		Store: store,
	})

	hookCtx := hooks.NewContext(hooks.HookAfterToolCall)
	hookCtx.ToolCall = &hooks.ToolCallContext{
		ID:       "tool-123",
		ToolName: "test_tool",
		Error:    "something went wrong",
	}

	handler := hook.Handler("test-audit")
	_, _ = handler.Handler(context.Background(), hookCtx)

	records := store.GetRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	record := records[0]
	if record.Success {
		t.Error("expected success to be false")
	}

	if record.Error != "something went wrong" {
		t.Errorf("expected error message, got '%s'", record.Error)
	}
}

func TestAuditHook_AuditSession(t *testing.T) {
	store := NewMemoryAuditStore(100)
	hook := NewAuditHook(AuditConfig{
		Store: store,
	})

	hookCtx := hooks.NewContext(hooks.HookSessionCreate)
	hookCtx.Session = &hooks.SessionContext{
		ID: "session-123",
	}

	handler := hook.Handler("test-audit")
	_, _ = handler.Handler(context.Background(), hookCtx)

	records := store.GetRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	record := records[0]
	if record.SessionID != "session-123" {
		t.Errorf("expected session ID 'session-123', got '%s'", record.SessionID)
	}
}

func TestMemoryAuditStore_MaxSize(t *testing.T) {
	store := NewMemoryAuditStore(3)

	for i := 0; i < 5; i++ {
		_ = store.Store(&AuditRecord{ID: string(rune('a' + i))})
	}

	records := store.GetRecords()
	if len(records) != 3 {
		t.Errorf("expected 3 records, got %d", len(records))
	}

	// Should have records c, d, e (oldest evicted)
	if records[0].ID != "c" {
		t.Errorf("expected first record 'c', got '%s'", records[0].ID)
	}
}

func TestMemoryAuditStore_Clear(t *testing.T) {
	store := NewMemoryAuditStore(100)
	_ = store.Store(&AuditRecord{ID: "test"})

	store.Clear()

	records := store.GetRecords()
	if len(records) != 0 {
		t.Errorf("expected 0 records after clear, got %d", len(records))
	}
}

func TestLogAuditStore_Store(t *testing.T) {
	store := NewLogAuditStore(nil)

	err := store.Store(&AuditRecord{
		ID:        "test-123",
		Timestamp: time.Now(),
		HookType:  hooks.HookAfterToolCall,
		ToolName:  "test_tool",
		Success:   true,
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestLogAuditStore_Close(t *testing.T) {
	store := NewLogAuditStore(nil)

	err := store.Close()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRegisterAuditHooks(t *testing.T) {
	manager := hooks.NewManager()
	store := NewMemoryAuditStore(100)

	err := RegisterAuditHooks(manager, AuditConfig{
		Store: store,
	})
	if err != nil {
		t.Fatalf("failed to register audit hooks: %v", err)
	}

	// Check that handlers are registered for expected hook types
	expectedTypes := []hooks.HookType{
		hooks.HookBeforeToolCall,
		hooks.HookAfterToolCall,
		hooks.HookSessionCreate,
		hooks.HookSessionEnd,
	}

	for _, hookType := range expectedTypes {
		if !manager.HasHandlers(hookType) {
			t.Errorf("expected handler registered for %s", hookType)
		}
	}
}

func TestHashParams(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
	}{
		{
			name:   "small params",
			params: map[string]any{"key": "value"},
		},
		{
			name:   "empty params",
			params: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hashParams(tt.params)
			if result == "" {
				t.Error("expected non-empty hash")
			}
		})
	}
}
