package hooks

import (
	"context"
	"testing"
	"time"
)

func TestManager_RegisterAndTrigger(t *testing.T) {
	m := NewManager()

	called := false
	handler := &Handler{
		ID:      "test",
		Enabled: true,
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			called = true
			return ContinueResult(), nil
		},
	}

	err := m.Register(HookBeforeMessage, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	_, err = m.Trigger(ctx, hookCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !called {
		t.Error("handler was not called")
	}
}

func TestManager_Unregister(t *testing.T) {
	m := NewManager()

	handler := &Handler{
		ID:      "test",
		Enabled: true,
	}

	_ = m.Register(HookBeforeMessage, handler)

	err := m.Unregister(HookBeforeMessage, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.HasHandlers(HookBeforeMessage) {
		t.Error("handler should have been unregistered")
	}
}

func TestManager_TriggerInvalidHookType(t *testing.T) {
	m := NewManager()

	ctx := context.Background()
	hookCtx := &Context{Type: HookType("invalid")}

	_, err := m.Trigger(ctx, hookCtx)
	if err == nil {
		t.Error("expected error for invalid hook type")
	}
}

func TestManager_TriggerNilContext(t *testing.T) {
	m := NewManager()

	ctx := context.Background()
	result, err := m.Trigger(ctx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true")
	}
}

func TestManager_TriggerNoHandlers(t *testing.T) {
	m := NewManager()

	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	result, err := m.Trigger(ctx, hookCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Continue {
		t.Error("expected Continue=true for no handlers")
	}
}

func TestManager_ListHandlers(t *testing.T) {
	m := NewManager()

	handler := &Handler{ID: "test", Enabled: true}
	_ = m.Register(HookBeforeMessage, handler)

	handlers := m.ListHandlers(HookBeforeMessage)
	if len(handlers) != 1 {
		t.Errorf("expected 1 handler, got %d", len(handlers))
	}
}

func TestManager_HasHandlers(t *testing.T) {
	m := NewManager()

	if m.HasHandlers(HookBeforeMessage) {
		t.Error("expected no handlers initially")
	}

	handler := &Handler{ID: "test", Enabled: true}
	_ = m.Register(HookBeforeMessage, handler)

	if !m.HasHandlers(HookBeforeMessage) {
		t.Error("expected handlers after registration")
	}
}

func TestManager_Clear(t *testing.T) {
	m := NewManager()

	handler := &Handler{ID: "test", Enabled: true}
	_ = m.Register(HookBeforeMessage, handler)

	m.Clear()

	if m.HasHandlers(HookBeforeMessage) {
		t.Error("expected no handlers after clear")
	}
}

func TestManager_TriggerBeforeMessage(t *testing.T) {
	m := NewManager()

	var capturedCtx *Context
	handler := &Handler{
		ID:      "test",
		Enabled: true,
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			capturedCtx = hookCtx
			return ContinueResult(), nil
		},
	}
	_ = m.Register(HookBeforeMessage, handler)

	_, _ = m.TriggerBeforeMessage(context.Background(), "test content", "user", "user123")

	if capturedCtx == nil {
		t.Fatal("context was not captured")
	}
	if capturedCtx.Type != HookBeforeMessage {
		t.Errorf("expected type %s, got %s", HookBeforeMessage, capturedCtx.Type)
	}
	if capturedCtx.Message.Content != "test content" {
		t.Errorf("expected content 'test content', got '%s'", capturedCtx.Message.Content)
	}
	if capturedCtx.Message.Role != "user" {
		t.Errorf("expected role 'user', got '%s'", capturedCtx.Message.Role)
	}
	if capturedCtx.Message.From != "user123" {
		t.Errorf("expected from 'user123', got '%s'", capturedCtx.Message.From)
	}
}

func TestManager_TriggerBeforeToolCall(t *testing.T) {
	m := NewManager()

	var capturedCtx *Context
	handler := &Handler{
		ID:      "test",
		Enabled: true,
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			capturedCtx = hookCtx
			return ContinueResult(), nil
		},
	}
	_ = m.Register(HookBeforeToolCall, handler)

	params := map[string]any{"arg1": "value1"}
	_, _ = m.TriggerBeforeToolCall(context.Background(), "tool-id-1", "test_tool", params)

	if capturedCtx == nil {
		t.Fatal("context was not captured")
	}
	if capturedCtx.Type != HookBeforeToolCall {
		t.Errorf("expected type %s, got %s", HookBeforeToolCall, capturedCtx.Type)
	}
	if capturedCtx.ToolCall.ID != "tool-id-1" {
		t.Errorf("expected ID 'tool-id-1', got '%s'", capturedCtx.ToolCall.ID)
	}
	if capturedCtx.ToolCall.ToolName != "test_tool" {
		t.Errorf("expected tool name 'test_tool', got '%s'", capturedCtx.ToolCall.ToolName)
	}
}

func TestManager_TriggerAfterToolCall(t *testing.T) {
	m := NewManager()

	var capturedCtx *Context
	handler := &Handler{
		ID:      "test",
		Enabled: true,
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			capturedCtx = hookCtx
			return ContinueResult(), nil
		},
	}
	_ = m.Register(HookAfterToolCall, handler)

	params := map[string]any{"arg1": "value1"}
	_, _ = m.TriggerAfterToolCall(context.Background(), "tool-id-1", "test_tool", params, "result", "", 100*time.Millisecond)

	if capturedCtx == nil {
		t.Fatal("context was not captured")
	}
	if capturedCtx.ToolCall.Result != "result" {
		t.Errorf("expected result 'result', got '%v'", capturedCtx.ToolCall.Result)
	}
	if capturedCtx.ToolCall.Duration != 100*time.Millisecond {
		t.Errorf("expected duration 100ms, got %v", capturedCtx.ToolCall.Duration)
	}
}

func TestManager_TriggerSessionCreate(t *testing.T) {
	m := NewManager()

	var capturedCtx *Context
	handler := &Handler{
		ID:      "test",
		Enabled: true,
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			capturedCtx = hookCtx
			return ContinueResult(), nil
		},
	}
	_ = m.Register(HookSessionCreate, handler)

	now := time.Now()
	metadata := map[string]any{"key": "value"}
	_, _ = m.TriggerSessionCreate(context.Background(), "session-1", now, metadata)

	if capturedCtx == nil {
		t.Fatal("context was not captured")
	}
	if capturedCtx.Session.ID != "session-1" {
		t.Errorf("expected session ID 'session-1', got '%s'", capturedCtx.Session.ID)
	}
}

func TestManager_TriggerBeforeResponse(t *testing.T) {
	m := NewManager()

	var capturedCtx *Context
	handler := &Handler{
		ID:      "test",
		Enabled: true,
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			capturedCtx = hookCtx
			return ContinueResult(), nil
		},
	}
	_ = m.Register(HookBeforeResponse, handler)

	_, _ = m.TriggerBeforeResponse(context.Background(), "response content", 100, "gpt-4")

	if capturedCtx == nil {
		t.Fatal("context was not captured")
	}
	if capturedCtx.Response.Content != "response content" {
		t.Errorf("expected content 'response content', got '%s'", capturedCtx.Response.Content)
	}
	if capturedCtx.Response.TokensUsed != 100 {
		t.Errorf("expected tokens 100, got %d", capturedCtx.Response.TokensUsed)
	}
	if capturedCtx.Response.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got '%s'", capturedCtx.Response.Model)
	}
}

func TestManager_TriggerStartupShutdown(t *testing.T) {
	m := NewManager()

	startupCalled := false
	shutdownCalled := false

	_ = m.Register(HookStartup, &Handler{
		ID:      "startup",
		Enabled: true,
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			startupCalled = true
			return ContinueResult(), nil
		},
	})

	_ = m.Register(HookShutdown, &Handler{
		ID:      "shutdown",
		Enabled: true,
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			shutdownCalled = true
			return ContinueResult(), nil
		},
	})

	ctx := context.Background()

	_, _ = m.TriggerStartup(ctx)
	if !startupCalled {
		t.Error("startup handler was not called")
	}

	_, _ = m.TriggerShutdown(ctx)
	if !shutdownCalled {
		t.Error("shutdown handler was not called")
	}
}

func TestManager_Close(t *testing.T) {
	m := NewManager()

	handler := &Handler{ID: "test", Enabled: true}
	_ = m.Register(HookBeforeMessage, handler)

	err := m.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.HasHandlers(HookBeforeMessage) {
		t.Error("expected no handlers after close")
	}
}

func TestManager_WithOptions(t *testing.T) {
	customRegistry := NewRegistry()
	customExecutor := NewExecutor()

	m := NewManagerWithOptions(
		WithRegistry(customRegistry),
		WithExecutor(customExecutor),
	)

	if m.registry != customRegistry {
		t.Error("custom registry was not set")
	}
	if m.executor != customExecutor {
		t.Error("custom executor was not set")
	}
}
