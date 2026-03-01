package delegate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"mote/internal/provider"
	"mote/internal/runner/delegate/cfg"
	"mote/internal/storage"
)

func setupCheckpointTestDB(t *testing.T) *storage.DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "checkpoint_test_*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	db, err := storage.Open(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		os.Remove(tmpPath)
	})
	return db
}

func makeTestCheckpoint() *cfg.PDACheckpoint {
	return &cfg.PDACheckpoint{
		AgentName: "test-agent",
		Stack: []cfg.SerializableFrame{
			{
				AgentName:  "test-agent",
				StepIndex:  1,
				TotalSteps: 3,
			},
		},
		LastResult:    "partial-result",
		ExecutedSteps: []string{"step-0"},
		DelegateInfo: cfg.DelegateInfo{
			AgentName: "test-agent",
			MaxDepth:  5,
		},
		InitialPrompt: "do the thing",
	}
}

func TestSavePDACheckpoint(t *testing.T) {
	db := setupCheckpointTestDB(t)
	sess, err := db.CreateSession(nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	cp := makeTestCheckpoint()
	if err := SavePDACheckpoint(db, sess.ID, cp); err != nil {
		t.Fatalf("SavePDACheckpoint: %v", err)
	}

	// Verify that Metadata now contains pda_checkpoint key
	updated, err := db.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	var meta map[string]json.RawMessage
	if err := json.Unmarshal(updated.Metadata, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if _, ok := meta[pdaCheckpointKey]; !ok {
		t.Errorf("metadata missing %q key", pdaCheckpointKey)
	}
}

func TestLoadPDACheckpoint(t *testing.T) {
	db := setupCheckpointTestDB(t)
	sess, err := db.CreateSession(nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	cp := makeTestCheckpoint()
	if err := SavePDACheckpoint(db, sess.ID, cp); err != nil {
		t.Fatalf("SavePDACheckpoint: %v", err)
	}

	loaded, err := LoadPDACheckpoint(db, sess.ID)
	if err != nil {
		t.Fatalf("LoadPDACheckpoint: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil checkpoint")
	}

	// Verify fields round-trip correctly
	if loaded.AgentName != "test-agent" {
		t.Errorf("AgentName = %q, want 'test-agent'", loaded.AgentName)
	}
	if len(loaded.Stack) != 1 || loaded.Stack[0].StepIndex != 1 {
		t.Errorf("Stack mismatch: %+v", loaded.Stack)
	}
	if loaded.LastResult != "partial-result" {
		t.Errorf("LastResult = %q, want 'partial-result'", loaded.LastResult)
	}
	if len(loaded.ExecutedSteps) != 1 || loaded.ExecutedSteps[0] != "step-0" {
		t.Errorf("ExecutedSteps = %v", loaded.ExecutedSteps)
	}
	if loaded.DelegateInfo.AgentName != "test-agent" || loaded.DelegateInfo.MaxDepth != 5 {
		t.Errorf("DelegateInfo = %+v", loaded.DelegateInfo)
	}
	if loaded.InitialPrompt != "do the thing" {
		t.Errorf("InitialPrompt = %q", loaded.InitialPrompt)
	}
}

func TestLoadPDACheckpointNone(t *testing.T) {
	db := setupCheckpointTestDB(t)
	sess, err := db.CreateSession(nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	loaded, err := LoadPDACheckpoint(db, sess.ID)
	if err != nil {
		t.Fatalf("LoadPDACheckpoint: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil checkpoint, got %+v", loaded)
	}
}

func TestClearPDACheckpoint(t *testing.T) {
	db := setupCheckpointTestDB(t)
	sess, err := db.CreateSession(nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	cp := makeTestCheckpoint()
	if err := SavePDACheckpoint(db, sess.ID, cp); err != nil {
		t.Fatalf("SavePDACheckpoint: %v", err)
	}

	if err := ClearPDACheckpoint(db, sess.ID); err != nil {
		t.Fatalf("ClearPDACheckpoint: %v", err)
	}

	loaded, err := LoadPDACheckpoint(db, sess.ID)
	if err != nil {
		t.Fatalf("LoadPDACheckpoint after clear: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil after clear, got %+v", loaded)
	}
}

func TestSavePreservesOtherMetadata(t *testing.T) {
	db := setupCheckpointTestDB(t)

	// Create session with existing metadata
	existingMeta := json.RawMessage(`{"user_id":"u-123","theme":"dark"}`)
	sess, err := db.CreateSession(existingMeta)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	cp := makeTestCheckpoint()
	if err := SavePDACheckpoint(db, sess.ID, cp); err != nil {
		t.Fatalf("SavePDACheckpoint: %v", err)
	}

	// Verify other keys are preserved
	updated, err := db.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	var meta map[string]json.RawMessage
	if err := json.Unmarshal(updated.Metadata, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}

	// Check existing keys survived
	var userID string
	if err := json.Unmarshal(meta["user_id"], &userID); err != nil {
		t.Fatalf("unmarshal user_id: %v", err)
	}
	if userID != "u-123" {
		t.Errorf("user_id = %q, want 'u-123'", userID)
	}

	var theme string
	if err := json.Unmarshal(meta["theme"], &theme); err != nil {
		t.Fatalf("unmarshal theme: %v", err)
	}
	if theme != "dark" {
		t.Errorf("theme = %q, want 'dark'", theme)
	}

	// Check checkpoint is also present
	if _, ok := meta[pdaCheckpointKey]; !ok {
		t.Errorf("missing %q key", pdaCheckpointKey)
	}
}

// TestCheckpointRoundTrip_EngineIntegration exercises the full lifecycle:
// engine error → OnCheckpoint → SavePDACheckpoint → LoadPDACheckpoint → engine resume → ClearPDACheckpoint.
// This integration test verifies all components work together end-to-end.
func TestCheckpointRoundTrip_EngineIntegration(t *testing.T) {
	db := setupCheckpointTestDB(t)
	sess, err := db.CreateSession(nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	steps := []cfg.Step{
		{Type: cfg.StepPrompt, Label: "step-0", Prompt: "do step 0"},
		{Type: cfg.StepPrompt, Label: "step-1", Prompt: "do step 1"},
		{Type: cfg.StepPrompt, Label: "step-2", Prompt: "do step 2"},
	}
	agents := map[string]*cfg.AgentCFG{
		"main": {Steps: steps},
	}
	agentProvider := func(name string) (*cfg.AgentCFG, bool) {
		a, ok := agents[name]
		return a, ok
	}

	// Phase 1: Run engine, simulate error at step-1
	callCount := 0
	engine1 := cfg.NewPDAEngine(cfg.PDAEngineOptions{
		RunPromptWithContext: func(ctx context.Context, agentName string, messages []provider.Message, userInput string) (string, cfg.Usage, []provider.Message, error) {
			callCount++
			if callCount == 2 { // step-1 fails
				return "", cfg.Usage{}, nil, fmt.Errorf("429 rate limit")
			}
			result := fmt.Sprintf("result-%d", callCount-1)
			newMsgs := []provider.Message{
				{Role: provider.RoleUser, Content: userInput},
				{Role: provider.RoleAssistant, Content: result},
			}
			return result, cfg.Usage{TotalTokens: 10}, newMsgs, nil
		},
		AgentProvider: agentProvider,
		MaxStackDepth: 10,
	})
	engine1.OnCheckpoint = func(cp *cfg.PDACheckpoint) error {
		return SavePDACheckpoint(db, sess.ID, cp)
	}

	delegateInfo := &cfg.DelegateInfo{
		Depth: 0, MaxDepth: 10, AgentName: "main",
		Chain: []string{"main"}, RecursionCounters: map[string]int{},
	}
	_, _, err = engine1.Execute(
		context.Background(), delegateInfo,
		cfg.AgentCFG{Steps: steps}, "integration test", nil,
	)
	if err == nil {
		t.Fatal("expected error from step-1, got nil")
	}

	// Verify checkpoint was saved with interrupt info
	loaded, err := LoadPDACheckpoint(db, sess.ID)
	if err != nil {
		t.Fatalf("LoadPDACheckpoint: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected saved checkpoint after error")
	}
	if loaded.InterruptReason == "" {
		t.Error("expected InterruptReason to be set")
	}
	if loaded.InterruptStep != 1 {
		t.Errorf("InterruptStep = %d, want 1", loaded.InterruptStep)
	}
	if len(loaded.ExecutedSteps) != 1 || loaded.ExecutedSteps[0] != "step-0" {
		t.Errorf("ExecutedSteps = %v, want [step-0]", loaded.ExecutedSteps)
	}

	// Phase 2: Resume from checkpoint — should skip step-0 and execute steps 1 and 2
	callCount2 := 0
	engine2 := cfg.NewPDAEngine(cfg.PDAEngineOptions{
		RunPromptWithContext: func(ctx context.Context, agentName string, messages []provider.Message, userInput string) (string, cfg.Usage, []provider.Message, error) {
			callCount2++
			result := fmt.Sprintf("resumed-result-%d", callCount2)
			newMsgs := []provider.Message{
				{Role: provider.RoleUser, Content: userInput},
				{Role: provider.RoleAssistant, Content: result},
			}
			return result, cfg.Usage{TotalTokens: 15}, newMsgs, nil
		},
		AgentProvider: agentProvider,
		MaxStackDepth: 10,
	})

	result, usage, err := engine2.Execute(
		context.Background(), delegateInfo,
		cfg.AgentCFG{Steps: steps}, "integration test", loaded,
	)
	if err != nil {
		t.Fatalf("Resume Execute failed: %v", err)
	}

	// Should have executed only steps 1 and 2
	if callCount2 != 2 {
		t.Errorf("expected 2 calls on resume, got %d", callCount2)
	}
	if result != "resumed-result-2" {
		t.Errorf("result = %q, want 'resumed-result-2'", result)
	}
	if usage.TotalTokens != 30 { // 2 * 15
		t.Errorf("TotalTokens = %d, want 30", usage.TotalTokens)
	}

	// Phase 3: Clear checkpoint after success
	if err := ClearPDACheckpoint(db, sess.ID); err != nil {
		t.Fatalf("ClearPDACheckpoint: %v", err)
	}
	cleared, err := LoadPDACheckpoint(db, sess.ID)
	if err != nil {
		t.Fatalf("LoadPDACheckpoint after clear: %v", err)
	}
	if cleared != nil {
		t.Errorf("expected nil after clear, got %+v", cleared)
	}
}
