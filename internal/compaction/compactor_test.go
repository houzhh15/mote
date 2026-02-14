package compaction

import (
	"context"
	"errors"
	"testing"

	"mote/internal/provider"
)

// mockProvider implements provider.Provider for testing.
type mockProvider struct {
	chatFunc func(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error)
}

func (m *mockProvider) Name() string {
	return "mock"
}

func (m *mockProvider) Models() []string {
	return []string{"mock-model"}
}

func (m *mockProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	if m.chatFunc != nil {
		return m.chatFunc(ctx, req)
	}
	return &provider.ChatResponse{Content: "Summary of the conversation."}, nil
}

func (m *mockProvider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.ChatEvent, error) {
	return nil, errors.New("not implemented")
}

func TestCompactor_NeedsCompaction(t *testing.T) {
	config := DefaultConfig()
	config.MaxContextTokens = 100
	config.TriggerThreshold = 0.8

	c := NewCompactor(config, nil)

	tests := []struct {
		name     string
		messages []provider.Message
		expected bool
	}{
		{
			name:     "empty messages",
			messages: []provider.Message{},
			expected: false,
		},
		{
			name: "below threshold",
			messages: []provider.Message{
				{Role: "user", Content: "hello"},
			},
			expected: false,
		},
		{
			name: "above threshold",
			messages: func() []provider.Message {
				msgs := make([]provider.Message, 0, 50)
				for i := 0; i < 50; i++ {
					msgs = append(msgs, provider.Message{
						Role:    "user",
						Content: "This is a test message with some content.",
					})
				}
				return msgs
			}(),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.NeedsCompaction(tt.messages)
			if got != tt.expected {
				t.Errorf("NeedsCompaction() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestCompactor_Compact(t *testing.T) {
	config := DefaultConfig()
	config.KeepRecentCount = 2

	t.Run("no provider", func(t *testing.T) {
		c := NewCompactor(config, nil)
		messages := []provider.Message{
			{Role: "user", Content: "msg1"},
			{Role: "assistant", Content: "msg2"},
			{Role: "user", Content: "msg3"},
		}
		_, err := c.Compact(context.Background(), messages)
		if !errors.Is(err, ErrNoProvider) {
			t.Errorf("expected ErrNoProvider, got %v", err)
		}
	})

	t.Run("too few messages", func(t *testing.T) {
		mp := &mockProvider{}
		c := NewCompactor(config, mp)
		messages := []provider.Message{
			{Role: "user", Content: "msg1"},
		}
		_, err := c.Compact(context.Background(), messages)
		if !errors.Is(err, ErrMessagesTooShort) {
			t.Errorf("expected ErrMessagesTooShort, got %v", err)
		}
	})

	t.Run("successful compaction", func(t *testing.T) {
		mp := &mockProvider{
			chatFunc: func(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
				return &provider.ChatResponse{Content: "Summary: discussed greetings."}, nil
			},
		}
		c := NewCompactor(config, mp)
		messages := []provider.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
			{Role: "user", Content: "how are you"},
			{Role: "assistant", Content: "I am fine"},
		}
		result, err := c.Compact(context.Background(), messages)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should have: 1 system + 1 summary + 2 kept = 4
		if len(result) != 4 {
			t.Errorf("expected 4 messages, got %d", len(result))
		}

		// First should be system
		if result[0].Role != "system" {
			t.Errorf("first message should be system, got %s", result[0].Role)
		}

		// Second should be summary
		if result[1].Role != "assistant" {
			t.Errorf("second message should be assistant (summary), got %s", result[1].Role)
		}
	})

	t.Run("LLM failure", func(t *testing.T) {
		mp := &mockProvider{
			chatFunc: func(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
				return nil, errors.New("API error")
			},
		}
		c := NewCompactor(config, mp)
		messages := []provider.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
			{Role: "user", Content: "how are you"},
			{Role: "assistant", Content: "I am fine"},
		}
		_, err := c.Compact(context.Background(), messages)
		if !errors.Is(err, ErrSummaryFailed) {
			t.Errorf("expected ErrSummaryFailed, got %v", err)
		}
	})
}

func TestCompactor_CompactWithFallback(t *testing.T) {
	config := DefaultConfig()
	config.MaxContextTokens = 100
	config.KeepRecentCount = 2

	t.Run("fallback to truncation", func(t *testing.T) {
		mp := &mockProvider{
			chatFunc: func(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
				return nil, errors.New("API error")
			},
		}
		c := NewCompactor(config, mp)
		messages := []provider.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "msg1"},
			{Role: "assistant", Content: "msg2"},
			{Role: "user", Content: "msg3"},
			{Role: "assistant", Content: "msg4"},
		}
		result := c.CompactWithFallback(context.Background(), messages)

		// Should have truncated but kept system message
		if len(result) == 0 {
			t.Error("expected non-empty result")
		}
		if result[0].Role != "system" {
			t.Errorf("first message should be system, got %s", result[0].Role)
		}
	})
}

func TestCompactor_separateMessages(t *testing.T) {
	c := NewCompactor(DefaultConfig(), nil)
	messages := []provider.Message{
		{Role: "system", Content: "sys1"},
		{Role: "user", Content: "usr1"},
		{Role: "system", Content: "sys2"},
		{Role: "assistant", Content: "asst1"},
	}

	system, conv := c.separateMessages(messages)

	if len(system) != 2 {
		t.Errorf("expected 2 system messages, got %d", len(system))
	}
	if len(conv) != 2 {
		t.Errorf("expected 2 conversation messages, got %d", len(conv))
	}
}

func TestCompactor_chunkMessages(t *testing.T) {
	config := DefaultConfig()
	config.ChunkMaxTokens = 50
	c := NewCompactor(config, nil)

	messages := []provider.Message{
		{Role: "user", Content: "Short message."},
		{Role: "assistant", Content: "Another short message."},
		{Role: "user", Content: "This is a longer message that might push us over the limit."},
		{Role: "assistant", Content: "Response to the longer message."},
	}

	chunks := c.chunkMessages(messages)

	if len(chunks) == 0 {
		t.Error("expected at least one chunk")
	}
}

func TestAdjustKeepBoundary(t *testing.T) {
	tests := []struct {
		name     string
		msgs     []provider.Message
		splitIdx int
		wantIdx  int
	}{
		{
			name: "no adjustment needed - starts with user",
			msgs: []provider.Message{
				{Role: "user", Content: "old"},
				{Role: "assistant", Content: "old reply"},
				{Role: "user", Content: "new"},
				{Role: "assistant", Content: "new reply"},
			},
			splitIdx: 2,
			wantIdx:  2,
		},
		{
			name: "adjust - starts with tool result",
			msgs: []provider.Message{
				{Role: "user", Content: "do something"},
				{Role: "assistant", Content: "", ToolCalls: []provider.ToolCall{{ID: "call_1", Name: "test"}}},
				{Role: "tool", Content: "result1", ToolCallID: "call_1"},
				{Role: "user", Content: "next question"},
				{Role: "assistant", Content: "answer"},
			},
			splitIdx: 2, // Would start at tool result
			wantIdx:  1, // Should include the assistant tool_call
		},
		{
			name: "adjust - multiple tool results",
			msgs: []provider.Message{
				{Role: "user", Content: "do two things"},
				{Role: "assistant", Content: "", ToolCalls: []provider.ToolCall{{ID: "call_1"}, {ID: "call_2"}}},
				{Role: "tool", Content: "result1", ToolCallID: "call_1"},
				{Role: "tool", Content: "result2", ToolCallID: "call_2"},
				{Role: "user", Content: "thanks"},
			},
			splitIdx: 2, // Would start at first tool result
			wantIdx:  1, // Should include the assistant tool_call
		},
		{
			name: "adjust - split at second tool result",
			msgs: []provider.Message{
				{Role: "user", Content: "do two things"},
				{Role: "assistant", Content: "", ToolCalls: []provider.ToolCall{{ID: "call_1"}, {ID: "call_2"}}},
				{Role: "tool", Content: "result1", ToolCallID: "call_1"},
				{Role: "tool", Content: "result2", ToolCallID: "call_2"},
				{Role: "user", Content: "thanks"},
			},
			splitIdx: 3, // Would start at second tool result
			wantIdx:  1, // Should include the assistant tool_call
		},
		{
			name:     "boundary at 0",
			msgs:     []provider.Message{{Role: "tool", Content: "r"}},
			splitIdx: 0,
			wantIdx:  0,
		},
		{
			name: "boundary at end",
			msgs: []provider.Message{
				{Role: "user", Content: "hi"},
				{Role: "assistant", Content: "hello"},
			},
			splitIdx: 2,
			wantIdx:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustKeepBoundary(tt.msgs, tt.splitIdx)
			if got != tt.wantIdx {
				t.Errorf("adjustKeepBoundary() = %d, want %d", got, tt.wantIdx)
			}
		})
	}
}
