package builtin

import (
	"context"
	"testing"
	"time"

	"mote/internal/hooks"
)

func TestRateLimitHook_Handler(t *testing.T) {
	hook := NewRateLimitHook(RateLimitConfig{
		MaxRequests: 10,
		Window:      time.Minute,
	})

	handler := hook.Handler("test-ratelimit")
	if handler == nil {
		t.Fatal("expected handler to be created")
	}

	if handler.ID != "test-ratelimit" {
		t.Errorf("expected ID 'test-ratelimit', got '%s'", handler.ID)
	}

	if handler.Priority != 80 {
		t.Errorf("expected priority 80, got %d", handler.Priority)
	}
}

func TestRateLimitHook_AllowWithinLimit(t *testing.T) {
	hook := NewRateLimitHook(RateLimitConfig{
		MaxRequests: 5,
		Window:      time.Minute,
	})

	handler := hook.Handler("test-ratelimit")

	// Make requests within limit
	for i := 0; i < 5; i++ {
		hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
		result, err := handler.Handler(context.Background(), hookCtx)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result to be returned")
		}

		if !result.Continue {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimitHook_BlockExceedLimit(t *testing.T) {
	hook := NewRateLimitHook(RateLimitConfig{
		MaxRequests: 3,
		Window:      time.Minute,
		Message:     "too many requests",
	})

	handler := hook.Handler("test-ratelimit")

	// Make requests exceeding limit
	for i := 0; i < 5; i++ {
		hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
		result, _ := handler.Handler(context.Background(), hookCtx)

		if i < 3 {
			if !result.Continue {
				t.Errorf("request %d should be allowed", i+1)
			}
		} else {
			if result.Continue {
				t.Errorf("request %d should be blocked", i+1)
			}
			if result.Error == nil || result.Error.Error() != "too many requests" {
				t.Error("expected error message")
			}
		}
	}
}

func TestRateLimitHook_WithKeyFunc(t *testing.T) {
	hook := NewRateLimitHook(RateLimitConfig{
		MaxRequests: 2,
		Window:      time.Minute,
		KeyFunc: func(hookCtx *hooks.Context) string {
			if hookCtx.Session != nil {
				return hookCtx.Session.ID
			}
			return "_global"
		},
	})

	handler := hook.Handler("test-ratelimit")

	// Session 1: make 3 requests
	for i := 0; i < 3; i++ {
		hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
		hookCtx.Session = &hooks.SessionContext{ID: "session-1"}
		result, _ := handler.Handler(context.Background(), hookCtx)

		if i < 2 {
			if !result.Continue {
				t.Errorf("session-1 request %d should be allowed", i+1)
			}
		} else {
			if result.Continue {
				t.Errorf("session-1 request %d should be blocked", i+1)
			}
		}
	}

	// Session 2: should have its own limit
	hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
	hookCtx.Session = &hooks.SessionContext{ID: "session-2"}
	result, _ := handler.Handler(context.Background(), hookCtx)

	if !result.Continue {
		t.Error("session-2 request should be allowed (separate counter)")
	}
}

func TestRateLimitHook_Reset(t *testing.T) {
	hook := NewRateLimitHook(RateLimitConfig{
		MaxRequests: 2,
		Window:      time.Minute,
	})

	handler := hook.Handler("test-ratelimit")

	// Make requests to hit limit
	for i := 0; i < 3; i++ {
		hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
		_, _ = handler.Handler(context.Background(), hookCtx)
	}

	// Reset and verify
	hook.Reset("_global")

	hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
	result, _ := handler.Handler(context.Background(), hookCtx)

	if !result.Continue {
		t.Error("request should be allowed after reset")
	}
}

func TestRateLimitHook_ResetAll(t *testing.T) {
	hook := NewRateLimitHook(RateLimitConfig{
		MaxRequests: 1,
		Window:      time.Minute,
		KeyFunc:     SessionRateLimitKeyFunc(),
	})

	handler := hook.Handler("test-ratelimit")

	// Make requests for multiple sessions
	for _, sessionID := range []string{"s1", "s2", "s3"} {
		hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
		hookCtx.Session = &hooks.SessionContext{ID: sessionID}
		_, _ = handler.Handler(context.Background(), hookCtx)
		_, _ = handler.Handler(context.Background(), hookCtx) // exceed limit
	}

	// Reset all
	hook.ResetAll()

	// All should be allowed again
	for _, sessionID := range []string{"s1", "s2", "s3"} {
		hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
		hookCtx.Session = &hooks.SessionContext{ID: sessionID}
		result, _ := handler.Handler(context.Background(), hookCtx)

		if !result.Continue {
			t.Errorf("session %s should be allowed after reset all", sessionID)
		}
	}
}

func TestRateLimitHook_GetCount(t *testing.T) {
	hook := NewRateLimitHook(RateLimitConfig{
		MaxRequests: 10,
		Window:      time.Minute,
	})

	handler := hook.Handler("test-ratelimit")

	// Make 3 requests
	for i := 0; i < 3; i++ {
		hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
		_, _ = handler.Handler(context.Background(), hookCtx)
	}

	count := hook.GetCount("_global")
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}

func TestRateLimitHook_Stats(t *testing.T) {
	hook := NewRateLimitHook(RateLimitConfig{
		MaxRequests: 10,
		Window:      time.Minute,
		KeyFunc:     SessionRateLimitKeyFunc(),
	})

	handler := hook.Handler("test-ratelimit")

	// Make requests for 2 sessions
	for _, sessionID := range []string{"s1", "s2"} {
		hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
		hookCtx.Session = &hooks.SessionContext{ID: sessionID}
		_, _ = handler.Handler(context.Background(), hookCtx)
	}

	stats := hook.Stats()
	if stats.ActiveKeys != 2 {
		t.Errorf("expected 2 active keys, got %d", stats.ActiveKeys)
	}
}

func TestRateLimitHook_OnLimit(t *testing.T) {
	limitedKey := ""
	limitedCount := 0

	hook := NewRateLimitHook(RateLimitConfig{
		MaxRequests: 1,
		Window:      time.Minute,
		OnLimit: func(key string, count int) {
			limitedKey = key
			limitedCount = count
		},
	})

	handler := hook.Handler("test-ratelimit")

	// First request - allowed
	hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
	_, _ = handler.Handler(context.Background(), hookCtx)

	// Second request - blocked, triggers OnLimit
	hookCtx = hooks.NewContext(hooks.HookBeforeMessage)
	_, _ = handler.Handler(context.Background(), hookCtx)

	if limitedKey != "_global" {
		t.Errorf("expected limited key '_global', got '%s'", limitedKey)
	}

	if limitedCount != 2 {
		t.Errorf("expected limited count 2, got %d", limitedCount)
	}
}

func TestSessionRateLimitKeyFunc(t *testing.T) {
	keyFunc := SessionRateLimitKeyFunc()

	// With session
	hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
	hookCtx.Session = &hooks.SessionContext{ID: "test-session"}
	key := keyFunc(hookCtx)

	if key != "session:test-session" {
		t.Errorf("expected 'session:test-session', got '%s'", key)
	}

	// Without session
	hookCtx = hooks.NewContext(hooks.HookBeforeMessage)
	key = keyFunc(hookCtx)

	if key != "_global" {
		t.Errorf("expected '_global', got '%s'", key)
	}
}

func TestUserRateLimitKeyFunc(t *testing.T) {
	keyFunc := UserRateLimitKeyFunc("user_id")

	// With user ID in metadata
	hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
	hookCtx.Session = &hooks.SessionContext{
		ID:       "session-1",
		Metadata: map[string]any{"user_id": "user-123"},
	}
	key := keyFunc(hookCtx)

	if key != "user:user-123" {
		t.Errorf("expected 'user:user-123', got '%s'", key)
	}

	// Without user ID
	hookCtx = hooks.NewContext(hooks.HookBeforeMessage)
	hookCtx.Session = &hooks.SessionContext{
		ID:       "session-2",
		Metadata: map[string]any{},
	}
	key = keyFunc(hookCtx)

	if key != "_global" {
		t.Errorf("expected '_global', got '%s'", key)
	}
}

func TestRegisterRateLimitHook(t *testing.T) {
	manager := hooks.NewManager()

	err := RegisterRateLimitHook(manager, RateLimitConfig{
		MaxRequests: 10,
		Window:      time.Minute,
	})
	if err != nil {
		t.Fatalf("failed to register rate limit hook: %v", err)
	}

	if !manager.HasHandlers(hooks.HookBeforeMessage) {
		t.Error("expected handler registered for before_message")
	}
}

func TestRateLimitHook_Cleanup(t *testing.T) {
	hook := NewRateLimitHook(RateLimitConfig{
		MaxRequests: 10,
		Window:      time.Millisecond * 10,
	})

	handler := hook.Handler("test-ratelimit")

	// Make a request
	hookCtx := hooks.NewContext(hooks.HookBeforeMessage)
	_, _ = handler.Handler(context.Background(), hookCtx)

	// Wait for window to expire
	time.Sleep(time.Millisecond * 20)

	// Cleanup
	hook.Cleanup()

	stats := hook.Stats()
	if stats.ActiveKeys != 0 {
		t.Errorf("expected 0 active keys after cleanup, got %d", stats.ActiveKeys)
	}
}
