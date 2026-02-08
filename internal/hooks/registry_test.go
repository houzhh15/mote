package hooks

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	handler := &Handler{
		ID:       "test-handler",
		Priority: 100,
		Source:   "test",
		Handler:  func(ctx context.Context, hookCtx *Context) (*Result, error) { return ContinueResult(), nil },
		Enabled:  true,
	}

	err := r.Register(HookBeforeMessage, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handlers := r.GetHandlers(HookBeforeMessage)
	if len(handlers) != 1 {
		t.Errorf("expected 1 handler, got %d", len(handlers))
	}
	if handlers[0].ID != "test-handler" {
		t.Errorf("expected ID 'test-handler', got '%s'", handlers[0].ID)
	}
}

func TestRegistry_Register_DuplicateID(t *testing.T) {
	r := NewRegistry()

	handler1 := &Handler{ID: "test-handler", Priority: 100, Enabled: true}
	handler2 := &Handler{ID: "test-handler", Priority: 50, Enabled: true}

	err := r.Register(HookBeforeMessage, handler1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = r.Register(HookBeforeMessage, handler2)
	if err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestRegistry_Register_InvalidHookType(t *testing.T) {
	r := NewRegistry()

	handler := &Handler{ID: "test-handler", Enabled: true}
	err := r.Register(HookType("invalid_type"), handler)
	if err == nil {
		t.Fatal("expected error for invalid hook type")
	}
}

func TestRegistry_Register_EmptyID(t *testing.T) {
	r := NewRegistry()

	handler := &Handler{ID: "", Enabled: true}
	err := r.Register(HookBeforeMessage, handler)
	if err == nil {
		t.Fatal("expected error for empty handler ID")
	}
}

func TestRegistry_Register_NilHandler(t *testing.T) {
	r := NewRegistry()

	err := r.Register(HookBeforeMessage, nil)
	if err == nil {
		t.Fatal("expected error for nil handler")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry()

	handler := &Handler{ID: "test-handler", Priority: 100, Enabled: true}
	r.Register(HookBeforeMessage, handler)

	err := r.Unregister(HookBeforeMessage, "test-handler")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handlers := r.GetHandlers(HookBeforeMessage)
	if len(handlers) != 0 {
		t.Errorf("expected 0 handlers, got %d", len(handlers))
	}
}

func TestRegistry_Unregister_NotFound(t *testing.T) {
	r := NewRegistry()

	err := r.Unregister(HookBeforeMessage, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent handler")
	}
}

func TestRegistry_PriorityOrdering(t *testing.T) {
	r := NewRegistry()

	// Register handlers in non-priority order
	handlers := []*Handler{
		{ID: "low", Priority: 0, Enabled: true},
		{ID: "high", Priority: 100, Enabled: true},
		{ID: "medium", Priority: 50, Enabled: true},
	}

	for _, h := range handlers {
		if err := r.Register(HookBeforeMessage, h); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	result := r.GetHandlers(HookBeforeMessage)
	if len(result) != 3 {
		t.Fatalf("expected 3 handlers, got %d", len(result))
	}

	// Should be sorted by priority descending
	expectedOrder := []string{"high", "medium", "low"}
	for i, h := range result {
		if h.ID != expectedOrder[i] {
			t.Errorf("expected handler[%d] to be '%s', got '%s'", i, expectedOrder[i], h.ID)
		}
	}
}

func TestRegistry_GetHandlers_Empty(t *testing.T) {
	r := NewRegistry()

	handlers := r.GetHandlers(HookBeforeMessage)
	if handlers != nil {
		t.Errorf("expected nil for empty handlers, got %v", handlers)
	}
}

func TestRegistry_GetHandlers_ReturnsCopy(t *testing.T) {
	r := NewRegistry()

	handler := &Handler{ID: "test-handler", Priority: 100, Enabled: true}
	r.Register(HookBeforeMessage, handler)

	handlers1 := r.GetHandlers(HookBeforeMessage)
	handlers2 := r.GetHandlers(HookBeforeMessage)

	// Modify the first slice
	handlers1[0] = &Handler{ID: "modified"}

	// Second slice should be unaffected
	if handlers2[0].ID != "test-handler" {
		t.Error("GetHandlers should return a copy")
	}
}

func TestRegistry_HasHandlers(t *testing.T) {
	r := NewRegistry()

	if r.HasHandlers(HookBeforeMessage) {
		t.Error("expected HasHandlers to return false for empty registry")
	}

	handler := &Handler{ID: "test-handler", Enabled: true}
	r.Register(HookBeforeMessage, handler)

	if !r.HasHandlers(HookBeforeMessage) {
		t.Error("expected HasHandlers to return true after registration")
	}

	if r.HasHandlers(HookAfterMessage) {
		t.Error("expected HasHandlers to return false for different hook type")
	}
}

func TestRegistry_Clear(t *testing.T) {
	r := NewRegistry()

	handler := &Handler{ID: "test-handler", Enabled: true}
	r.Register(HookBeforeMessage, handler)
	r.Register(HookAfterMessage, handler)

	r.Clear()

	if r.HasHandlers(HookBeforeMessage) {
		t.Error("expected no handlers after Clear")
	}
	if r.HasHandlers(HookAfterMessage) {
		t.Error("expected no handlers after Clear")
	}
}

func TestRegistry_Count(t *testing.T) {
	r := NewRegistry()

	if r.Count() != 0 {
		t.Errorf("expected count 0, got %d", r.Count())
	}

	r.Register(HookBeforeMessage, &Handler{ID: "handler1", Enabled: true})
	r.Register(HookAfterMessage, &Handler{ID: "handler2", Enabled: true})
	r.Register(HookBeforeMessage, &Handler{ID: "handler3", Enabled: true})

	if r.Count() != 3 {
		t.Errorf("expected count 3, got %d", r.Count())
	}
}

func TestRegistry_ListTypes(t *testing.T) {
	r := NewRegistry()

	types := r.ListTypes()
	if len(types) != 0 {
		t.Errorf("expected 0 types, got %d", len(types))
	}

	r.Register(HookBeforeMessage, &Handler{ID: "handler1", Enabled: true})
	r.Register(HookAfterMessage, &Handler{ID: "handler2", Enabled: true})

	types = r.ListTypes()
	if len(types) != 2 {
		t.Errorf("expected 2 types, got %d", len(types))
	}
}

func TestRegistry_GetAllHandlers(t *testing.T) {
	r := NewRegistry()

	r.Register(HookBeforeMessage, &Handler{ID: "handler1", Enabled: true})
	r.Register(HookAfterMessage, &Handler{ID: "handler2", Enabled: true})

	all := r.GetAllHandlers()
	if len(all) != 2 {
		t.Errorf("expected 2 hook types, got %d", len(all))
	}
	if len(all[HookBeforeMessage]) != 1 {
		t.Errorf("expected 1 handler for BeforeMessage, got %d", len(all[HookBeforeMessage]))
	}
	if len(all[HookAfterMessage]) != 1 {
		t.Errorf("expected 1 handler for AfterMessage, got %d", len(all[HookAfterMessage]))
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent registrations
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			handler := &Handler{
				ID:       fmt.Sprintf("handler-%d", id),
				Priority: id,
				Enabled:  true,
			}
			// Ignore errors since we expect some duplicates
			r.Register(HookBeforeMessage, handler)
		}(i)
	}
	wg.Wait()

	// Verify all handlers were registered
	handlers := r.GetHandlers(HookBeforeMessage)
	if len(handlers) != numGoroutines {
		t.Errorf("expected %d handlers, got %d", numGoroutines, len(handlers))
	}

	// Concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			r.GetHandlers(HookBeforeMessage)
			r.HasHandlers(HookBeforeMessage)
			r.Count()
		}()
	}
	wg.Wait()

	// Concurrent unregistrations
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			r.Unregister(HookBeforeMessage, fmt.Sprintf("handler-%d", id))
		}(i)
	}
	wg.Wait()

	// Verify all handlers were unregistered
	handlers = r.GetHandlers(HookBeforeMessage)
	if len(handlers) != 0 {
		t.Errorf("expected 0 handlers after unregistration, got %d", len(handlers))
	}
}
