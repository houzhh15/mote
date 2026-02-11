package builtin

import (
	"context"
	"testing"

	"mote/internal/hooks"

	"github.com/rs/zerolog"
)

func TestNewMemoryHookBridge(t *testing.T) {
	bridge := NewMemoryHookBridge(MemoryHookConfig{
		Logger: zerolog.Nop(),
	})

	if bridge == nil {
		t.Fatal("bridge is nil")
	}
}

func TestMemoryHookBridge_BeforeMessageHandler(t *testing.T) {
	bridge := NewMemoryHookBridge(MemoryHookConfig{
		Logger: zerolog.Nop(),
	})

	handler := bridge.BeforeMessageHandler("test-before")

	if handler == nil {
		t.Fatal("handler is nil")
	}
	if handler.ID != "test-before" {
		t.Errorf("expected ID 'test-before', got %s", handler.ID)
	}
	if handler.Source != "_builtin" {
		t.Errorf("expected source '_builtin', got %s", handler.Source)
	}
}

func TestMemoryHookBridge_AfterMessageHandler(t *testing.T) {
	bridge := NewMemoryHookBridge(MemoryHookConfig{
		Logger: zerolog.Nop(),
	})

	handler := bridge.AfterMessageHandler("test-after")

	if handler == nil {
		t.Fatal("handler is nil")
	}
	if handler.ID != "test-after" {
		t.Errorf("expected ID 'test-after', got %s", handler.ID)
	}
}

func TestMemoryHookBridge_SessionCreateHandler(t *testing.T) {
	bridge := NewMemoryHookBridge(MemoryHookConfig{
		Logger: zerolog.Nop(),
	})

	handler := bridge.SessionCreateHandler("test-session")

	if handler == nil {
		t.Fatal("handler is nil")
	}
	if handler.ID != "test-session" {
		t.Errorf("expected ID 'test-session', got %s", handler.ID)
	}
}

func TestMemoryHookBridge_HandleBeforeMessage_NilRecallEngine(t *testing.T) {
	bridge := NewMemoryHookBridge(MemoryHookConfig{
		Logger: zerolog.Nop(),
	})

	ctx := context.Background()
	hookCtx := &hooks.Context{
		Type: hooks.HookBeforeMessage,
		Message: &hooks.MessageContext{
			Role:    "user",
			Content: "Tell me about user preferences",
		},
	}

	handler := bridge.BeforeMessageHandler("test")
	result, err := handler.Handler(ctx, hookCtx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for nil recall engine")
	}
}

func TestMemoryHookBridge_HandleBeforeMessage_SystemMessage(t *testing.T) {
	bridge := NewMemoryHookBridge(MemoryHookConfig{
		Logger: zerolog.Nop(),
	})

	ctx := context.Background()
	hookCtx := &hooks.Context{
		Type: hooks.HookBeforeMessage,
		Message: &hooks.MessageContext{
			Role:    "system",
			Content: "You are a helpful assistant",
		},
	}

	handler := bridge.BeforeMessageHandler("test")
	result, err := handler.Handler(ctx, hookCtx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for system message")
	}
}

func TestMemoryHookBridge_HandleAfterMessage_NilCaptureEngine(t *testing.T) {
	bridge := NewMemoryHookBridge(MemoryHookConfig{
		Logger: zerolog.Nop(),
	})

	ctx := context.Background()
	hookCtx := &hooks.Context{
		Type: hooks.HookAfterMessage,
		Message: &hooks.MessageContext{
			Role:    "user",
			Content: "Remember my preference for dark mode",
		},
	}

	handler := bridge.AfterMessageHandler("test")
	result, err := handler.Handler(ctx, hookCtx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for nil capture engine")
	}
}

func TestMemoryHookBridge_HandleSessionCreate(t *testing.T) {
	bridge := NewMemoryHookBridge(MemoryHookConfig{
		Logger: zerolog.Nop(),
	})

	ctx := context.Background()
	hookCtx := &hooks.Context{
		Type: hooks.HookSessionCreate,
		Session: &hooks.SessionContext{
			ID: "session-123",
		},
	}

	handler := bridge.SessionCreateHandler("test")
	result, err := handler.Handler(ctx, hookCtx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result")
	}
}
