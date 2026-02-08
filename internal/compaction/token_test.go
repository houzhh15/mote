package compaction

import (
	"testing"

	"mote/internal/provider"
)

func TestTokenCounter_EstimateText(t *testing.T) {
	tc := NewTokenCounter()

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{
			name:     "empty text",
			text:     "",
			expected: 0,
		},
		{
			name:     "short English text",
			text:     "hello",
			expected: 2,
		},
		{
			name:     "English sentence",
			text:     "Hello, how are you?",
			expected: 7,
		},
		{
			name:     "Chinese text",
			text:     "你好世界",
			expected: 4,
		},
		{
			name:     "mixed English and Chinese",
			text:     "Hello 你好",
			expected: 4,
		},
		{
			name:     "long text",
			text:     "This is a longer piece of text that should result in more tokens being estimated.",
			expected: 27,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.EstimateText(tt.text)
			if got != tt.expected {
				t.Errorf("EstimateText(%q) = %d, want %d", tt.text, got, tt.expected)
			}
		})
	}
}

func TestTokenCounter_EstimateMessages(t *testing.T) {
	tc := NewTokenCounter()

	tests := []struct {
		name     string
		messages []provider.Message
		expected int
	}{
		{
			name:     "empty messages",
			messages: []provider.Message{},
			expected: 0,
		},
		{
			name: "single simple message",
			messages: []provider.Message{
				{Role: "user", Content: "hello"},
			},
			expected: 6,
		},
		{
			name: "multiple messages",
			messages: []provider.Message{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "Hi there!"},
			},
			expected: 13,
		},
		{
			name: "message with tool call",
			messages: []provider.Message{
				{
					Role:    "assistant",
					Content: "",
					ToolCalls: []provider.ToolCall{
						{
							ID:        "call_123",
							Arguments: `{"path": "/tmp/test.txt"}`,
						},
					},
				},
			},
			expected: 13,
		},
		{
			name: "conversation with mixed content",
			messages: []provider.Message{
				{Role: "system", Content: "You are a helpful assistant."},
				{Role: "user", Content: "What is 2+2?"},
				{Role: "assistant", Content: "2+2 equals 4."},
			},
			expected: 31,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tc.EstimateMessages(tt.messages)
			if got != tt.expected {
				t.Errorf("EstimateMessages() = %d, want %d", got, tt.expected)
			}
		})
	}
}
