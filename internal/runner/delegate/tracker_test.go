package delegate_test

import (
	"database/sql"
	"testing"
	"time"

	"mote/internal/runner/delegate"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE delegate_invocations (
			id TEXT PRIMARY KEY,
			parent_session_id TEXT NOT NULL,
			child_session_id TEXT NOT NULL,
			agent_name TEXT NOT NULL,
			depth INTEGER NOT NULL,
			chain TEXT NOT NULL,
			prompt TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'running',
			started_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			completed_at TIMESTAMP,
			result_length INTEGER DEFAULT 0,
			tokens_used INTEGER DEFAULT 0,
			error_message TEXT,
			mode TEXT NOT NULL DEFAULT 'legacy',
			executed_steps TEXT,
			pda_stack_depth INTEGER NOT NULL DEFAULT 0
		)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestTracker_StartAndComplete(t *testing.T) {
	db := setupTestDB(t)
	tracker := delegate.NewTracker(db)

	record := delegate.DelegationRecord{
		ID:              "dlg_test_001",
		ParentSessionID: "parent-sess-1",
		ChildSessionID:  "delegate:parent-sess-1:researcher:123",
		AgentName:       "researcher",
		Depth:           1,
		Chain:           delegate.ChainToJSON([]string{"main", "researcher"}),
		Prompt:          "Research quantum computing",
		StartedAt:       time.Now(),
	}

	// Start delegation
	err := tracker.StartDelegation(record)
	if err != nil {
		t.Fatalf("StartDelegation failed: %v", err)
	}

	// Verify it was recorded
	got, err := tracker.GetByID("dlg_test_001")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected record, got nil")
	}
	if got.Status != "running" {
		t.Errorf("expected status 'running', got %q", got.Status)
	}
	if got.AgentName != "researcher" {
		t.Errorf("expected agent 'researcher', got %q", got.AgentName)
	}

	// Complete delegation
	err = tracker.CompleteDelegation("dlg_test_001", "completed", 1500, 250, nil)
	if err != nil {
		t.Fatalf("CompleteDelegation failed: %v", err)
	}

	// Verify completion
	got, err = tracker.GetByID("dlg_test_001")
	if err != nil {
		t.Fatalf("GetByID after complete failed: %v", err)
	}
	if got.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", got.Status)
	}
	if got.ResultLength != 1500 {
		t.Errorf("expected result_length 1500, got %d", got.ResultLength)
	}
	if got.TokensUsed != 250 {
		t.Errorf("expected tokens_used 250, got %d", got.TokensUsed)
	}
	if got.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
}

func TestTracker_CompleteFailed(t *testing.T) {
	db := setupTestDB(t)
	tracker := delegate.NewTracker(db)

	record := delegate.DelegationRecord{
		ID:              "dlg_fail_001",
		ParentSessionID: "parent-sess-2",
		ChildSessionID:  "delegate:parent-sess-2:coder:456",
		AgentName:       "coder",
		Depth:           1,
		Chain:           delegate.ChainToJSON([]string{"main", "coder"}),
		Prompt:          "Fix the bug",
		StartedAt:       time.Now(),
	}

	_ = tracker.StartDelegation(record)

	errMsg := "context deadline exceeded"
	err := tracker.CompleteDelegation("dlg_fail_001", "timeout", 0, 100, &errMsg)
	if err != nil {
		t.Fatalf("CompleteDelegation failed: %v", err)
	}

	got, _ := tracker.GetByID("dlg_fail_001")
	if got.Status != "timeout" {
		t.Errorf("expected status 'timeout', got %q", got.Status)
	}
	if got.ErrorMessage == nil || *got.ErrorMessage != errMsg {
		t.Errorf("expected error message %q, got %v", errMsg, got.ErrorMessage)
	}
}

func TestTracker_GetByParentSession(t *testing.T) {
	db := setupTestDB(t)
	tracker := delegate.NewTracker(db)

	parentID := "parent-multi"

	for i, name := range []string{"researcher", "coder", "reviewer"} {
		_ = tracker.StartDelegation(delegate.DelegationRecord{
			ID:              delegate.GenerateInvocationID(parentID, name),
			ParentSessionID: parentID,
			ChildSessionID:  "child-" + name,
			AgentName:       name,
			Depth:           1,
			Chain:           delegate.ChainToJSON([]string{"main", name}),
			Prompt:          "Task " + name,
			StartedAt:       time.Now().Add(time.Duration(i) * time.Millisecond),
		})
	}

	records, err := tracker.GetByParentSession(parentID)
	if err != nil {
		t.Fatalf("GetByParentSession failed: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	// Verify ordering
	for _, r := range records {
		if r.ParentSessionID != parentID {
			t.Errorf("expected parent %q, got %q", parentID, r.ParentSessionID)
		}
	}
}

func TestTracker_GetByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	tracker := delegate.NewTracker(db)

	got, err := tracker.GetByID("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for non-existent ID, got %v", got)
	}
}

func TestChainToJSON(t *testing.T) {
	result := delegate.ChainToJSON([]string{"main", "researcher", "summarizer"})
	expected := `["main","researcher","summarizer"]`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}
