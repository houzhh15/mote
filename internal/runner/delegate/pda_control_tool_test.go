package delegate

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"mote/internal/runner/delegate/cfg"
	"mote/internal/storage"
)

func setupPDAControlTestDB(t *testing.T) *storage.DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "pda_control_test_*.db")
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

func makePDAControlCheckpoint() *cfg.PDACheckpoint {
	return &cfg.PDACheckpoint{
		AgentName: "researcher",
		Stack: []cfg.SerializableFrame{
			{
				AgentName:  "researcher",
				StepIndex:  2,
				TotalSteps: 5,
			},
		},
		LastResult:      "partial analysis",
		ExecutedSteps:   []string{"step-0", "step-1"},
		InterruptStep:   2,
		InterruptAgent:  "researcher",
		InterruptReason: "429 rate limit exceeded",
		DelegateInfo: cfg.DelegateInfo{
			AgentName: "researcher",
			MaxDepth:  5,
		},
		InitialPrompt: "analyze the data",
	}
}

func TestPDAControlToolContinue(t *testing.T) {
	db := setupPDAControlTestDB(t)
	sess, err := db.CreateSession(nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	cp := makePDAControlCheckpoint()
	if err := SavePDACheckpoint(db, sess.ID, cp); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	tool := NewPDAControlTool(PDAControlToolOptions{
		Store:      db,
		SessionID:  sess.ID,
		Checkpoint: cp,
	})

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "continue",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}
	// Without a resumeFn, the fallback text mentions @agent
	if !strings.Contains(result.Content, "researcher") {
		t.Errorf("expected agent name in result, got: %s", result.Content)
	}

	// Checkpoint should still be in storage (continue retains it)
	loaded, err := LoadPDACheckpoint(db, sess.ID)
	if err != nil {
		t.Fatalf("load after continue: %v", err)
	}
	if loaded == nil {
		t.Error("checkpoint should still exist after 'continue' action")
	}
}

func TestPDAControlToolContinueWithResumeFn(t *testing.T) {
	cp := makePDAControlCheckpoint()

	// Simulate a successful PDA resume
	resumed := false
	tool := NewPDAControlTool(PDAControlToolOptions{
		Checkpoint: cp,
		SessionID:  "test-session",
		ResumeFn: func(ctx context.Context) (string, error) {
			resumed = true
			return "PDA completed all 5 steps successfully", nil
		},
	})

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "continue",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}
	if !resumed {
		t.Error("resumeFn was not called")
	}
	if !strings.Contains(result.Content, "PDA Resumed") {
		t.Errorf("expected 'PDA Resumed' in result, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "PDA completed all 5 steps") {
		t.Errorf("expected PDA result in output, got: %s", result.Content)
	}
}

func TestPDAControlToolContinueWithResumeFnError(t *testing.T) {
	cp := makePDAControlCheckpoint()

	tool := NewPDAControlTool(PDAControlToolOptions{
		Checkpoint: cp,
		SessionID:  "test-session",
		ResumeFn: func(ctx context.Context) (string, error) {
			return "", fmt.Errorf("rate limit exceeded")
		},
	})

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "continue",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result when resumeFn fails")
	}
	if !strings.Contains(result.Content, "rate limit") {
		t.Errorf("expected error message in result, got: %s", result.Content)
	}
}

func TestPDAControlToolRestart(t *testing.T) {
	db := setupPDAControlTestDB(t)
	sess, err := db.CreateSession(nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	cp := makePDAControlCheckpoint()
	if err := SavePDACheckpoint(db, sess.ID, cp); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	tool := NewPDAControlTool(PDAControlToolOptions{
		Store:      db,
		SessionID:  sess.ID,
		Checkpoint: cp,
	})

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "restart",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "cleared") {
		t.Errorf("expected 'cleared' in result, got: %s", result.Content)
	}

	// Checkpoint should be removed after restart
	loaded, err := LoadPDACheckpoint(db, sess.ID)
	if err != nil {
		t.Fatalf("load after restart: %v", err)
	}
	if loaded != nil {
		t.Error("checkpoint should be nil after 'restart' action")
	}
}

func TestPDAControlToolDescription(t *testing.T) {
	cp := makePDAControlCheckpoint()
	tool := NewPDAControlTool(PDAControlToolOptions{
		Checkpoint: cp,
		SessionID:  "test-session",
	})

	desc := tool.Description()

	// Should contain agent name
	if !strings.Contains(desc, "researcher") {
		t.Errorf("description missing agent name, got: %s", desc)
	}
	// Should contain interrupt step
	if !strings.Contains(desc, "step 2") {
		t.Errorf("description missing interrupt step, got: %s", desc)
	}
	// Should contain interrupt reason
	if !strings.Contains(desc, "429") {
		t.Errorf("description missing interrupt reason, got: %s", desc)
	}
	// Should contain completed steps
	if !strings.Contains(desc, "step-0") || !strings.Contains(desc, "step-1") {
		t.Errorf("description missing completed steps, got: %s", desc)
	}
}

func TestPDAControlToolInvalidAction(t *testing.T) {
	tool := NewPDAControlTool(PDAControlToolOptions{
		Checkpoint: makePDAControlCheckpoint(),
		SessionID:  "test-session",
	})

	result, err := tool.Execute(context.Background(), map[string]any{
		"action": "invalid",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for invalid action")
	}
}

func TestPDAResumeHint(t *testing.T) {
	cp := makePDAControlCheckpoint()
	hint := PDAResumeHint(cp)

	if !strings.Contains(hint, "PDA Checkpoint Detected") {
		t.Error("hint missing header")
	}
	if !strings.Contains(hint, "researcher") {
		t.Error("hint missing agent name")
	}
	if !strings.Contains(hint, "pda_control") {
		t.Error("hint missing tool reference")
	}
}
