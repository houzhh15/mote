package runner

import (
	"context"
	"os"
	"testing"

	"mote/internal/compaction"
	"mote/internal/provider"
	"mote/internal/scheduler"
	"mote/internal/storage"
	"mote/internal/tools"
)

// setupTestDBM04 creates a temporary database for testing.
func setupTestDBM04(t *testing.T) *storage.DB {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "runner_m04_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })
	tmpFile.Close()

	db, err := storage.Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return db
}

// mockProviderM04 implements provider.Provider for M04 tests.
type mockProviderM04 struct {
	responses []provider.ChatResponse
	callCount int
}

func (m *mockProviderM04) Name() string     { return "mock-m04" }
func (m *mockProviderM04) Models() []string { return []string{"mock-model"} }

func (m *mockProviderM04) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	if m.callCount >= len(m.responses) {
		return &provider.ChatResponse{Content: "done"}, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return &resp, nil
}

func (m *mockProviderM04) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.ChatEvent, error) {
	ch := make(chan provider.ChatEvent)
	close(ch)
	return ch, nil
}

func TestRunner_SetCompactor(t *testing.T) {
	db := setupTestDBM04(t)

	sessions := scheduler.NewSessionManager(db, 100)
	registry := tools.NewRegistry()
	prov := &mockProviderM04{
		responses: []provider.ChatResponse{{Content: "Compacted response"}},
	}

	runCfg := DefaultConfig().WithStreamOutput(false).WithTimeout(0)
	r := NewRunner(prov, registry, sessions, runCfg)

	// Create compactor with very low threshold to trigger compression
	compCfg := compaction.CompactionConfig{
		MaxContextTokens: 100,
		TriggerThreshold: 0.1,
		KeepRecentCount:  2,
		SummaryMaxTokens: 50,
		ChunkMaxTokens:   50,
	}
	comp := compaction.NewCompactor(compCfg, prov)
	r.SetCompactor(comp)

	// Create a session first
	cached, err := sessions.Create(nil)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	sessionID := cached.Session.ID

	// Run a query
	events, err := r.Run(context.Background(), sessionID, "Test input")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Consume events
	for range events {
	}

	// Verify compactor was set
	if r.compactor == nil {
		t.Error("compactor should be set")
	}
}

func TestRunner_WithoutSystemPrompt(t *testing.T) {
	db := setupTestDBM04(t)

	sessions := scheduler.NewSessionManager(db, 100)
	registry := tools.NewRegistry()
	prov := &mockProviderM04{
		responses: []provider.ChatResponse{{Content: "Default response"}},
	}

	cfg := DefaultConfig().WithStreamOutput(false).WithTimeout(0)
	r := NewRunner(prov, registry, sessions, cfg)
	// Don't set systemPrompt - should use minimal default prompt

	// Create a session first
	cached, err := sessions.Create(nil)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	sessionID := cached.Session.ID

	events, err := r.Run(context.Background(), sessionID, "Hi")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Consume events
	var gotDone bool
	var eventTypes []string
	for ev := range events {
		eventTypes = append(eventTypes, ev.Type.String())
		if ev.Type == EventTypeDone {
			gotDone = true
		}
		if ev.Type == EventTypeError {
			t.Logf("Got error event: %v", ev.Error)
		}
	}

	t.Logf("Got event types: %v", eventTypes)

	if !gotDone {
		t.Error("expected done event")
	}

	// Verify systemPrompt is nil (using minimal default)
	if r.systemPrompt != nil {
		t.Error("systemPrompt should be nil when not explicitly set")
	}
}
