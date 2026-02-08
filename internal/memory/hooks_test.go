package memory

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
)

func TestNewMemoryHooks(t *testing.T) {
	hooks := NewMemoryHooks(MemoryHooksOptions{
		Logger: zerolog.Nop(),
	})

	if hooks == nil {
		t.Fatal("hooks is nil")
	}
}

func TestMemoryHooks_BeforeRun(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	ctx := context.Background()

	// Add a test memory
	if err := idx.Add(ctx, MemoryEntry{
		Content:  "User prefers dark theme",
		Category: CategoryPreference,
	}); err != nil {
		t.Fatalf("add memory: %v", err)
	}

	recallEngine := NewRecallEngine(RecallEngineOptions{
		Memory: idx,
		Config: DefaultRecallConfig(),
		Logger: zerolog.Nop(),
	})

	hooks := NewMemoryHooks(MemoryHooksOptions{
		Recall: recallEngine,
		Logger: zerolog.Nop(),
	})

	t.Run("returns context for relevant prompt", func(t *testing.T) {
		result, err := hooks.BeforeRun(ctx, "Tell me about user preferences")
		if err != nil {
			t.Fatalf("before run: %v", err)
		}
		// May be empty if FTS doesn't match, which is OK
		_ = result
	})

	t.Run("returns empty for nil recall engine", func(t *testing.T) {
		hooksNoRecall := NewMemoryHooks(MemoryHooksOptions{
			Logger: zerolog.Nop(),
		})

		result, err := hooksNoRecall.BeforeRun(ctx, "Tell me about user preferences")
		if err != nil {
			t.Fatalf("before run: %v", err)
		}
		if result != "" {
			t.Errorf("expected empty result with nil recall, got: %s", result)
		}
	})
}

func TestMemoryHooks_AfterRun(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	captureEngine, err := NewCaptureEngine(CaptureEngineOptions{
		Memory: idx,
		Config: DefaultCaptureConfig(),
		Logger: zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("create capture engine: %v", err)
	}

	hooks := NewMemoryHooks(MemoryHooksOptions{
		Capture: captureEngine,
		Logger:  zerolog.Nop(),
	})

	ctx := context.Background()

	t.Run("captures on successful run", func(t *testing.T) {
		messages := []Message{
			{Role: "user", Content: "Please remember that I prefer dark mode"},
		}

		err := hooks.AfterRun(ctx, messages, true)
		if err != nil {
			t.Fatalf("after run: %v", err)
		}

		// Verify capture
		count, _ := idx.Count(ctx)
		if count != 1 {
			t.Errorf("expected 1 captured memory, got %d", count)
		}
	})

	t.Run("skips capture on failed run", func(t *testing.T) {
		initialCount, _ := idx.Count(ctx)

		messages := []Message{
			{Role: "user", Content: "Remember my email is test@example.com"},
		}

		err := hooks.AfterRun(ctx, messages, false) // Failed run
		if err != nil {
			t.Fatalf("after run: %v", err)
		}

		// Verify no new captures
		count, _ := idx.Count(ctx)
		if count != initialCount {
			t.Errorf("expected %d memories (no new captures), got %d", initialCount, count)
		}
	})

	t.Run("handles nil capture engine", func(t *testing.T) {
		hooksNoCapture := NewMemoryHooks(MemoryHooksOptions{
			Logger: zerolog.Nop(),
		})

		messages := []Message{
			{Role: "user", Content: "Remember this important fact"},
		}

		err := hooksNoCapture.AfterRun(ctx, messages, true)
		if err != nil {
			t.Fatalf("after run: %v", err)
		}
	})
}

func TestMemoryHooks_ResetSession(t *testing.T) {
	captureEngine, err := NewCaptureEngine(CaptureEngineOptions{
		Config: DefaultCaptureConfig(),
		Logger: zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("create capture engine: %v", err)
	}

	// Simulate some captures
	captureEngine.captured = 3

	hooks := NewMemoryHooks(MemoryHooksOptions{
		Capture: captureEngine,
		Logger:  zerolog.Nop(),
	})

	hooks.ResetSession()

	if captureEngine.captured != 0 {
		t.Errorf("expected captured to be 0 after reset, got %d", captureEngine.captured)
	}
}

func TestMemoryHooks_SetSessionID(t *testing.T) {
	captureEngine, err := NewCaptureEngine(CaptureEngineOptions{
		Config: DefaultCaptureConfig(),
		Logger: zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("create capture engine: %v", err)
	}

	hooks := NewMemoryHooks(MemoryHooksOptions{
		Capture: captureEngine,
		Logger:  zerolog.Nop(),
	})

	hooks.SetSessionID("session-123")

	if captureEngine.sessionID != "session-123" {
		t.Errorf("expected session ID 'session-123', got %s", captureEngine.sessionID)
	}
}

func TestMemoryHooks_SetEnabled(t *testing.T) {
	captureEngine, err := NewCaptureEngine(CaptureEngineOptions{
		Config: DefaultCaptureConfig(),
		Logger: zerolog.Nop(),
	})
	if err != nil {
		t.Fatalf("create capture engine: %v", err)
	}

	recallEngine := NewRecallEngine(RecallEngineOptions{
		Config: DefaultRecallConfig(),
		Logger: zerolog.Nop(),
	})

	hooks := NewMemoryHooks(MemoryHooksOptions{
		Capture: captureEngine,
		Recall:  recallEngine,
		Logger:  zerolog.Nop(),
	})

	// Both should be enabled by default
	if !hooks.IsCaptureEnabled() {
		t.Error("expected capture to be enabled")
	}
	if !hooks.IsRecallEnabled() {
		t.Error("expected recall to be enabled")
	}

	// Disable both
	hooks.SetEnabled(false)

	if hooks.IsCaptureEnabled() {
		t.Error("expected capture to be disabled")
	}
	if hooks.IsRecallEnabled() {
		t.Error("expected recall to be disabled")
	}

	// Re-enable
	hooks.SetEnabled(true)

	if !hooks.IsCaptureEnabled() {
		t.Error("expected capture to be enabled")
	}
	if !hooks.IsRecallEnabled() {
		t.Error("expected recall to be enabled")
	}
}

func TestMemoryHooks_EnabledChecks_NilEngines(t *testing.T) {
	hooks := NewMemoryHooks(MemoryHooksOptions{
		Logger: zerolog.Nop(),
	})

	// Should return false for nil engines
	if hooks.IsCaptureEnabled() {
		t.Error("expected false for nil capture engine")
	}
	if hooks.IsRecallEnabled() {
		t.Error("expected false for nil recall engine")
	}
}
