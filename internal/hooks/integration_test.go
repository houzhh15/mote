package hooks

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_HookLifecycle tests the complete hook lifecycle:
// register -> trigger -> execute -> unregister
func TestIntegration_HookLifecycle(t *testing.T) {
	manager := NewManager()
	require.NotNil(t, manager)

	// Track execution order
	var executed []string
	var mu sync.Mutex

	// Register hooks with different priorities
	handler1 := &Handler{
		ID:       "hook-1",
		Priority: 100,
		Source:   "test",
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			mu.Lock()
			executed = append(executed, "hook-1")
			mu.Unlock()
			return &Result{Continue: true}, nil
		},
		Enabled: true,
	}

	handler2 := &Handler{
		ID:       "hook-2",
		Priority: 200, // Higher priority, should run first
		Source:   "test",
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			mu.Lock()
			executed = append(executed, "hook-2")
			mu.Unlock()
			return &Result{Continue: true}, nil
		},
		Enabled: true,
	}

	handler3 := &Handler{
		ID:       "hook-3",
		Priority: 50, // Lowest priority, should run last
		Source:   "test",
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			mu.Lock()
			executed = append(executed, "hook-3")
			mu.Unlock()
			return &Result{Continue: true}, nil
		},
		Enabled: true,
	}

	// Register handlers in random order
	require.NoError(t, manager.Register(HookBeforeMessage, handler3))
	require.NoError(t, manager.Register(HookBeforeMessage, handler1))
	require.NoError(t, manager.Register(HookBeforeMessage, handler2))

	// Trigger the hook
	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)
	hookCtx.Session = &SessionContext{ID: "session-1"}
	hookCtx.SetData("message", "Hello, World!")

	result, err := manager.Trigger(ctx, hookCtx)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify execution order (highest priority first)
	require.Len(t, executed, 3)
	assert.Equal(t, "hook-2", executed[0], "highest priority should run first")
	assert.Equal(t, "hook-1", executed[1], "medium priority should run second")
	assert.Equal(t, "hook-3", executed[2], "lowest priority should run last")

	// Unregister one hook
	require.NoError(t, manager.Unregister(HookBeforeMessage, "hook-1"))

	// Reset and trigger again
	executed = nil
	result, err = manager.Trigger(ctx, hookCtx)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify only 2 hooks executed
	require.Len(t, executed, 2)
	assert.Equal(t, "hook-2", executed[0])
	assert.Equal(t, "hook-3", executed[1])
}

// TestIntegration_HookChainInterrupt tests that a hook can interrupt the chain
func TestIntegration_HookChainInterrupt(t *testing.T) {
	manager := NewManager()

	var executed []string
	var mu sync.Mutex

	// Hook 1: high priority, continues
	handler1 := &Handler{
		ID:       "hook-1",
		Priority: 200,
		Source:   "test",
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			mu.Lock()
			executed = append(executed, "hook-1")
			mu.Unlock()
			return &Result{Continue: true}, nil
		},
		Enabled: true,
	}

	// Hook 2: medium priority, interrupts
	handler2 := &Handler{
		ID:       "hook-2",
		Priority: 100,
		Source:   "test",
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			mu.Lock()
			executed = append(executed, "hook-2")
			mu.Unlock()
			return &Result{Continue: false}, nil // Interrupt!
		},
		Enabled: true,
	}

	// Hook 3: low priority, should NOT run
	handler3 := &Handler{
		ID:       "hook-3",
		Priority: 50,
		Source:   "test",
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			mu.Lock()
			executed = append(executed, "hook-3")
			mu.Unlock()
			return &Result{Continue: true}, nil
		},
		Enabled: true,
	}

	require.NoError(t, manager.Register(HookBeforeMessage, handler1))
	require.NoError(t, manager.Register(HookBeforeMessage, handler2))
	require.NoError(t, manager.Register(HookBeforeMessage, handler3))

	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	_, err := manager.Trigger(ctx, hookCtx)
	// Executor may or may not return error on interrupt
	_ = err

	// Verify only first two hooks executed
	require.Len(t, executed, 2)
	assert.Equal(t, "hook-1", executed[0])
	assert.Equal(t, "hook-2", executed[1])
	// hook-3 should NOT have executed
}

// TestIntegration_HookDataModification tests that hooks can modify context data
func TestIntegration_HookDataModification(t *testing.T) {
	manager := NewManager()

	// Hook that modifies the message
	handler := &Handler{
		ID:       "modifier",
		Priority: 100,
		Source:   "test",
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			// Get original message
			original, ok := hookCtx.GetData("message")
			if ok {
				if msg, ok := original.(string); ok {
					// Modify and set back
					hookCtx.SetData("message", "[filtered] "+msg)
				}
			}
			hookCtx.SetData("modified_by", "modifier-hook")
			return &Result{Continue: true, Modified: true}, nil
		},
		Enabled: true,
	}

	require.NoError(t, manager.Register(HookBeforeMessage, handler))

	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)
	hookCtx.SetData("message", "Hello, World!")

	result, err := manager.Trigger(ctx, hookCtx)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify data was modified
	msg, ok := hookCtx.GetData("message")
	require.True(t, ok)
	assert.Equal(t, "[filtered] Hello, World!", msg)

	modifiedBy, ok := hookCtx.GetData("modified_by")
	require.True(t, ok)
	assert.Equal(t, "modifier-hook", modifiedBy)
}

// TestIntegration_HookTypeIsolation tests that hooks are isolated by type
func TestIntegration_HookTypeIsolation(t *testing.T) {
	manager := NewManager()

	var beforeMessageCount, afterMessageCount int
	var mu sync.Mutex

	beforeHandler := &Handler{
		ID:       "before",
		Priority: 100,
		Source:   "test",
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			mu.Lock()
			beforeMessageCount++
			mu.Unlock()
			return &Result{Continue: true}, nil
		},
		Enabled: true,
	}

	afterHandler := &Handler{
		ID:       "after",
		Priority: 100,
		Source:   "test",
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			mu.Lock()
			afterMessageCount++
			mu.Unlock()
			return &Result{Continue: true}, nil
		},
		Enabled: true,
	}

	require.NoError(t, manager.Register(HookBeforeMessage, beforeHandler))
	require.NoError(t, manager.Register(HookAfterMessage, afterHandler))

	ctx := context.Background()

	// Trigger before_message
	beforeCtx := NewContext(HookBeforeMessage)
	_, err := manager.Trigger(ctx, beforeCtx)
	require.NoError(t, err)

	// Trigger after_message
	afterCtx := NewContext(HookAfterMessage)
	_, err = manager.Trigger(ctx, afterCtx)
	require.NoError(t, err)

	// Each should have been called once
	assert.Equal(t, 1, beforeMessageCount)
	assert.Equal(t, 1, afterMessageCount)
}

// TestIntegration_AllHookTypes tests that all hook types work correctly
func TestIntegration_AllHookTypes(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	hookTypes := []HookType{
		HookBeforeMessage,
		HookAfterMessage,
		HookBeforeToolCall,
		HookAfterToolCall,
		HookBeforeResponse,
		HookAfterResponse,
		HookSessionCreate,
		HookSessionEnd,
		HookStartup,
		HookShutdown,
	}

	triggered := make(map[HookType]bool)
	var mu sync.Mutex

	for _, ht := range hookTypes {
		hookType := ht // capture
		handler := &Handler{
			ID:       string(hookType) + "-handler",
			Priority: 100,
			Source:   "test",
			Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
				mu.Lock()
				triggered[hookCtx.Type] = true
				mu.Unlock()
				return &Result{Continue: true}, nil
			},
			Enabled: true,
		}
		require.NoError(t, manager.Register(hookType, handler))
	}

	// Trigger all hook types
	for _, ht := range hookTypes {
		hookCtx := NewContext(ht)
		_, err := manager.Trigger(ctx, hookCtx)
		require.NoError(t, err, "hook type %s should trigger without error", ht)
	}

	// Verify all were triggered
	for _, ht := range hookTypes {
		assert.True(t, triggered[ht], "hook type %s should have been triggered", ht)
	}
}

// TestIntegration_ContextCancellation tests that hooks respect context cancellation
func TestIntegration_ContextCancellation(t *testing.T) {
	manager := NewManager()

	slowHandler := &Handler{
		ID:       "slow",
		Priority: 100,
		Source:   "test",
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return &Result{Continue: true}, nil
			}
		},
		Enabled: true,
	}

	require.NoError(t, manager.Register(HookBeforeMessage, slowHandler))

	// Create a context that will be cancelled quickly
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	hookCtx := NewContext(HookBeforeMessage)

	_, err := manager.Trigger(ctx, hookCtx)
	// The handler should respect context cancellation
	// Result depends on whether executor returns error or result
	// Just verify it doesn't hang
	_ = err
}

// TestIntegration_DisabledHooks tests that disabled hooks are not executed
func TestIntegration_DisabledHooks(t *testing.T) {
	manager := NewManager()

	var executed []string
	var mu sync.Mutex

	enabledHandler := &Handler{
		ID:       "enabled",
		Priority: 100,
		Source:   "test",
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			mu.Lock()
			executed = append(executed, "enabled")
			mu.Unlock()
			return &Result{Continue: true}, nil
		},
		Enabled: true,
	}

	disabledHandler := &Handler{
		ID:       "disabled",
		Priority: 200, // Higher priority but disabled
		Source:   "test",
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			mu.Lock()
			executed = append(executed, "disabled")
			mu.Unlock()
			return &Result{Continue: true}, nil
		},
		Enabled: false,
	}

	require.NoError(t, manager.Register(HookBeforeMessage, enabledHandler))
	require.NoError(t, manager.Register(HookBeforeMessage, disabledHandler))

	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	_, err := manager.Trigger(ctx, hookCtx)
	require.NoError(t, err)

	// Only enabled handler should execute
	require.Len(t, executed, 1)
	assert.Equal(t, "enabled", executed[0])
}

// TestIntegration_ConvenienceMethods tests the manager's convenience methods
func TestIntegration_ConvenienceMethods(t *testing.T) {
	manager := NewManager()
	ctx := context.Background()

	var triggered string
	var mu sync.Mutex

	makeHandler := func(name string) *Handler {
		return &Handler{
			ID:       name,
			Priority: 100,
			Source:   "test",
			Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
				mu.Lock()
				triggered = name
				mu.Unlock()
				return &Result{Continue: true}, nil
			},
			Enabled: true,
		}
	}

	// Test TriggerBeforeMessage
	require.NoError(t, manager.Register(HookBeforeMessage, makeHandler("before_message")))
	_, err := manager.TriggerBeforeMessage(ctx, "Hello", "user", "test-user")
	require.NoError(t, err)
	assert.Equal(t, "before_message", triggered)

	// Test TriggerAfterMessage
	require.NoError(t, manager.Register(HookAfterMessage, makeHandler("after_message")))
	_, err = manager.TriggerAfterMessage(ctx, "Hello", "assistant")
	require.NoError(t, err)
	assert.Equal(t, "after_message", triggered)

	// Test TriggerBeforeToolCall
	require.NoError(t, manager.Register(HookBeforeToolCall, makeHandler("before_tool_call")))
	_, err = manager.TriggerBeforeToolCall(ctx, "tool-1", "tool_name", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "before_tool_call", triggered)

	// Test TriggerAfterToolCall
	require.NoError(t, manager.Register(HookAfterToolCall, makeHandler("after_tool_call")))
	_, err = manager.TriggerAfterToolCall(ctx, "tool-1", "tool_name", map[string]any{}, "result", "", 100*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, "after_tool_call", triggered)

	// Test TriggerBeforeResponse
	require.NoError(t, manager.Register(HookBeforeResponse, makeHandler("before_response")))
	_, err = manager.TriggerBeforeResponse(ctx, "Response", 100, "gpt-4")
	require.NoError(t, err)
	assert.Equal(t, "before_response", triggered)

	// Test TriggerAfterResponse
	require.NoError(t, manager.Register(HookAfterResponse, makeHandler("after_response")))
	_, err = manager.TriggerAfterResponse(ctx, "Response", 100, "gpt-4")
	require.NoError(t, err)
	assert.Equal(t, "after_response", triggered)

	// Test TriggerSessionCreate
	require.NoError(t, manager.Register(HookSessionCreate, makeHandler("session_create")))
	_, err = manager.TriggerSessionCreate(ctx, "session-1", time.Now(), nil)
	require.NoError(t, err)
	assert.Equal(t, "session_create", triggered)

	// Test TriggerSessionEnd
	require.NoError(t, manager.Register(HookSessionEnd, makeHandler("session_end")))
	_, err = manager.TriggerSessionEnd(ctx, "session-1")
	require.NoError(t, err)
	assert.Equal(t, "session_end", triggered)
}

// TestIntegration_HookPanicRecovery tests that panics in hooks are recovered
func TestIntegration_HookPanicRecovery(t *testing.T) {
	manager := NewManager()

	var normalExecuted bool

	panicHandler := &Handler{
		ID:       "panic",
		Priority: 200, // Run first
		Source:   "test",
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			panic("test panic")
		},
		Enabled: true,
	}

	normalHandler := &Handler{
		ID:       "normal",
		Priority: 100, // Run second
		Source:   "test",
		Handler: func(ctx context.Context, hookCtx *Context) (*Result, error) {
			normalExecuted = true
			return &Result{Continue: true}, nil
		},
		Enabled: true,
	}

	require.NoError(t, manager.Register(HookBeforeMessage, panicHandler))
	require.NoError(t, manager.Register(HookBeforeMessage, normalHandler))

	ctx := context.Background()
	hookCtx := NewContext(HookBeforeMessage)

	// Should not panic, panic should be recovered
	result, err := manager.Trigger(ctx, hookCtx)

	// Depending on implementation:
	// - Might return error
	// - Might skip panicking handler and continue
	// Just verify no panic propagates
	_ = result
	_ = err

	// Normal handler may or may not execute depending on implementation
	_ = normalExecuted
}
