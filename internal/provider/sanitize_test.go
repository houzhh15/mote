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

func TestSanitizeMessages_ToolResultSeparatedFromOwner(t *testing.T) {
	// Scenario: after context compression, a tool result ends up separated
	// from its assistant message by an intervening user message.
	// This triggers MiniMax "tool call result does not follow tool call" error.
	msgs := []Message{
		{Role: RoleSystem, Content: "system prompt"},
		{Role: RoleAssistant, Content: "I'll read the file", ToolCalls: []ToolCall{
			{ID: "call_1", Name: "read_file", Arguments: `{"path": "foo.txt"}`},
		}},
		{Role: RoleUser, Content: "interrupting message"}, // breaks the pair
		{Role: RoleTool, Content: "file contents", ToolCallID: "call_1"},
		{Role: RoleAssistant, Content: "done"},
	}
	result := SanitizeMessages(msgs)
	// The tool result AND the assistant's tool_calls should be cleaned up.
	// The assistant with content "I'll read the file" should be kept but without tool_calls.
	for _, m := range result {
		if m.Role == RoleTool {
			t.Errorf("expected tool result to be removed, but found role=tool with callID=%s", m.ToolCallID)
		}
		if m.Role == RoleAssistant && len(m.ToolCalls) > 0 {
			t.Errorf("expected assistant tool_calls to be stripped, but found %d tool_calls", len(m.ToolCalls))
		}
	}
	// The assistant with content should still be present (just without tool_calls)
	hasAssistantContent := false
	for _, m := range result {
		if m.Role == RoleAssistant && m.Content == "I'll read the file" {
			hasAssistantContent = true
		}
	}
	if !hasAssistantContent {
		t.Error("expected assistant with content to be kept (without tool_calls)")
	}
}

func TestSanitizeMessages_ToolResultSeparatedNoContent(t *testing.T) {
	// Same as above but assistant has no content — should be removed entirely
	msgs := []Message{
		{Role: RoleSystem, Content: "system prompt"},
		{Role: RoleAssistant, Content: "", ToolCalls: []ToolCall{
			{ID: "call_1", Name: "read_file", Arguments: `{"path": "foo.txt"}`},
		}},
		{Role: RoleUser, Content: "interrupting message"},
		{Role: RoleTool, Content: "file contents", ToolCallID: "call_1"},
		{Role: RoleAssistant, Content: "done"},
	}
	result := SanitizeMessages(msgs)
	// Both the empty assistant+tool_calls and the displaced tool result should be gone
	if len(result) != 3 { // system + user + "done" assistant
		t.Errorf("expected 3 messages, got %d", len(result))
		for i, m := range result {
			t.Logf("  msg[%d]: role=%s content=%q toolCalls=%d", i, m.Role, m.Content, len(m.ToolCalls))
		}
	}
}

func TestSanitizeMessages_ToolResultAfterSummary(t *testing.T) {
	// Scenario: compressed context has summary assistant (no tool_calls)
	// followed by tool results from kept messages
	msgs := []Message{
		{Role: RoleSystem, Content: "system"},
		{Role: RoleAssistant, Content: "[Previous conversation summary]\nSome summary"},
		{Role: RoleTool, Content: "orphaned tool result", ToolCallID: "old_call_1"},
		{Role: RoleUser, Content: "hello"},
	}
	result := SanitizeMessages(msgs)
	// Tool result has no matching assistant with tool_calls → should be removed
	for _, m := range result {
		if m.Role == RoleTool {
			t.Errorf("expected orphaned tool result to be removed")
		}
	}
}

func TestSanitizeMessages_MultipleToolResultsGrouped(t *testing.T) {
	// Multiple tool results properly following their assistant message
	msgs := []Message{
		{Role: RoleUser, Content: "do two things"},
		{Role: RoleAssistant, Content: "", ToolCalls: []ToolCall{
			{ID: "call_1", Name: "tool_a", Arguments: `{}`},
			{ID: "call_2", Name: "tool_b", Arguments: `{}`},
		}},
		{Role: RoleTool, Content: "result a", ToolCallID: "call_1"},
		{Role: RoleTool, Content: "result b", ToolCallID: "call_2"},
		{Role: RoleAssistant, Content: "done"},
	}
	result := SanitizeMessages(msgs)
	if len(result) != 5 {
		t.Errorf("expected 5 messages, got %d", len(result))
	}
}

func TestSanitizeMessages_ToolResultBeforeOwner(t *testing.T) {
	// Tool result appears before its owning assistant message (impossible
	// in normal flow but can happen with context merging)
	msgs := []Message{
		{Role: RoleSystem, Content: "system"},
		{Role: RoleTool, Content: "premature result", ToolCallID: "call_future"},
		{Role: RoleAssistant, Content: "", ToolCalls: []ToolCall{
			{ID: "call_future", Name: "tool", Arguments: `{}`},
		}},
	}
	result := SanitizeMessages(msgs)
	// The premature tool result should be dropped, and the assistant with
	// no content and incomplete tool_call group should also be dropped
	for _, m := range result {
		if m.Role == RoleTool {
			t.Error("expected premature tool result to be removed")
		}
		if m.Role == RoleAssistant && len(m.ToolCalls) > 0 {
			t.Error("expected assistant with orphaned tool_calls to be removed")
		}
	}
	if len(result) != 1 { // only system
		t.Errorf("expected 1 message (system only), got %d", len(result))
	}
}

func TestSanitizeMessages_PartialToolResults(t *testing.T) {
	// Assistant has 2 tool_calls but only 1 result follows
	msgs := []Message{
		{Role: RoleUser, Content: "do two things"},
		{Role: RoleAssistant, Content: "I'll try both", ToolCalls: []ToolCall{
			{ID: "call_1", Name: "tool_a", Arguments: `{}`},
			{ID: "call_2", Name: "tool_b", Arguments: `{}`},
		}},
		{Role: RoleTool, Content: "result a", ToolCallID: "call_1"},
		// call_2 result missing!
		{Role: RoleUser, Content: "what happened?"},
	}
	result := SanitizeMessages(msgs)
	// Incomplete group: strip tool_calls from assistant, remove tool result
	for _, m := range result {
		if m.Role == RoleTool {
			t.Error("expected partial tool results to be removed")
		}
		if m.Role == RoleAssistant && len(m.ToolCalls) > 0 {
			t.Errorf("expected tool_calls to be stripped, found %d", len(m.ToolCalls))
		}
	}
	// Assistant with content "I'll try both" should remain (text only)
	found := false
	for _, m := range result {
		if m.Role == RoleAssistant && m.Content == "I'll try both" {
			found = true
		}
	}
	if !found {
		t.Error("expected assistant content to be preserved")
	}
}

func TestSanitizeMessages_MixedValidAndInvalidGroups(t *testing.T) {
	// First group is valid, second is broken
	msgs := []Message{
		{Role: RoleUser, Content: "first request"},
		{Role: RoleAssistant, Content: "", ToolCalls: []ToolCall{
			{ID: "call_1", Name: "tool_a", Arguments: `{}`},
		}},
		{Role: RoleTool, Content: "result 1", ToolCallID: "call_1"},
		{Role: RoleAssistant, Content: "ok, next"},
		{Role: RoleUser, Content: "second request"},
		{Role: RoleAssistant, Content: "", ToolCalls: []ToolCall{
			{ID: "call_2", Name: "tool_b", Arguments: `{}`},
		}},
		// call_2 result is displaced
		{Role: RoleUser, Content: "where's the result?"},
		{Role: RoleTool, Content: "late result", ToolCallID: "call_2"},
	}
	result := SanitizeMessages(msgs)
	// First group (call_1) should be intact; second group (call_2) should be cleaned
	toolCount := 0
	for _, m := range result {
		if m.Role == RoleTool {
			toolCount++
			if m.ToolCallID != "call_1" {
				t.Errorf("expected only call_1 tool result, got %s", m.ToolCallID)
			}
		}
	}
	if toolCount != 1 {
		t.Errorf("expected 1 tool result, got %d", toolCount)
	}
}
