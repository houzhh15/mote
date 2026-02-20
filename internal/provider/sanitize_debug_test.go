package provider

import (
	"testing"
)

func TestSanitizeMessages_EmptyContentNoToolCalls(t *testing.T) {
	messages := []Message{
		{
			Role:      RoleAssistant,
			Content:   "",
			ToolCalls: []ToolCall{},
		},
	}

	result := SanitizeMessages(messages)
	if len(result) != 0 {
		t.Errorf("Expected 0 messages, got %d. Message with empty content and no tool calls should have been removed.", len(result))
		if len(result) > 0 {
			t.Logf("Kept message: %+v", result[0])
		}
	}
}

func TestSanitizeMessages_AssistantWithValidToolCall(t *testing.T) {
	tc := ToolCall{
		ID:   "call_123",
		Type: "function",
		Function: &struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{
			Name:      "test_tool",
			Arguments: `{"arg": "value"}`,
		},
	}

	// Assistant with tool_calls but no following tool results is an incomplete
	// group — enforceToolCallOrdering removes it to prevent provider errors.
	messages := []Message{
		{
			Role:      RoleAssistant,
			Content:   "",
			ToolCalls: []ToolCall{tc},
		},
	}
	result := SanitizeMessages(messages)
	if len(result) != 0 {
		t.Errorf("Expected 0 messages (incomplete tool_call group removed), got %d", len(result))
	}

	// With a matching tool result immediately following, the group is valid.
	messagesWithResult := []Message{
		{
			Role:      RoleAssistant,
			Content:   "",
			ToolCalls: []ToolCall{tc},
		},
		{
			Role:       RoleTool,
			Content:    "result",
			ToolCallID: "call_123",
		},
	}
	result2 := SanitizeMessages(messagesWithResult)
	if len(result2) != 2 {
		t.Errorf("Expected 2 messages (complete tool_call group kept), got %d", len(result2))
	}
}
