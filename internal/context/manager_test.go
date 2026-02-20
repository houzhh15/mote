package context

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"mote/internal/compaction"
	"mote/internal/provider"
	"mote/internal/storage"
)

func setupTestManager(t *testing.T) (*Manager, *storage.DB) {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := storage.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	compactorConfig := compaction.DefaultConfig()
	compactorConfig.KeepRecentCount = 3
	compactorConfig.MaxContextTokens = 100
	compactorConfig.TriggerThreshold = 0.8

	c := compaction.NewCompactor(compactorConfig, nil)

	config := Config{
		MaxContextTokens:       100,
		TriggerThreshold:       0.8,
		KeepRecentCount:        3,
		TargetCompressionRatio: 0.3,
	}

	mgr := NewManager(db, c, nil, config)
	return mgr, db
}

func TestNewManager(t *testing.T) {
	mgr, db := setupTestManager(t)
	defer db.Close()

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if mgr.db == nil {
		t.Error("expected non-nil db")
	}
	if mgr.compactor == nil {
		t.Error("expected non-nil compactor")
	}
	if mgr.tokenCounter == nil {
		t.Error("expected non-nil tokenCounter")
	}
}

func TestNeedsCompression(t *testing.T) {
	mgr, db := setupTestManager(t)
	defer db.Close()

	small := []provider.Message{
		{Role: provider.RoleUser, Content: "hello"},
	}
	if mgr.NeedsCompression(small) {
		t.Error("expected no compression for small messages")
	}

	large := make([]provider.Message, 50)
	for i := range large {
		large[i] = provider.Message{
			Role:    provider.RoleUser,
			Content: "This is a test message with enough content to exceed the token threshold.",
		}
	}
	if !mgr.NeedsCompression(large) {
		t.Error("expected compression needed for large messages")
	}
}

func TestBuildContext_NoCompressedContext(t *testing.T) {
	mgr, db := setupTestManager(t)
	defer db.Close()

	session, err := db.CreateSession(nil)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if _, err := db.AppendMessage(session.ID, "user", "Hello", nil, ""); err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}
	if _, err := db.AppendMessage(session.ID, "assistant", "Hi there!", nil, ""); err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}

	messages, err := mgr.BuildContext(context.Background(), session.ID, "You are a helper.", "How are you?")
	if err != nil {
		t.Fatalf("BuildContext failed: %v", err)
	}

	// system + 2 history + current user input = 4
	if len(messages) != 4 {
		t.Errorf("expected 4 messages, got %d", len(messages))
	}
	if messages[0].Role != provider.RoleSystem {
		t.Errorf("expected first message to be system, got %s", messages[0].Role)
	}
	if messages[0].Content != "You are a helper." {
		t.Errorf("unexpected system content: %s", messages[0].Content)
	}
	last := messages[len(messages)-1]
	if last.Role != provider.RoleUser || last.Content != "How are you?" {
		t.Errorf("expected last message to be current user input, got %s: %s", last.Role, last.Content)
	}
}

func TestBuildContext_WithCompressedContext(t *testing.T) {
	mgr, db := setupTestManager(t)
	defer db.Close()

	session, err := db.CreateSession(nil)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, _ = db.AppendMessage(session.ID, "user", "Hello", nil, "")
	msg2, _ := db.AppendMessage(session.ID, "assistant", "Hi there!", nil, "")

	if err := db.SaveContext(&storage.Context{
		SessionID:      session.ID,
		Version:        1,
		Summary:        "User greeted the assistant.",
		KeptMessageIDs: []string{msg2.ID},
		TotalTokens:    100,
		OriginalTokens: 500,
	}); err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	_, _ = db.AppendMessage(session.ID, "user", "What's the weather?", nil, "")

	messages, err := mgr.BuildContext(context.Background(), session.ID, "You are a helper.", "Tell me more.")
	if err != nil {
		t.Fatalf("BuildContext failed: %v", err)
	}

	if len(messages) < 3 {
		t.Errorf("expected at least 3 messages, got %d", len(messages))
	}
	if messages[0].Role != provider.RoleSystem {
		t.Errorf("expected system role, got %s", messages[0].Role)
	}

	found := false
	for _, msg := range messages {
		if msg.Role == provider.RoleAssistant && strings.Contains(msg.Content, "summary") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected summary message in context")
	}
}

func TestLoadLatestContext_Empty(t *testing.T) {
	mgr, db := setupTestManager(t)
	defer db.Close()

	msgs, err := mgr.LoadLatestContext("nonexistent-session")
	if err != nil {
		t.Fatalf("LoadLatestContext failed: %v", err)
	}
	if msgs != nil {
		t.Error("expected nil for nonexistent session")
	}
}

func TestLoadLatestContext_WithData(t *testing.T) {
	mgr, db := setupTestManager(t)
	defer db.Close()

	session, _ := db.CreateSession(nil)
	msg1, _ := db.AppendMessage(session.ID, "user", "Hello", nil, "")
	msg2, _ := db.AppendMessage(session.ID, "assistant", "Hi!", nil, "")

	if err := db.SaveContext(&storage.Context{
		SessionID:      session.ID,
		Version:        1,
		Summary:        "Greeting exchange.",
		KeptMessageIDs: []string{msg1.ID, msg2.ID},
		TotalTokens:    200,
		OriginalTokens: 1000,
	}); err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	msgs, err := mgr.LoadLatestContext(session.ID)
	if err != nil {
		t.Fatalf("LoadLatestContext failed: %v", err)
	}
	if msgs == nil {
		t.Fatal("expected non-nil messages")
	}
	// 1 summary + 2 kept messages = 3
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].Role != provider.RoleAssistant {
		t.Errorf("expected assistant role for summary, got %s", msgs[0].Role)
	}
}

func TestSaveContext_IncreasesVersion(t *testing.T) {
	mgr, db := setupTestManager(t)
	defer db.Close()

	session, _ := db.CreateSession(nil)

	err := mgr.SaveContext(context.Background(), session.ID, "Summary v1", nil, 500, 2000)
	if err != nil {
		t.Fatalf("SaveContext v1 failed: %v", err)
	}

	err = mgr.SaveContext(context.Background(), session.ID, "Summary v2", nil, 300, 2000)
	if err != nil {
		t.Fatalf("SaveContext v2 failed: %v", err)
	}

	ctx, err := db.GetLatestContext(session.ID)
	if err != nil {
		t.Fatalf("GetLatestContext failed: %v", err)
	}
	if ctx.Version != 2 {
		t.Errorf("expected version 2, got %d", ctx.Version)
	}
	if ctx.Summary != "Summary v2" {
		t.Errorf("expected 'Summary v2', got %q", ctx.Summary)
	}
}

func TestSetMemory(t *testing.T) {
	mgr, db := setupTestManager(t)
	defer db.Close()

	if mgr.memory != nil {
		t.Error("expected nil memory initially")
	}

	mgr.SetMemory(nil)
	if mgr.memory != nil {
		t.Error("expected nil memory after SetMemory(nil)")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxContextTokens != 100000 {
		t.Errorf("expected MaxContextTokens 100000, got %d", cfg.MaxContextTokens)
	}
	if cfg.TriggerThreshold != 0.8 {
		t.Errorf("expected TriggerThreshold 0.8, got %f", cfg.TriggerThreshold)
	}
	if cfg.KeepRecentCount != 20 {
		t.Errorf("expected KeepRecentCount 20, got %d", cfg.KeepRecentCount)
	}
	if cfg.TargetCompressionRatio != 0.3 {
		t.Errorf("expected TargetCompressionRatio 0.3, got %f", cfg.TargetCompressionRatio)
	}
}
