package minimax

import (
	"encoding/json"
	"strings"
	"testing"

	"mote/internal/provider"
)

func TestBuildRequest_AssistantToolCallsContentNull(t *testing.T) {
	p := &MinimaxProvider{model: "MiniMax-M2.5", maxTokens: 1024}

	req := provider.ChatRequest{
		Model: "MiniMax-M2.5",
		Messages: []provider.Message{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "do something"},
			{
				Role:    "assistant",
				Content: "", // empty content
				ToolCalls: []provider.ToolCall{
					{
						ID:   "call_123",
						Type: "function",
						Function: &struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						}{
							Name:      "test_tool",
							Arguments: `{"arg": "value"}`,
						},
					},
				},
			},
			{Role: "tool", Content: "tool result", ToolCallID: "call_123"},
		},
		Tools: []provider.Tool{
			{Type: "function", Function: provider.ToolFunction{Name: "test_tool", Description: "A test tool"}},
		},
	}

	chatReq := p.buildRequest(req, true)

	// Marshal to JSON and verify
	data, err := json.Marshal(chatReq)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	t.Logf("Request JSON: %s", jsonStr)

	// The assistant message with tool_calls should have "content":null, NOT "content":""
	if strings.Contains(jsonStr, `"content":""`) {
		t.Errorf("Found empty string content in request JSON. MiniMax requires null, not empty string.\nJSON: %s", jsonStr)
	}

	// Verify the assistant message specifically
	for i, msg := range chatReq.Messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			if msg.Content != nil {
				t.Errorf("Message[%d]: assistant with tool_calls should have nil Content, got %q", i, *msg.Content)
			}
		}
	}

	// Verify system and user messages have non-nil content
	for i, msg := range chatReq.Messages {
		if msg.Role == "system" || msg.Role == "user" {
			if msg.Content == nil {
				t.Errorf("Message[%d] role=%s: Content should not be nil", i, msg.Role)
			}
		}
	}
}

func TestBuildRequest_AssistantWithContentKeepsIt(t *testing.T) {
	p := &MinimaxProvider{model: "MiniMax-M2.5", maxTokens: 1024}

	req := provider.ChatRequest{
		Model: "MiniMax-M2.5",
		Messages: []provider.Message{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "Hi there!"},
		},
	}

	chatReq := p.buildRequest(req, false)

	// Verify assistant message with content keeps it
	for _, msg := range chatReq.Messages {
		if msg.Role == "assistant" {
			if msg.Content == nil {
				t.Error("assistant message with content should not have nil Content")
			} else if *msg.Content != "Hi there!" {
				t.Errorf("expected 'Hi there!', got %q", *msg.Content)
			}
		}
	}
}
