package message

import (
	"context"
	"testing"

	"mote/internal/provider"
	"mote/internal/scheduler"
	"mote/internal/storage"
)

func TestStandardBuilder_BuildMessages_MinimalDefault(t *testing.T) {
	builder := NewStandardBuilder()

	cached := &scheduler.CachedSession{
		Session: &storage.Session{
			ID: "test-session",
		},
		Messages: []*storage.Message{},
	}

	request := &BuildRequest{
		SessionID:     "test-session",
		UserInput:     "Hello",
		CachedSession: cached,
	}

	messages, err := builder.BuildMessages(context.Background(), request)
	if err != nil {
		t.Fatalf("BuildMessages failed: %v", err)
	}

	// Should have system message + user input
	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}

	// Check system message
	if messages[0].Role != provider.RoleSystem {
		t.Errorf("expected first message to be system, got %s", messages[0].Role)
	}
	if messages[0].Content != "You are a helpful AI assistant." {
		t.Errorf("unexpected system prompt: %s", messages[0].Content)
	}

	// Check user message
	if messages[1].Role != provider.RoleUser {
		t.Errorf("expected second message to be user, got %s", messages[1].Role)
	}
	if messages[1].Content != "Hello" {
		t.Errorf("expected user content 'Hello', got '%s'", messages[1].Content)
	}
}

func TestStandardBuilder_BuildMessages_WithStaticPrompt(t *testing.T) {
	builder := NewStandardBuilder()
	builder.SetStaticPrompt("Custom system prompt")

	cached := &scheduler.CachedSession{
		Session: &storage.Session{
			ID: "test-session",
		},
		Messages: []*storage.Message{},
	}

	request := &BuildRequest{
		SessionID:     "test-session",
		UserInput:     "Test",
		CachedSession: cached,
	}

	messages, err := builder.BuildMessages(context.Background(), request)
	if err != nil {
		t.Fatalf("BuildMessages failed: %v", err)
	}

	if messages[0].Content != "Custom system prompt" {
		t.Errorf("expected custom prompt, got: %s", messages[0].Content)
	}
}

func TestStandardBuilder_BuildMessages_WithHistory(t *testing.T) {
	builder := NewStandardBuilder()

	cached := &scheduler.CachedSession{
		Session: &storage.Session{
			ID: "test-session",
		},
		Messages: []*storage.Message{
			{
				Role:    provider.RoleUser,
				Content: "Previous question",
			},
			{
				Role:    provider.RoleAssistant,
				Content: "Previous answer",
			},
		},
	}

	request := &BuildRequest{
		SessionID:     "test-session",
		UserInput:     "New question",
		CachedSession: cached,
	}

	messages, err := builder.BuildMessages(context.Background(), request)
	if err != nil {
		t.Fatalf("BuildMessages failed: %v", err)
	}

	// Should have: system + 2 history + current user
	if len(messages) != 4 {
		t.Errorf("expected 4 messages, got %d", len(messages))
	}

	// Verify order
	if messages[0].Role != provider.RoleSystem {
		t.Error("expected system message first")
	}
	if messages[1].Role != provider.RoleUser || messages[1].Content != "Previous question" {
		t.Error("expected previous user message second")
	}
	if messages[2].Role != provider.RoleAssistant || messages[2].Content != "Previous answer" {
		t.Error("expected previous assistant message third")
	}
	if messages[3].Role != provider.RoleUser || messages[3].Content != "New question" {
		t.Error("expected current user message last")
	}
}

func TestStandardBuilder_BuildMessages_RequestOverride(t *testing.T) {
	builder := NewStandardBuilder()
	builder.SetStaticPrompt("Default prompt")

	cached := &scheduler.CachedSession{
		Session: &storage.Session{
			ID: "test-session",
		},
		Messages: []*storage.Message{},
	}

	request := &BuildRequest{
		SessionID:     "test-session",
		UserInput:     "Test",
		CachedSession: cached,
		SystemPrompt:  "Override prompt", // Request-level override
	}

	messages, err := builder.BuildMessages(context.Background(), request)
	if err != nil {
		t.Fatalf("BuildMessages failed: %v", err)
	}

	// Request override should take precedence
	if messages[0].Content != "Override prompt" {
		t.Errorf("expected override prompt, got: %s", messages[0].Content)
	}
}

func TestStandardBuilder_BuildMessages_WithToolCalls(t *testing.T) {
	builder := NewStandardBuilder()

	// Create tool call JSON
	toolCallJSON := []byte(`{"name":"test_tool","arguments":"{}"}`)

	cached := &scheduler.CachedSession{
		Session: &storage.Session{
			ID: "test-session",
		},
		Messages: []*storage.Message{
			{
				Role:    provider.RoleAssistant,
				Content: "",
				ToolCalls: []storage.ToolCall{
					{
						ID:       "call_123",
						Type:     "function",
						Function: toolCallJSON,
					},
				},
			},
			{
				Role:       provider.RoleTool,
				Content:    "Tool result",
				ToolCallID: "call_123",
			},
		},
	}

	request := &BuildRequest{
		SessionID:     "test-session",
		UserInput:     "Follow up",
		CachedSession: cached,
	}

	messages, err := builder.BuildMessages(context.Background(), request)
	if err != nil {
		t.Fatalf("BuildMessages failed: %v", err)
	}

	// Should have: system + assistant with tool call + tool result + user
	if len(messages) != 4 {
		t.Errorf("expected 4 messages, got %d", len(messages))
	}

	// Check assistant message has tool calls
	assistantMsg := messages[1]
	if assistantMsg.Role != provider.RoleAssistant {
		t.Errorf("expected assistant message, got %s", assistantMsg.Role)
	}
	if len(assistantMsg.ToolCalls) != 1 {
		t.Errorf("expected 1 tool call, got %d", len(assistantMsg.ToolCalls))
	}
	if assistantMsg.ToolCalls[0].ID != "call_123" {
		t.Errorf("expected tool call ID 'call_123', got '%s'", assistantMsg.ToolCalls[0].ID)
	}

	// Check tool result
	toolMsg := messages[2]
	if toolMsg.Role != provider.RoleTool {
		t.Errorf("expected tool message, got %s", toolMsg.Role)
	}
	if toolMsg.ToolCallID != "call_123" {
		t.Errorf("expected tool call ID 'call_123', got '%s'", toolMsg.ToolCallID)
	}
}
