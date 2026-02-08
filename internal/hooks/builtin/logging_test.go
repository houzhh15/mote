package builtin

import (
	"context"
	"testing"

	"mote/internal/hooks"

	"github.com/rs/zerolog"
)

func TestLoggingHook_Handler(t *testing.T) {
	cfg := LoggingConfig{
		Level: zerolog.DebugLevel,
	}
	hook := NewLoggingHook(cfg)

	handler := hook.Handler("test-logging")
	if handler == nil {
		t.Fatal("expected handler to be created")
	}

	if handler.ID != "test-logging" {
		t.Errorf("expected ID 'test-logging', got '%s'", handler.ID)
	}

	if handler.Source != "_builtin" {
		t.Errorf("expected source '_builtin', got '%s'", handler.Source)
	}

	if handler.Priority != 100 {
		t.Errorf("expected priority 100, got %d", handler.Priority)
	}
}

func TestLoggingHook_Handle(t *testing.T) {
	cfg := LoggingConfig{
		Level: zerolog.DebugLevel,
	}
	hook := NewLoggingHook(cfg)

	tests := []struct {
		name     string
		hookType hooks.HookType
		setup    func(ctx *hooks.Context)
	}{
		{
			name:     "before_message",
			hookType: hooks.HookBeforeMessage,
			setup: func(ctx *hooks.Context) {
				ctx.Message = &hooks.MessageContext{
					Content: "test message",
					Role:    "user",
					From:    "test-user",
				}
			},
		},
		{
			name:     "before_tool_call",
			hookType: hooks.HookBeforeToolCall,
			setup: func(ctx *hooks.Context) {
				ctx.ToolCall = &hooks.ToolCallContext{
					ID:       "tool-123",
					ToolName: "test_tool",
					Params:   map[string]any{"arg": "value"},
				}
			},
		},
		{
			name:     "session_create",
			hookType: hooks.HookSessionCreate,
			setup: func(ctx *hooks.Context) {
				ctx.Session = &hooks.SessionContext{
					ID: "session-123",
				}
			},
		},
		{
			name:     "before_response",
			hookType: hooks.HookBeforeResponse,
			setup: func(ctx *hooks.Context) {
				ctx.Response = &hooks.ResponseContext{
					Content:    "response content",
					TokensUsed: 100,
					Model:      "test-model",
				}
			},
		},
		{
			name:     "startup",
			hookType: hooks.HookStartup,
			setup:    func(ctx *hooks.Context) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hookCtx := hooks.NewContext(tt.hookType)
			tt.setup(hookCtx)

			handler := hook.Handler("test-logging")
			result, err := handler.Handler(context.Background(), hookCtx)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result == nil {
				t.Fatal("expected result to be returned")
			}
			if !result.Continue {
				t.Error("expected Continue to be true")
			}
		})
	}
}

func TestRegisterLoggingHooks(t *testing.T) {
	manager := hooks.NewManager()

	err := RegisterLoggingHooks(manager, LoggingConfig{
		Level: zerolog.DebugLevel,
	})
	if err != nil {
		t.Fatalf("failed to register logging hooks: %v", err)
	}

	// Check that handlers are registered for all hook types
	for _, hookType := range hooks.AllHookTypes() {
		if !manager.HasHandlers(hookType) {
			t.Errorf("expected handler registered for %s", hookType)
		}
	}
}

func TestNewLoggingStats(t *testing.T) {
	stats := NewLoggingStats()
	if stats == nil {
		t.Fatal("expected stats to be created")
	}
	if stats.EventsByType == nil {
		t.Error("expected EventsByType to be initialized")
	}
}
