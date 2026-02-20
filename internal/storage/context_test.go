package storage

import (
	"path/filepath"
	"testing"
)

func TestSaveContext(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx := &Context{
		SessionID:      "session-1",
		Version:        1,
		Summary:        "Test summary of the conversation",
		KeptMessageIDs: []string{"msg-1", "msg-2", "msg-3"},
		TotalTokens:    500,
		OriginalTokens: 2000,
	}

	if err := db.SaveContext(ctx); err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	if ctx.ID == "" {
		t.Error("expected ID to be set")
	}
	if ctx.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestSaveContext_DuplicateVersionFails(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	ctx1 := &Context{
		SessionID:      "session-1",
		Version:        1,
		Summary:        "Summary v1",
		TotalTokens:    500,
		OriginalTokens: 2000,
	}
	if err := db.SaveContext(ctx1); err != nil {
		t.Fatalf("SaveContext v1 failed: %v", err)
	}

	ctx2 := &Context{
		SessionID:      "session-1",
		Version:        1, // duplicate version
		Summary:        "Summary v1 duplicate",
		TotalTokens:    500,
		OriginalTokens: 2000,
	}
	if err := db.SaveContext(ctx2); err == nil {
		t.Error("expected duplicate version to fail")
	}
}

func TestGetLatestContext(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// No context yet
	ctx, err := db.GetLatestContext("session-1")
	if err != nil {
		t.Fatalf("GetLatestContext failed: %v", err)
	}
	if ctx != nil {
		t.Error("expected nil for non-existent session")
	}

	// Save two versions
	if err := db.SaveContext(&Context{
		SessionID: "session-1", Version: 1, Summary: "v1",
		TotalTokens: 500, OriginalTokens: 2000,
		KeptMessageIDs: []string{"msg-1"},
	}); err != nil {
		t.Fatalf("SaveContext v1 failed: %v", err)
	}
	if err := db.SaveContext(&Context{
		SessionID: "session-1", Version: 2, Summary: "v2",
		TotalTokens: 300, OriginalTokens: 2000,
		KeptMessageIDs: []string{"msg-2", "msg-3"},
	}); err != nil {
		t.Fatalf("SaveContext v2 failed: %v", err)
	}

	// Should return v2
	ctx, err = db.GetLatestContext("session-1")
	if err != nil {
		t.Fatalf("GetLatestContext failed: %v", err)
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if ctx.Version != 2 {
		t.Errorf("expected version 2, got %d", ctx.Version)
	}
	if ctx.Summary != "v2" {
		t.Errorf("expected summary 'v2', got %q", ctx.Summary)
	}
	if len(ctx.KeptMessageIDs) != 2 {
		t.Errorf("expected 2 kept message IDs, got %d", len(ctx.KeptMessageIDs))
	}
}

func TestGetLatestContext_SessionIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if err := db.SaveContext(&Context{
		SessionID: "session-1", Version: 1, Summary: "s1-v1",
		TotalTokens: 500, OriginalTokens: 2000,
	}); err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}
	if err := db.SaveContext(&Context{
		SessionID: "session-2", Version: 1, Summary: "s2-v1",
		TotalTokens: 300, OriginalTokens: 1500,
	}); err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	ctx, err := db.GetLatestContext("session-1")
	if err != nil {
		t.Fatalf("GetLatestContext failed: %v", err)
	}
	if ctx.Summary != "s1-v1" {
		t.Errorf("expected 's1-v1', got %q", ctx.Summary)
	}
}

func TestListContexts(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// Save 3 versions
	for i := 1; i <= 3; i++ {
		if err := db.SaveContext(&Context{
			SessionID: "session-1", Version: i, Summary: "summary",
			TotalTokens: 100 * i, OriginalTokens: 2000,
		}); err != nil {
			t.Fatalf("SaveContext v%d failed: %v", i, err)
		}
	}

	// List with limit
	contexts, err := db.ListContexts("session-1", 2)
	if err != nil {
		t.Fatalf("ListContexts failed: %v", err)
	}
	if len(contexts) != 2 {
		t.Errorf("expected 2 contexts, got %d", len(contexts))
	}
	// Should be ordered by version DESC
	if contexts[0].Version != 3 {
		t.Errorf("expected first context version 3, got %d", contexts[0].Version)
	}

	// List all
	contexts, err = db.ListContexts("session-1", 0)
	if err != nil {
		t.Fatalf("ListContexts failed: %v", err)
	}
	if len(contexts) != 3 {
		t.Errorf("expected 3 contexts, got %d", len(contexts))
	}
}

func TestGetMaxContextVersion(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	// No contexts yet
	ver, err := db.GetMaxContextVersion("session-1")
	if err != nil {
		t.Fatalf("GetMaxContextVersion failed: %v", err)
	}
	if ver != 0 {
		t.Errorf("expected 0 for empty, got %d", ver)
	}

	// Add contexts
	if err := db.SaveContext(&Context{
		SessionID: "session-1", Version: 1, Summary: "v1",
		TotalTokens: 500, OriginalTokens: 2000,
	}); err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}
	if err := db.SaveContext(&Context{
		SessionID: "session-1", Version: 5, Summary: "v5",
		TotalTokens: 300, OriginalTokens: 2000,
	}); err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	ver, err = db.GetMaxContextVersion("session-1")
	if err != nil {
		t.Fatalf("GetMaxContextVersion failed: %v", err)
	}
	if ver != 5 {
		t.Errorf("expected 5, got %d", ver)
	}
}
