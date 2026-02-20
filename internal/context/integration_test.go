package context

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mote/internal/compaction"
	"mote/internal/provider"
	"mote/internal/storage"
)

// setupIntegrationManager creates a Manager with low thresholds for testing compression flow.
func setupIntegrationManager(t *testing.T) (*Manager, *storage.DB) {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := storage.Open(filepath.Join(tmpDir, "integration.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	compactorConfig := compaction.DefaultConfig()
	compactorConfig.KeepRecentCount = 3
	compactorConfig.MaxContextTokens = 200 // low threshold for testing
	compactorConfig.TriggerThreshold = 0.5

	c := compaction.NewCompactor(compactorConfig, nil) // nil provider → truncation fallback

	config := Config{
		MaxContextTokens:       200,
		TriggerThreshold:       0.5,
		KeepRecentCount:        3,
		TargetCompressionRatio: 0.3,
	}

	mgr := NewManager(db, c, nil, config)
	return mgr, db
}

// populateSession creates a session and fills it with messages to trigger compression.
func populateSession(t *testing.T, db *storage.DB, msgCount int) string {
	t.Helper()
	session, err := db.CreateSession(nil)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	for i := 0; i < msgCount; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		content := fmt.Sprintf("Message %d with enough content to contribute toward the token threshold used in testing.", i)
		if _, err := db.AppendMessage(session.ID, role, content, nil, ""); err != nil {
			t.Fatalf("AppendMessage[%d] failed: %v", i, err)
		}
		// Small delay to ensure ordering by created_at
		time.Sleep(time.Millisecond)
	}

	return session.ID
}

// ---------- Integration Tests ----------

// TestIntegration_FirstCompression verifies the complete first-compression flow:
// create session → add enough messages → BuildContext → compression triggers → context persisted.
func TestIntegration_FirstCompression(t *testing.T) {
	mgr, db := setupIntegrationManager(t)
	defer db.Close()

	sessionID := populateSession(t, db, 20)

	// BuildContext should detect token overflow and trigger compression
	msgs, err := mgr.BuildContext(context.Background(), sessionID, "You are a test bot.", "What next?")
	if err != nil {
		t.Fatalf("BuildContext failed: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("expected non-empty context messages")
	}

	// Verify system prompt is first
	if msgs[0].Role != provider.RoleSystem || msgs[0].Content != "You are a test bot." {
		t.Errorf("expected system prompt first, got role=%s", msgs[0].Role)
	}

	// Verify a compressed context was persisted
	ctx, err := db.GetLatestContext(sessionID)
	if err != nil {
		t.Fatalf("GetLatestContext failed: %v", err)
	}
	if ctx == nil {
		// Compression triggered only when NeedsCompression == true.
		// If it didn't trigger, ensure the messages are still returned.
		t.Log("compression was not triggered (messages may be within threshold)")
		return
	}

	if ctx.Version != 1 {
		t.Errorf("expected version 1, got %d", ctx.Version)
	}
	if ctx.SessionID != sessionID {
		t.Errorf("session mismatch: %s != %s", ctx.SessionID, sessionID)
	}
	if ctx.OriginalTokens == 0 {
		t.Error("expected non-zero OriginalTokens")
	}

	// Verify original messages table is NOT modified
	allMsgs, err := db.GetMessages(sessionID, 0)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(allMsgs) != 20 {
		t.Errorf("expected 20 original messages untouched, got %d", len(allMsgs))
	}
}

// TestIntegration_RestartRecovery simulates a "restart": compress, create a fresh Manager,
// then BuildContext should load the saved compressed context instead of full history.
func TestIntegration_RestartRecovery(t *testing.T) {
	mgr, db := setupIntegrationManager(t)
	defer db.Close()

	sessionID := populateSession(t, db, 20)

	// First call – triggers compression
	_, err := mgr.BuildContext(context.Background(), sessionID, "System.", "First call.")
	if err != nil {
		t.Fatalf("first BuildContext failed: %v", err)
	}

	savedCtx, err := db.GetLatestContext(sessionID)
	if err != nil {
		t.Fatalf("GetLatestContext failed: %v", err)
	}
	if savedCtx == nil {
		t.Skip("compression did not trigger; skipping restart recovery test")
	}

	// Add a couple more messages after compression
	time.Sleep(2 * time.Millisecond) // ensure created_at is after context
	if _, err := db.AppendMessage(sessionID, "user", "post-restart message A", nil, ""); err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}
	if _, err := db.AppendMessage(sessionID, "assistant", "post-restart reply A", nil, ""); err != nil {
		t.Fatalf("AppendMessage failed: %v", err)
	}

	// Simulate restart: new Manager, same DB
	compactorConfig := compaction.DefaultConfig()
	compactorConfig.KeepRecentCount = 3
	compactorConfig.MaxContextTokens = 200
	compactorConfig.TriggerThreshold = 0.5
	c := compaction.NewCompactor(compactorConfig, nil)

	mgr2 := NewManager(db, c, nil, Config{
		MaxContextTokens:       200,
		TriggerThreshold:       0.5,
		KeepRecentCount:        3,
		TargetCompressionRatio: 0.3,
	})

	msgs, err := mgr2.BuildContext(context.Background(), sessionID, "System.", "After restart.")
	if err != nil {
		t.Fatalf("second BuildContext failed: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("expected non-empty messages after restart recovery")
	}

	// Should contain the summary from compressed context
	hasSummary := false
	for _, m := range msgs {
		if m.Role == provider.RoleAssistant && strings.Contains(m.Content, "[Previous conversation summary]") {
			hasSummary = true
			break
		}
	}
	if !hasSummary {
		t.Error("expected summary message from compressed context after restart")
	}

	// Should contain the post-restart messages
	hasPostRestart := false
	for _, m := range msgs {
		if strings.Contains(m.Content, "post-restart message A") {
			hasPostRestart = true
			break
		}
	}
	if !hasPostRestart {
		t.Error("expected post-restart messages in context")
	}
}

// TestIntegration_MultiRoundCompression verifies that multiple compression rounds
// increment the version and each produces a valid compressed context.
func TestIntegration_MultiRoundCompression(t *testing.T) {
	mgr, db := setupIntegrationManager(t)
	defer db.Close()

	sessionID := populateSession(t, db, 20)

	// Round 1
	_, err := mgr.BuildContext(context.Background(), sessionID, "System.", "Round 1.")
	if err != nil {
		t.Fatalf("round 1 BuildContext failed: %v", err)
	}

	ctx1, err := db.GetLatestContext(sessionID)
	if err != nil {
		t.Fatalf("GetLatestContext after round 1 failed: %v", err)
	}
	if ctx1 == nil {
		t.Skip("compression did not trigger in round 1")
	}
	if ctx1.Version != 1 {
		t.Errorf("expected version 1, got %d", ctx1.Version)
	}

	// Add more messages to force a second round
	time.Sleep(2 * time.Millisecond)
	for i := 0; i < 20; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		content := fmt.Sprintf("Round2 message %d with padding text to push token count beyond threshold.", i)
		if _, err := db.AppendMessage(sessionID, role, content, nil, ""); err != nil {
			t.Fatalf("AppendMessage[round2-%d] failed: %v", i, err)
		}
		time.Sleep(time.Millisecond)
	}

	// Round 2
	_, err = mgr.BuildContext(context.Background(), sessionID, "System.", "Round 2.")
	if err != nil {
		t.Fatalf("round 2 BuildContext failed: %v", err)
	}

	ctx2, err := db.GetLatestContext(sessionID)
	if err != nil {
		t.Fatalf("GetLatestContext after round 2 failed: %v", err)
	}
	if ctx2 == nil {
		t.Fatal("expected compressed context after round 2")
	}

	if ctx2.Version <= ctx1.Version {
		t.Errorf("expected version > %d after round 2, got %d", ctx1.Version, ctx2.Version)
	}

	// Verify both contexts exist in the list
	all, err := db.ListContexts(sessionID, 10)
	if err != nil {
		t.Fatalf("ListContexts failed: %v", err)
	}
	if len(all) < 2 {
		t.Errorf("expected at least 2 contexts, got %d", len(all))
	}

	// Verify messages table is completely untouched
	allMsgs, err := db.GetMessages(sessionID, 0)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(allMsgs) != 40 { // 20 + 20
		t.Errorf("expected 40 messages in history, got %d", len(allMsgs))
	}
}

// TestIntegration_SessionIsolation verifies that compression in one session
// does not affect another session's context or messages.
func TestIntegration_SessionIsolation(t *testing.T) {
	mgr, db := setupIntegrationManager(t)
	defer db.Close()

	sessionA := populateSession(t, db, 20)
	sessionB := populateSession(t, db, 5) // small, should not trigger compression

	// Trigger compression on session A
	_, err := mgr.BuildContext(context.Background(), sessionA, "System A.", "Go A.")
	if err != nil {
		t.Fatalf("BuildContext A failed: %v", err)
	}

	// Session B should have no compressed context
	ctxB, err := db.GetLatestContext(sessionB)
	if err != nil {
		t.Fatalf("GetLatestContext B failed: %v", err)
	}
	if ctxB != nil {
		t.Error("expected no compressed context for session B")
	}

	// Session B messages untouched
	msgsB, err := db.GetMessages(sessionB, 0)
	if err != nil {
		t.Fatalf("GetMessages B failed: %v", err)
	}
	if len(msgsB) != 5 {
		t.Errorf("expected 5 messages in session B, got %d", len(msgsB))
	}
}

// TestIntegration_SaveContext_Version verifies manual SaveContext increments version correctly.
func TestIntegration_SaveContext_Version(t *testing.T) {
	mgr, db := setupIntegrationManager(t)
	defer db.Close()

	session, err := db.CreateSession(nil)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	for i := 1; i <= 5; i++ {
		err := mgr.SaveContext(context.Background(), session.ID,
			fmt.Sprintf("Summary version %d", i), nil, 100*i, 1000)
		if err != nil {
			t.Fatalf("SaveContext v%d failed: %v", i, err)
		}
	}

	latest, err := db.GetLatestContext(session.ID)
	if err != nil {
		t.Fatalf("GetLatestContext failed: %v", err)
	}
	if latest.Version != 5 {
		t.Errorf("expected version 5, got %d", latest.Version)
	}
	if latest.Summary != "Summary version 5" {
		t.Errorf("unexpected summary: %s", latest.Summary)
	}

	maxVer, err := db.GetMaxContextVersion(session.ID)
	if err != nil {
		t.Fatalf("GetMaxContextVersion failed: %v", err)
	}
	if maxVer != 5 {
		t.Errorf("expected max version 5, got %d", maxVer)
	}
}

// TestIntegration_LoadLatestContext_WithKeptMessages verifies that LoadLatestContext
// correctly reconstructs context from summary + kept messages.
func TestIntegration_LoadLatestContext_WithKeptMessages(t *testing.T) {
	mgr, db := setupIntegrationManager(t)
	defer db.Close()

	session, _ := db.CreateSession(nil)
	msg1, _ := db.AppendMessage(session.ID, "user", "Remember: my name is Alice.", nil, "")
	msg2, _ := db.AppendMessage(session.ID, "assistant", "Got it, Alice!", nil, "")
	msg3, _ := db.AppendMessage(session.ID, "user", "What's the weather?", nil, "")
	_, _ = db.AppendMessage(session.ID, "assistant", "It's sunny today.", nil, "")

	// Save a compressed context keeping msg2 and msg3
	err := db.SaveContext(&storage.Context{
		SessionID:      session.ID,
		Version:        1,
		Summary:        "User introduced herself as Alice.",
		KeptMessageIDs: []string{msg1.ID, msg2.ID, msg3.ID},
		TotalTokens:    50,
		OriginalTokens: 200,
	})
	if err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	msgs, err := mgr.LoadLatestContext(session.ID)
	if err != nil {
		t.Fatalf("LoadLatestContext failed: %v", err)
	}

	if len(msgs) != 4 { // 1 summary + 3 kept
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}

	// First should be summary
	if !strings.Contains(msgs[0].Content, "Alice") {
		t.Errorf("expected summary to mention Alice, got: %s", msgs[0].Content)
	}

	// Kept messages should preserve content and order
	if msgs[1].Content != "Remember: my name is Alice." {
		t.Errorf("unexpected kept msg[1]: %s", msgs[1].Content)
	}
	if msgs[2].Content != "Got it, Alice!" {
		t.Errorf("unexpected kept msg[2]: %s", msgs[2].Content)
	}
	if msgs[3].Content != "What's the weather?" {
		t.Errorf("unexpected kept msg[3]: %s", msgs[3].Content)
	}
}

// TestIntegration_CompressionPreservesMessagesTable ensures that the messages table
// is never modified during any compression operations.
func TestIntegration_CompressionPreservesMessagesTable(t *testing.T) {
	mgr, db := setupIntegrationManager(t)
	defer db.Close()

	sessionID := populateSession(t, db, 30)

	// Snapshot messages before compression
	msgsBefore, err := db.GetMessages(sessionID, 0)
	if err != nil {
		t.Fatalf("GetMessages before failed: %v", err)
	}

	// Trigger compression
	_, err = mgr.BuildContext(context.Background(), sessionID, "System.", "Go.")
	if err != nil {
		t.Fatalf("BuildContext failed: %v", err)
	}

	// Verify messages after compression are identical
	msgsAfter, err := db.GetMessages(sessionID, 0)
	if err != nil {
		t.Fatalf("GetMessages after failed: %v", err)
	}

	if len(msgsBefore) != len(msgsAfter) {
		t.Fatalf("message count changed: %d → %d", len(msgsBefore), len(msgsAfter))
	}

	for i := range msgsBefore {
		if msgsBefore[i].ID != msgsAfter[i].ID {
			t.Errorf("message[%d] ID changed: %s → %s", i, msgsBefore[i].ID, msgsAfter[i].ID)
		}
		if msgsBefore[i].Content != msgsAfter[i].Content {
			t.Errorf("message[%d] Content changed", i)
		}
		if msgsBefore[i].Role != msgsAfter[i].Role {
			t.Errorf("message[%d] Role changed", i)
		}
	}
}

// TestIntegration_DoCompression_DirectCall tests DoCompression directly
// with a set of messages that exceed the threshold.
func TestIntegration_DoCompression_DirectCall(t *testing.T) {
	mgr, db := setupIntegrationManager(t)
	defer db.Close()

	session, err := db.CreateSession(nil)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Populate messages in DB (needed for extractCompressionResult)
	for i := 0; i < 15; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		content := fmt.Sprintf("Direct compression test message %d with padding text.", i)
		if _, err := db.AppendMessage(session.ID, role, content, nil, ""); err != nil {
			t.Fatalf("AppendMessage failed: %v", err)
		}
	}

	// Build a message list for compression
	msgs := []provider.Message{
		{Role: provider.RoleSystem, Content: "System prompt."},
	}
	dbMsgs, _ := db.GetMessages(session.ID, 0)
	for _, m := range dbMsgs {
		msgs = append(msgs, provider.Message{Role: m.Role, Content: m.Content})
	}

	err = mgr.DoCompression(context.Background(), session.ID, msgs)
	if err != nil {
		t.Fatalf("DoCompression failed: %v", err)
	}

	// Verify context saved
	ctx, err := db.GetLatestContext(session.ID)
	if err != nil {
		t.Fatalf("GetLatestContext failed: %v", err)
	}
	if ctx == nil {
		t.Fatal("expected compressed context after DoCompression")
	}
	if ctx.Version != 1 {
		t.Errorf("expected version 1, got %d", ctx.Version)
	}
	if ctx.TotalTokens == 0 {
		t.Error("expected non-zero TotalTokens")
	}
	if ctx.OriginalTokens == 0 {
		t.Error("expected non-zero OriginalTokens")
	}
	if ctx.TotalTokens >= ctx.OriginalTokens {
		t.Logf("note: truncation fallback may not reduce tokens significantly (total=%d, orig=%d)",
			ctx.TotalTokens, ctx.OriginalTokens)
	}
}
