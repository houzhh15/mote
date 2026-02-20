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

func TestNeedsCompaction_FewConversationMessages(t *testing.T) {
	// With few messages but low token threshold, compaction SHOULD trigger
	// so that truncate can shrink oversized messages.
	config := DefaultConfig()
	config.MaxContextTokens = 100 // Very low to ensure token threshold is exceeded
	config.TriggerThreshold = 0.8 // Threshold = 80 tokens
	config.KeepRecentCount = 10

	c := NewCompactor(config, nil)

	// Build content large enough to exceed 80 tokens.
	// EstimateText uses len()/3, so ~300 chars ≈ 100 tokens.
	largeContent := ""
	for i := 0; i < 60; i++ {
		largeContent += "word "
	}

	// 3 conversation messages whose tokens exceed the threshold
	messages := []provider.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "fetch this URL"},
		{Role: "assistant", Content: "", ToolCalls: []provider.ToolCall{{ID: "call_1"}}},
		{Role: "tool", Content: largeContent, ToolCallID: "call_1"},
	}

	// Even with few messages, NeedsCompaction should return true when tokens
	// exceed threshold, because truncate will handle the oversized content.
	if !c.NeedsCompaction(messages) {
		t.Error("NeedsCompaction() should return true when tokens exceed threshold, even with few messages")
	}
}

func TestTruncate_OversizedToolResult(t *testing.T) {
	// Simulate: system prompt + user + assistant(tool_calls) + huge tool result
	// The truncate should NOT produce an empty conversation.
	config := DefaultConfig()
	config.MaxContextTokens = 200 // Small limit
	config.KeepRecentCount = 10

	c := NewCompactor(config, nil)

	// Build a huge tool result (~1000 tokens worth of content)
	hugeContent := ""
	for i := 0; i < 500; i++ {
		hugeContent += "word "
	}

	messages := []provider.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "fetch this page"},
		{Role: "assistant", Content: "", ToolCalls: []provider.ToolCall{{ID: "call_1"}}},
		{Role: "tool", Content: hugeContent, ToolCallID: "call_1"},
	}

	result := c.truncate(messages)

	// Must have at least one user or assistant message
	hasConv := false
	for _, m := range result {
		if m.Role == "user" || m.Role == "assistant" {
			hasConv = true
			break
		}
	}
	if !hasConv {
		t.Errorf("truncate() removed all conversation messages, got %d messages with roles:", len(result))
		for i, m := range result {
			t.Logf("  [%d] role=%s contentLen=%d", i, m.Role, len(m.Content))
		}
	}

	// Verify that huge tool result was truncated, not just discarded
	totalTokens := c.counter.EstimateMessages(result)
	if totalTokens > config.MaxContextTokens*2 {
		t.Errorf("truncated result still too large: %d tokens (max %d)", totalTokens, config.MaxContextTokens)
	}
}

func TestTruncate_MultipleOversizedToolResults(t *testing.T) {
	// 10 messages including multiple huge HTTP fetches.
	// This is the real-world scenario: user asks questions, model fetches
	// multiple URLs, and the tool results contain huge web pages.
	config := DefaultConfig()
	config.MaxContextTokens = 500
	config.KeepRecentCount = 10

	c := NewCompactor(config, nil)

	hugeContent := ""
	for i := 0; i < 1000; i++ {
		hugeContent += "word "
	}

	messages := []provider.Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "search for minimax API docs"},
		{Role: "assistant", Content: "Let me search.", ToolCalls: []provider.ToolCall{{ID: "call_1"}}},
		{Role: "tool", Content: hugeContent, ToolCallID: "call_1"},
		{Role: "assistant", Content: "Let me fetch the docs.", ToolCalls: []provider.ToolCall{{ID: "call_2"}}},
		{Role: "tool", Content: hugeContent, ToolCallID: "call_2"},
		{Role: "user", Content: "Can you try the speech API?"},
		{Role: "assistant", Content: "", ToolCalls: []provider.ToolCall{{ID: "call_3"}}},
		{Role: "tool", Content: hugeContent, ToolCallID: "call_3"},
	}

	result := c.truncate(messages)

	// Must contain conversation content
	hasConv := false
	for _, m := range result {
		if m.Role == "user" || m.Role == "assistant" {
			hasConv = true
			break
		}
	}
	if !hasConv {
		t.Fatal("truncate() removed all conversation messages")
	}

	// Result should fit within reasonable bounds
	totalTokens := c.counter.EstimateMessages(result)
	if totalTokens > config.MaxContextTokens*2 {
		t.Errorf("result too large: %d tokens (max %d)", totalTokens, config.MaxContextTokens)
	}

	t.Logf("Truncated from %d to %d messages, %d tokens", len(messages), len(result), totalTokens)
	for i, m := range result {
		t.Logf("  [%d] role=%s contentLen=%d toolCalls=%d", i, m.Role, len(m.Content), len(m.ToolCalls))
	}
}

func TestCompactWithFallback_ReturnsOriginalIfBothFail(t *testing.T) {
	config := DefaultConfig()
	config.MaxContextTokens = 10 // Absurdly low
	config.KeepRecentCount = 100 // Way more than messages

	// Provider that always fails
	mp := &mockProvider{
		chatFunc: func(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
			return nil, errors.New("mock error")
		},
	}
	c := NewCompactor(config, mp)

	messages := []provider.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "hello"},
	}

	result := c.CompactWithFallback(context.Background(), messages)

	// Should return original messages since both paths fail to produce valid results
	// (but the safety net in CompactWithFallback or truncate should still produce
	// something with conversation content)
	hasConv := false
	for _, m := range result {
		if m.Role == "user" || m.Role == "assistant" {
			hasConv = true
			break
		}
	}
	if !hasConv {
		t.Error("CompactWithFallback should never return messages without conversation content")
	}
}
