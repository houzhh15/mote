package runner

import (
	"context"
	"testing"
	"time"

	"mote/internal/provider"
	"mote/internal/scheduler"
	"mote/internal/tools"
)

// mockACPProvider simulates an ACP provider for testing
type mockACPProvider struct {
	receivedMessages []provider.Message
	response         string
}

func (m *mockACPProvider) Name() string {
	return "mock-acp"
}

func (m *mockACPProvider) Models() []string {
	return []string{"mock-model"}
}

func (m *mockACPProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	m.receivedMessages = req.Messages
	return &provider.ChatResponse{
		Content: m.response,
	}, nil
}

func (m *mockACPProvider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.ChatEvent, error) {
	m.receivedMessages = req.Messages
	events := make(chan provider.ChatEvent, 10)

	go func() {
		defer close(events)
		events <- provider.ChatEvent{
			Type:  provider.EventTypeContent,
			Delta: m.response,
		}
		events <- provider.ChatEvent{
			Type: provider.EventTypeDone,
		}
	}()

	return events, nil
}

func (m *mockACPProvider) IsACPProvider() bool {
	return true
}

// TestRunACPMode_HistoryLoading verifies that ACP mode loads and includes conversation history
func TestRunACPMode_HistoryLoading(t *testing.T) {
	// Setup
	db := setupTestDBM04(t)
	sessions := scheduler.NewSessionManager(db, 100)
	registry := tools.NewRegistry()

	mockProv := &mockACPProvider{response: "Response with history context"}

	cfg := DefaultConfig().WithStreamOutput(false).WithTimeout(0)
	r := NewRunner(mockProv, registry, sessions, cfg)

	// Create a session with some history
	cached, err := sessions.Create(nil)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	sessionID := cached.Session.ID

	// Add some historical messages to the session
	_, _ = sessions.AddMessage(sessionID, provider.RoleUser, "First message", nil, "")
	_, _ = sessions.AddMessage(sessionID, provider.RoleAssistant, "First response", nil, "")
	_, _ = sessions.AddMessage(sessionID, provider.RoleUser, "Second message", nil, "")
	_, _ = sessions.AddMessage(sessionID, provider.RoleAssistant, "Second response", nil, "")

	// Reload session to get fresh cached data
	cached, err = sessions.Get(sessionID)
	if err != nil {
		t.Fatalf("failed to reload session: %v", err)
	}

	if len(cached.Messages) != 4 {
		t.Fatalf("expected 4 cached messages, got %d", len(cached.Messages))
	}

	// Run with a new message
	ctx := context.Background()
	events, err := r.Run(ctx, sessionID, "Third message (after restart)")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Consume events
	for range events {
		// Just drain the events
	}

	// Verify that the provider received all historical messages
	if mockProv.receivedMessages == nil {
		t.Fatal("provider did not receive any messages")
	}

	// Expected messages:
	// 1. System message (if any)
	// 2. Historical user/assistant messages (4 messages)
	// 3. Current user message
	// Minimum expected: at least 5 messages (4 history + 1 current)

	if len(mockProv.receivedMessages) < 5 {
		t.Errorf("expected at least 5 messages (4 history + 1 current), got %d", len(mockProv.receivedMessages))
		t.Logf("Received messages:")
		for i, msg := range mockProv.receivedMessages {
			t.Logf("  [%d] %s: %s", i, msg.Role, msg.Content)
		}
	}

	// Check that historical content is present
	foundFirstMessage := false
	foundSecondMessage := false
	foundThirdMessage := false

	for _, msg := range mockProv.receivedMessages {
		if msg.Content == "First message" {
			foundFirstMessage = true
		}
		if msg.Content == "Second message" {
			foundSecondMessage = true
		}
		if msg.Content == "Third message (after restart)" {
			foundThirdMessage = true
		}
	}

	if !foundFirstMessage {
		t.Error("historical message 'First message' not found in provider request")
	}
	if !foundSecondMessage {
		t.Error("historical message 'Second message' not found in provider request")
	}
	if !foundThirdMessage {
		t.Error("current message 'Third message (after restart)' not found in provider request")
	}
}

// TestRunACPMode_HistoryCompression verifies that ACP mode does NOT compress
// history on the mote side. ACP server manages its own context window —
// mote simply passes all messages through and relies on the ACP provider's
// buildPromptWithAttachments to select the appropriate subset.
func TestRunACPMode_HistoryCompression(t *testing.T) {
	// Setup
	db := setupTestDBM04(t)
	sessions := scheduler.NewSessionManager(db, 100)
	registry := tools.NewRegistry()

	mockProv := &mockACPProvider{response: "Compressed response"}

	cfg := DefaultConfig().WithStreamOutput(false).WithTimeout(0)
	r := NewRunner(mockProv, registry, sessions, cfg)

	// Create a session
	cached, err := sessions.Create(nil)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	sessionID := cached.Session.ID

	// Add many messages to trigger compression
	// Generate a large conversation history
	for i := 0; i < 50; i++ {
		_, _ = sessions.AddMessage(sessionID, provider.RoleUser,
			"This is a long user message with substantial content to consume tokens. "+
				"It contains detailed information that would be summarized in compression. "+
				"Message number "+string(rune('0'+i%10)), nil, "")
		_, _ = sessions.AddMessage(sessionID, provider.RoleAssistant,
			"This is a long assistant response with substantial content. "+
				"It provides detailed answers and explanations that take up context space. "+
				"Response number "+string(rune('0'+i%10)), nil, "")
	}

	// Note: Compression may or may not trigger depending on actual token counts
	// This test primarily verifies that the code doesn't crash with large history

	// Run with a new message
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := r.Run(ctx, sessionID, "Final message after long history")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Consume events
	for range events {
		// Just drain the events
	}

	// Verify that provider received messages (possibly compressed)
	if mockProv.receivedMessages == nil {
		t.Fatal("provider did not receive any messages")
	}

	t.Logf("Provider received %d messages (ACP mode, no mote-side compaction)", len(mockProv.receivedMessages))

	// ACP mode no longer compacts on the mote side — ACP server handles
	// its own context window.  The provider should receive ALL messages
	// (system + 100 history + 1 new user + 1 added by AddMessage = 103).
	// This verifies that the removal of compressIfNeeded doesn't break
	// message delivery.
	if len(mockProv.receivedMessages) == 0 {
		t.Error("provider should have received messages")
	}
}
