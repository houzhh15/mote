package provider

import (
	"testing"
)

func TestSanitizeMessages_NoToolCalls(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "hello"},
		{Role: RoleAssistant, Content: "hi there"},
	}
	result := SanitizeMessages(msgs)
	if len(result) != 2 {
		t.Errorf("expected 2 messages, got %d", len(result))
	}
}

func TestSanitizeMessages_ValidToolCalls(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "do something"},
		{Role: RoleAssistant, Content: "", ToolCalls: []ToolCall{
			{ID: "call_1", Name: "test_tool", Arguments: `{"key": "value"}`},
		}},
		{Role: RoleTool, Content: "result", ToolCallID: "call_1"},
		{Role: RoleAssistant, Content: "done"},
	}
	result := SanitizeMessages(msgs)
	if len(result) != 4 {
		t.Errorf("expected 4 messages, got %d", len(result))
	}
}

func TestSanitizeMessages_InvalidToolCallArgs(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "do something"},
		{Role: RoleAssistant, Content: "", ToolCalls: []ToolCall{
			{ID: "call_1", Name: "test_tool", Arguments: `{"arguments": `}, // truncated JSON
		}},
		{Role: RoleTool, Content: "error result", ToolCallID: "call_1"},
		{Role: RoleUser, Content: "try again"},
	}
	result := SanitizeMessages(msgs)
	// Should remove: assistant with invalid tool call (no content) + orphaned tool result
	if len(result) != 2 {
		t.Errorf("expected 2 messages, got %d", len(result))
		for i, m := range result {
			t.Logf("  msg[%d]: role=%s content=%q toolCalls=%d toolCallID=%s",
				i, m.Role, m.Content, len(m.ToolCalls), m.ToolCallID)
		}
	}
}

func TestSanitizeMessages_MixedValidInvalid(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "do two things"},
		{Role: RoleAssistant, Content: "I'll do both", ToolCalls: []ToolCall{
			{ID: "call_1", Name: "good_tool", Arguments: `{"a": 1}`},
			{ID: "call_2", Name: "bad_tool", Arguments: `{"b": `}, // truncated
		}},
		{Role: RoleTool, Content: "result1", ToolCallID: "call_1"},
		{Role: RoleTool, Content: "result2", ToolCallID: "call_2"},
		{Role: RoleAssistant, Content: "done"},
	}
	result := SanitizeMessages(msgs)
	// Should keep: user, assistant (with only call_1), tool result for call_1, assistant "done"
	// Should remove: tool result for call_2
	if len(result) != 4 {
		t.Errorf("expected 4 messages, got %d", len(result))
		for i, m := range result {
			t.Logf("  msg[%d]: role=%s content=%q toolCalls=%d toolCallID=%s",
				i, m.Role, m.Content, len(m.ToolCalls), m.ToolCallID)
		}
	}
	// Check that assistant still has content and one valid tool call
	if result[1].Role != RoleAssistant || len(result[1].ToolCalls) != 1 {
		t.Errorf("expected assistant with 1 tool call, got role=%s toolCalls=%d",
			result[1].Role, len(result[1].ToolCalls))
	}
}

func TestSanitizeMessages_EmptyArgs(t *testing.T) {
	msgs := []Message{
		{Role: RoleAssistant, Content: "", ToolCalls: []ToolCall{
			{ID: "call_1", Name: "no_args_tool", Arguments: ""},
		}},
		{Role: RoleTool, Content: "result", ToolCallID: "call_1"},
	}
	result := SanitizeMessages(msgs)
	// Empty arguments are valid
	if len(result) != 2 {
		t.Errorf("expected 2 messages, got %d", len(result))
	}
}

func TestSanitizeMessages_FunctionField(t *testing.T) {
	fnField := &struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}{Name: "tool", Arguments: `{"broken`}
	msgs := []Message{
		{Role: RoleAssistant, Content: "", ToolCalls: []ToolCall{
			{ID: "call_1", Name: "tool", Arguments: "", Function: fnField},
		}},
		{Role: RoleTool, Content: "result", ToolCallID: "call_1"},
	}
	result := SanitizeMessages(msgs)
	// Function.Arguments is invalid, should be removed
	if len(result) != 0 {
		t.Errorf("expected 0 messages, got %d", len(result))
	}
}

func TestSanitizeMessages_Empty(t *testing.T) {
	result := SanitizeMessages(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
	result = SanitizeMessages([]Message{})
	if len(result) != 0 {
		t.Errorf("expected empty, got %d", len(result))
	}
}
