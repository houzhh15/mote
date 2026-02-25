package compaction

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

func TestBudgetMessages_Phase1_TruncateToolResults(t *testing.T) {
	config := DefaultConfig()
	config.MaxRequestBytes = 5000 // Small budget
	c := NewCompactor(config, nil)

	messages := []provider.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "", ToolCalls: []provider.ToolCall{{ID: "t1", Name: "read_file"}}},
		{Role: "tool", Content: strings.Repeat("x", 8000), ToolCallID: "t1"},
		{Role: "user", Content: "what did you find?"},
	}

	// protectedTail=0: these are all historical messages, none are current iteration
	result := c.BudgetMessages(messages, 0, 0)

	// Phase 1 should have truncated the tool result
	for _, m := range result {
		if m.Role == "tool" && len(m.Content) > 4200 {
			t.Errorf("tool result should be truncated, got %d bytes", len(m.Content))
		}
	}
}

func TestBudgetMessages_Phase2_TruncateAssistantContent(t *testing.T) {
	config := DefaultConfig()
	config.MaxRequestBytes = 3000
	c := NewCompactor(config, nil)

	messages := []provider.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
		// Large assistant message — Phase 1 won't touch it (not a tool result)
		{Role: "assistant", Content: strings.Repeat("a", 5000)},
		{Role: "user", Content: "ok"},
		{Role: "assistant", Content: "final"},
	}

	// protectedTail=0: these are all historical messages
	result := c.BudgetMessages(messages, 0, 0)

	// Phase 2 should have truncated the large assistant message
	for _, m := range result {
		if m.Role == "assistant" && len(m.Content) > 2200 {
			t.Errorf("assistant message should be truncated by Phase 2, got %d bytes", len(m.Content))
		}
	}
}

func TestBudgetMessages_Phase3_DropOldestMessages(t *testing.T) {
	config := DefaultConfig()
	config.MaxRequestBytes = 5000
	c := NewCompactor(config, nil)

	// Create many short messages whose combined overhead (80 B each) exceeds budget.
	// 60 messages × 80 B overhead = 4800 B overhead + 20 KB baseline = way over 5 KB budget.
	// Even with MaxRequestBytes=5000 and 0 toolsOverhead, the baseline alone is 20 KB.
	// Let's use a realistic budget.
	config.MaxRequestBytes = 25000
	c = NewCompactor(config, nil)

	messages := []provider.Message{
		{Role: "system", Content: "sys"},
	}
	// Add 50 assistant→tool rounds with small content
	for i := 0; i < 50; i++ {
		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   "",
			ToolCalls: []provider.ToolCall{{ID: fmt.Sprintf("t%d", i), Name: "shell"}},
		})
		messages = append(messages, provider.Message{
			Role:       "tool",
			Content:    fmt.Sprintf("result %d: ok", i),
			ToolCallID: fmt.Sprintf("t%d", i),
		})
	}
	messages = append(messages, provider.Message{Role: "user", Content: "done?"})

	result := c.BudgetMessages(messages, 0)

	// Should have fewer messages (Phase 3 dropped oldest)
	if len(result) >= len(messages) {
		t.Errorf("expected fewer messages after Phase 3, got %d (original %d)", len(result), len(messages))
	}

	// Last message should still be the user "done?"
	lastMsg := result[len(result)-1]
	if lastMsg.Role != "user" || lastMsg.Content != "done?" {
		t.Errorf("last message should be user 'done?', got role=%s content=%q", lastMsg.Role, lastMsg.Content)
	}

	// Should contain the notice about dropped context
	found := false
	for _, m := range result {
		if strings.Contains(m.Content, "dropped to fit request size budget") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a notice about dropped context")
	}

	// Verify no orphaned tool results (every tool msg has a preceding assistant with matching tool_call)
	for i, m := range result {
		if m.Role == "tool" && m.ToolCallID != "" {
			// Find preceding assistant with matching tool call
			foundPair := false
			for j := i - 1; j >= 0; j-- {
				if result[j].Role == "assistant" {
					for _, tc := range result[j].ToolCalls {
						if tc.ID == m.ToolCallID {
							foundPair = true
							break
						}
					}
					break
				}
			}
			if !foundPair {
				t.Errorf("orphaned tool result at index %d, toolCallID=%s", i, m.ToolCallID)
			}
		}
	}
}

func TestBudgetMessages_RealisticGLMScenario(t *testing.T) {
	// Reproduce the actual failure: 107 messages, toolsOverhead=17438,
	// estimated 110222 bytes, budget 65536.
	config := DefaultConfig()
	config.MaxRequestBytes = 65536
	c := NewCompactor(config, nil)

	messages := []provider.Message{
		{Role: "system", Content: strings.Repeat("s", 3000)},
		{Role: "assistant", Content: "[Previous conversation context was truncated...]"},
	}
	// Add tool call rounds with moderate content (simulating post-TruncateOnly state)
	for i := 0; i < 50; i++ {
		messages = append(messages, provider.Message{
			Role:      "assistant",
			Content:   fmt.Sprintf("I'll check file %d", i),
			ToolCalls: []provider.ToolCall{{ID: fmt.Sprintf("call_%d", i), Name: "read_file", Arguments: fmt.Sprintf(`{"path":"/some/path/%d"}`, i)}},
		})
		messages = append(messages, provider.Message{
			Role:       "tool",
			Content:    strings.Repeat("x", 800), // already truncated by TruncateOnly
			ToolCallID: fmt.Sprintf("call_%d", i),
		})
	}
	messages = append(messages, provider.Message{Role: "user", Content: "summarize"})

	toolsOverhead := 17438

	estimated := c.estimateRequestBytes(messages, toolsOverhead)
	t.Logf("Before BudgetMessages: %d messages, estimated %d bytes (budget %d)",
		len(messages), estimated, config.MaxRequestBytes)

	result := c.BudgetMessages(messages, toolsOverhead)

	estimatedAfter := c.estimateRequestBytes(result, toolsOverhead)
	t.Logf("After BudgetMessages: %d messages, estimated %d bytes",
		len(result), estimatedAfter)

	if estimatedAfter > config.MaxRequestBytes {
		t.Errorf("BudgetMessages failed to bring request under budget: %d > %d",
			estimatedAfter, config.MaxRequestBytes)
	}

	// Must still have conversation content
	hasConv := false
	for _, m := range result {
		if m.Role == "user" || (m.Role == "assistant" && len(m.ToolCalls) == 0) {
			hasConv = true
			break
		}
	}
	if !hasConv {
		t.Error("result must contain at least one conversation message")
	}
}

// assertNoConsecutiveRoles checks that no two adjacent non-system messages
// have the same role, which is a requirement for OpenAI-compatible APIs
// (including GLM).  Returns the first violation found, or "".
func assertNoConsecutiveRoles(t *testing.T, messages []provider.Message) {
	t.Helper()
	var prevRole string
	for i, m := range messages {
		if m.Role == provider.RoleSystem {
			continue
		}
		if m.Role == provider.RoleTool {
			// tool results can follow other tool results (multi-tool round)
			prevRole = m.Role
			continue
		}
		if m.Role == prevRole {
			t.Errorf("consecutive %q messages at index %d and previous non-system index (violates role alternation)",
				m.Role, i)
			return
		}
		prevRole = m.Role
	}
}

func TestTruncateOnly_NoConsecutiveAssistant(t *testing.T) {
	// After adjustKeepBoundary, the first kept message can be assistant
	// with tool_calls.  The notice inserted by TruncateOnly must not
	// create assistant → assistant.
	config := DefaultConfig()
	config.MaxContextTokens = 600
	config.KeepRecentCount = 6
	c := NewCompactor(config, nil)

	messages := []provider.Message{
		{Role: "system", Content: "system prompt"},
		// Old messages that will be truncated
		{Role: "user", Content: strings.Repeat("old question ", 100)},
		{Role: "assistant", Content: strings.Repeat("old answer ", 100)},
		{Role: "user", Content: strings.Repeat("another old ", 100)},
		{Role: "assistant", Content: strings.Repeat("another reply ", 100)},
		// Recent: user prompt then assistant triggers tool call
		{Role: "user", Content: "please check the file"},
		{Role: "assistant", Content: "", ToolCalls: []provider.ToolCall{
			{ID: "tc1", Name: "read_file", Arguments: `{"path":"a"}`},
		}},
		{Role: "tool", Content: "file contents", ToolCallID: "tc1"},
		{Role: "assistant", Content: "Here is what I found"},
		{Role: "user", Content: "thanks"},
	}

	result := c.TruncateOnly(messages)
	t.Logf("TruncateOnly result: %d messages", len(result))
	for i, m := range result {
		tc := ""
		if len(m.ToolCalls) > 0 {
			tc = " [has tool_calls]"
		}
		t.Logf("  [%d] %s: %.40s%s", i, m.Role, m.Content, tc)
	}

	assertNoConsecutiveRoles(t, result)
}

func TestCompact_NoConsecutiveAssistant(t *testing.T) {
	// Compact inserts a summary. If keptMsgs starts with assistant (tool_calls),
	// the summary role must adapt.
	config := DefaultConfig()
	config.MaxContextTokens = 600
	config.KeepRecentCount = 4
	config.ChunkMaxTokens = 5000

	mock := &mockProvider{}
	c := NewCompactor(config, mock)

	messages := []provider.Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "old question"},
		{Role: "assistant", Content: "old answer"},
		{Role: "user", Content: "another question"},
		{Role: "assistant", Content: "another answer"},
		{Role: "user", Content: "do X"},
		// Kept window starts here — assistant with tool_calls (adjustKeepBoundary
		// pulls the boundary back to include this when the original boundary
		// would have split the tool pair)
		{Role: "assistant", Content: "", ToolCalls: []provider.ToolCall{
			{ID: "tc1", Name: "read_file", Arguments: `{"path":"a"}`},
		}},
		{Role: "tool", Content: "file contents", ToolCallID: "tc1"},
		{Role: "assistant", Content: "Here is what I found"},
		{Role: "user", Content: "thanks"},
	}

	result, err := c.Compact(context.Background(), messages)
	if err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	t.Logf("Compact result: %d messages", len(result))
	for i, m := range result {
		tc := ""
		if len(m.ToolCalls) > 0 {
			tc = " [has tool_calls]"
		}
		t.Logf("  [%d] %s: %.60s%s", i, m.Role, m.Content, tc)
	}

	assertNoConsecutiveRoles(t, result)
}

func TestDropOldestToBudget_NoConsecutiveAssistant(t *testing.T) {
	// dropOldestToBudget inserts a notice. When keptConv starts with
	// assistant, the notice must use user role.
	config := DefaultConfig()
	config.MaxRequestBytes = 2000
	c := NewCompactor(config, nil)

	messages := []provider.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: strings.Repeat("x", 500)},
		{Role: "assistant", Content: strings.Repeat("y", 500)},
		{Role: "user", Content: strings.Repeat("z", 500)},
		// This will be the first kept after drop — assistant with tool_calls
		{Role: "assistant", Content: "", ToolCalls: []provider.ToolCall{
			{ID: "tc1", Name: "run", Arguments: `{"cmd":"ls"}`},
		}},
		{Role: "tool", Content: "output", ToolCallID: "tc1"},
		{Role: "assistant", Content: "done"},
		{Role: "user", Content: "ok"},
	}

	// Force Phase 3 by making messages exceed budget
	result := c.BudgetMessages(messages, 0)

	t.Logf("BudgetMessages result: %d messages", len(result))
	for i, m := range result {
		tc := ""
		if len(m.ToolCalls) > 0 {
			tc = " [has tool_calls]"
		}
		t.Logf("  [%d] %s: %.60s%s", i, m.Role, m.Content, tc)
	}

	assertNoConsecutiveRoles(t, result)
}

func TestBudgetMessages_RecentWindowTruncation(t *testing.T) {
	// Reproduce the actual failure: 14 messages with 3 large read_file tool
	// results.  With protectedTail=4 (1 assistant + 3 tool results), the
	// current round's tool results must NOT be truncated — only historical
	// messages are compressed/dropped.
	config := DefaultConfig()
	config.MaxRequestBytes = 65536
	c := NewCompactor(config, nil)

	// Large system prompt (~38 KB) — realistic for coding assistants
	messages := []provider.Message{
		{Role: "system", Content: strings.Repeat("system instructions ", 1900)}, // ~38 KB
	}

	// Older conversation (already truncated from prior TruncateOnly)
	for i := 0; i < 4; i++ {
		messages = append(messages, provider.Message{
			Role:    "user",
			Content: fmt.Sprintf("old question %d", i),
		})
		messages = append(messages, provider.Message{
			Role:    "assistant",
			Content: fmt.Sprintf("old answer %d", i),
		})
	}

	// Current user request
	messages = append(messages, provider.Message{
		Role:    "user",
		Content: "请分析这些文件的内存管理",
	})

	// Model called 3x read_file — big tool results (current iteration)
	messages = append(messages, provider.Message{
		Role:    "assistant",
		Content: "I'll read the files",
		ToolCalls: []provider.ToolCall{
			{ID: "call_1", Name: "read_file", Arguments: `{"path":"types.go"}`},
			{ID: "call_2", Name: "read_file", Arguments: `{"path":"types_v2.go"}`},
			{ID: "call_3", Name: "read_file", Arguments: `{"path":"manager.go"}`},
		},
	})
	toolResult1 := strings.Repeat("package memory\n// types.go\n", 500)    // ~14 KB
	toolResult2 := strings.Repeat("package memory\n// types_v2.go\n", 400) // ~12 KB
	toolResult3 := strings.Repeat("package memory\n// manager.go\n", 600)  // ~18 KB
	messages = append(messages, provider.Message{
		Role:       "tool",
		Content:    toolResult1,
		ToolCallID: "call_1",
	})
	messages = append(messages, provider.Message{
		Role:       "tool",
		Content:    toolResult2,
		ToolCallID: "call_2",
	})
	messages = append(messages, provider.Message{
		Role:       "tool",
		Content:    toolResult3,
		ToolCallID: "call_3",
	})

	toolsOverhead := 17438
	// protectedTail = 4: 1 assistant + 3 tool results from current iteration
	protectedTail := 4

	estimated := c.estimateRequestBytes(messages, toolsOverhead)
	t.Logf("Before: %d messages, estimated %d bytes (budget %d)",
		len(messages), estimated, config.MaxRequestBytes)

	result := c.BudgetMessages(messages, toolsOverhead, protectedTail)

	estimatedAfter := c.estimateRequestBytes(result, toolsOverhead)
	t.Logf("After: %d messages, estimated %d bytes", len(result), estimatedAfter)
	for i, m := range result {
		tc := ""
		if len(m.ToolCalls) > 0 {
			tc = fmt.Sprintf(" [%d tool_calls]", len(m.ToolCalls))
		}
		t.Logf("  [%d] %s (len=%d)%s", i, m.Role, len(m.Content), tc)
	}

	// The request may exceed budget because the protected tail alone is large.
	// That's OK — the priority is to preserve the current iteration's fresh
	// tool results so the model can actually process then.

	// Must contain at least one user message — the original user request
	hasUser := false
	for _, m := range result {
		if m.Role == "user" {
			hasUser = true
			break
		}
	}
	if !hasUser {
		t.Error("result must contain at least one user message (GLM requirement)")
	}

	// Must contain tool results (the model needs them to formulate response)
	hasToolResult := false
	for _, m := range result {
		if m.Role == "tool" {
			hasToolResult = true
			break
		}
	}
	if !hasToolResult {
		t.Error("result should preserve tool results from the current round")
	}

	// Must contain the assistant with tool_calls
	hasToolCalls := false
	for _, m := range result {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			hasToolCalls = true
			break
		}
	}
	if !hasToolCalls {
		t.Error("result should preserve assistant tool_calls message")
	}

	// CRITICAL: Protected tail tool results must NOT be truncated.
	// The model just called read_file — it needs the full file content.
	for _, m := range result {
		if m.Role == "tool" {
			if strings.Contains(m.Content, "[... truncated") {
				t.Errorf("protected tail tool result should NOT be truncated (len=%d)", len(m.Content))
			}
		}
	}

	// Old conversation messages should have been dropped to make room
	if len(result) >= len(messages) {
		t.Logf("note: no messages were dropped (system+protected might fit)")
	}

	// Role alternation must be valid
	assertNoConsecutiveRoles(t, result)
}

func TestDropOldestToBudget_SafetyNet(t *testing.T) {
	// When available budget is tiny (huge system + toolsOverhead), even the
	// most recent message doesn't fit.  dropOldestToBudget must still return
	// at least one conversation round.
	config := DefaultConfig()
	config.MaxRequestBytes = 50000
	c := NewCompactor(config, nil)

	messages := []provider.Message{
		{Role: "system", Content: strings.Repeat("s", 28000)}, // 28 KB system prompt
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "", ToolCalls: []provider.ToolCall{
			{ID: "c1", Name: "ls", Arguments: `{}`},
		}},
		{Role: "tool", Content: strings.Repeat("f", 20000), ToolCallID: "c1"}, // 20 KB
		{Role: "assistant", Content: "done"},
		{Role: "user", Content: "thanks"},
	}

	// With toolsOverhead=19000, system baseline ≈ 28080+21000 = 49080
	// Available ≈ 50000 - 49080 = 920 — not enough for most messages.
	result := c.BudgetMessages(messages, 19000)

	t.Logf("Result: %d messages", len(result))
	for i, m := range result {
		tc := ""
		if len(m.ToolCalls) > 0 {
			tc = " [tool_calls]"
		}
		t.Logf("  [%d] %s (len=%d)%s", i, m.Role, len(m.Content), tc)
	}

	// Must have at least one user message
	hasUser := false
	for _, m := range result {
		if m.Role == "user" {
			hasUser = true
			break
		}
	}
	if !hasUser {
		t.Error("safety net must preserve at least one user message")
	}

	assertNoConsecutiveRoles(t, result)
}

// ========================================================================
// Tests for OpenClaw-inspired improvements
// ========================================================================

func TestNeedsCompaction_ReserveTokens(t *testing.T) {
	config := DefaultConfig()
	config.MaxContextTokens = 1000
	config.ReserveTokens = 200 // threshold = 1000-200 = 800 tokens
	config.MaxMessageCount = 0 // disable message count trigger

	c := NewCompactor(config, nil)

	// Build messages that total ~600 tokens (below threshold)
	smallMsgs := make([]provider.Message, 0, 10)
	for i := 0; i < 10; i++ {
		smallMsgs = append(smallMsgs, provider.Message{
			Role:    "user",
			Content: "short msg",
		})
	}
	if c.NeedsCompaction(smallMsgs) {
		t.Error("should NOT need compaction below threshold")
	}

	// Build messages that total >800 tokens (above threshold)
	largeMsgs := make([]provider.Message, 0, 80)
	for i := 0; i < 80; i++ {
		largeMsgs = append(largeMsgs, provider.Message{
			Role:    "user",
			Content: "This is a reasonably long message to accumulate tokens quickly.",
		})
	}
	if !c.NeedsCompaction(largeMsgs) {
		t.Error("should need compaction above ReserveTokens-based threshold")
	}
}

func TestNeedsCompaction_ReserveTokensFallback(t *testing.T) {
	// When ReserveTokens >= MaxContextTokens, fall back to TriggerThreshold
	config := DefaultConfig()
	config.MaxContextTokens = 100
	config.ReserveTokens = 200 // >= MaxContextTokens → fallback
	config.TriggerThreshold = 0.8

	c := NewCompactor(config, nil)

	smallMsgs := []provider.Message{{Role: "user", Content: "hi"}}
	if c.NeedsCompaction(smallMsgs) {
		t.Error("should fall back to TriggerThreshold and not trigger for small message")
	}
}

func TestNeedsMemoryFlush(t *testing.T) {
	config := DefaultConfig()
	config.MaxContextTokens = 1000
	config.ReserveTokens = 200 // compaction threshold = 800
	config.MemoryFlush.Enabled = true
	config.MemoryFlush.SoftThresholdTokens = 100 // memory flush threshold = 700
	config.MaxMessageCount = 0

	c := NewCompactor(config, nil)

	// Build messages totaling ~720 tokens (above memory flush 700, below compaction 800)
	// Each message: "This message has moderate content." = 36 chars → (36+2)/3 = 12 content tokens + 4 overhead = 16 tokens
	// 45 × 16 = 720 tokens
	msgs := make([]provider.Message, 0, 45)
	for i := 0; i < 45; i++ {
		msgs = append(msgs, provider.Message{
			Role:    "user",
			Content: "This message has moderate content.",
		})
	}

	if !c.NeedsMemoryFlush(msgs) {
		t.Error("should trigger memory flush when above flush threshold")
	}
	// But should NOT trigger compaction
	if c.NeedsCompaction(msgs) {
		t.Error("should NOT trigger compaction yet (below compaction threshold)")
	}
}

func TestNeedsMemoryFlush_Disabled(t *testing.T) {
	config := DefaultConfig()
	config.MemoryFlush.Enabled = false
	c := NewCompactor(config, nil)

	msgs := make([]provider.Message, 100)
	for i := range msgs {
		msgs[i] = provider.Message{Role: "user", Content: strings.Repeat("x", 1000)}
	}
	if c.NeedsMemoryFlush(msgs) {
		t.Error("should not trigger memory flush when disabled")
	}
}

func TestFindCutPoint_TokenBased(t *testing.T) {
	config := DefaultConfig()
	config.MaxContextTokens = 10000
	config.ReserveTokens = 200 // keep ~200 tokens worth of recent messages
	config.KeepRecentCount = 2 // minimum floor

	c := NewCompactor(config, nil)

	// Create messages: 8 small + 1 big (at end)
	msgs := []provider.Message{
		{Role: "user", Content: "msg1"},
		{Role: "assistant", Content: "msg2"},
		{Role: "user", Content: "msg3"},
		{Role: "assistant", Content: "msg4"},
		{Role: "user", Content: "msg5"},
		{Role: "assistant", Content: "msg6"},
		{Role: "user", Content: "msg7"},
		{Role: "assistant", Content: strings.Repeat("x", 300)}, // big message
	}

	splitIdx := c.findCutPoint(msgs)

	// The big message alone exceeds 200 tokens, so only 1-2 messages should be kept.
	// KeepRecentCount=2 is the minimum floor.
	kept := len(msgs) - splitIdx
	if kept < 2 {
		t.Errorf("should keep at least KeepRecentCount(2), got %d", kept)
	}
	if splitIdx == 0 {
		t.Error("should compact some messages, not keep all")
	}
}

func TestFindCutPoint_LegacyFallback(t *testing.T) {
	config := DefaultConfig()
	config.ReserveTokens = 0 // force legacy mode
	config.KeepRecentCount = 3

	c := NewCompactor(config, nil)

	msgs := []provider.Message{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "c"},
		{Role: "assistant", Content: "d"},
		{Role: "user", Content: "e"},
	}

	splitIdx := c.findCutPoint(msgs)
	kept := len(msgs) - splitIdx
	if kept != 3 {
		t.Errorf("legacy mode should keep exactly KeepRecentCount(3), got %d", kept)
	}
}

func TestDetectPreviousSummary(t *testing.T) {
	c := NewCompactor(DefaultConfig(), nil)

	t.Run("found", func(t *testing.T) {
		msgs := []provider.Message{
			{Role: "user", Content: "[Previous conversation summary]\n## Goal\nFix bugs"},
			{Role: "assistant", Content: "ok"},
			{Role: "user", Content: "next task"},
		}
		got := c.detectPreviousSummary(msgs)
		if got != "## Goal\nFix bugs" {
			t.Errorf("expected '## Goal\\nFix bugs', got %q", got)
		}
	})

	t.Run("not found", func(t *testing.T) {
		msgs := []provider.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "world"},
		}
		got := c.detectPreviousSummary(msgs)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}

func TestReplaceOversizedMessages(t *testing.T) {
	config := DefaultConfig()
	config.MaxContextTokens = 100
	config.MaxSingleMsgRatio = 0.5 // threshold = 50 tokens

	c := NewCompactor(config, nil)

	msgs := []provider.Message{
		{Role: "user", Content: "small"},
		{Role: "tool", Content: strings.Repeat("x", 500), ToolCallID: "tc1"}, // oversized
		{Role: "assistant", Content: "reply"},
		{Role: "system", Content: strings.Repeat("y", 500)}, // system → skip
	}

	result := c.replaceOversizedMessages(msgs)

	// The tool message should be replaced with a notice.
	if !strings.Contains(result[1].Content, "Oversized") {
		t.Errorf("expected oversized notice, got %q", result[1].Content)
	}
	// ToolCallID preserved.
	if result[1].ToolCallID != "tc1" {
		t.Error("ToolCallID should be preserved")
	}
	// System message should NOT be replaced.
	if strings.Contains(result[3].Content, "Oversized") {
		t.Error("system messages should never be replaced")
	}
	// Small user message unchanged.
	if result[0].Content != "small" {
		t.Error("small messages should be unchanged")
	}
}

func TestReplaceOversizedMessages_Disabled(t *testing.T) {
	config := DefaultConfig()
	config.MaxSingleMsgRatio = 0 // disabled
	c := NewCompactor(config, nil)

	msgs := []provider.Message{
		{Role: "tool", Content: strings.Repeat("x", 100000)},
	}
	result := c.replaceOversizedMessages(msgs)
	if result[0].Content != msgs[0].Content {
		t.Error("should be unchanged when disabled")
	}
}

func TestExtractToolFailures(t *testing.T) {
	c := NewCompactor(DefaultConfig(), nil)

	msgs := []provider.Message{
		{Role: "tool", Content: "Success: file written"},
		{Role: "tool", Content: "Error: permission denied for /etc/hosts"},
		{Role: "tool", Content: "file content..."},
		{Role: "tool", Content: "Failed to connect to database"},
		{Role: "tool", Content: "ok"},
		{Role: "tool", Content: "Error: not found"},
	}

	failures := c.extractToolFailures(msgs, 8)

	if len(failures) != 3 {
		t.Fatalf("expected 3 failures, got %d: %v", len(failures), failures)
	}
	// Should be chronological order (oldest first).
	if !strings.Contains(failures[0], "permission denied") {
		t.Errorf("first failure should be 'permission denied', got %q", failures[0])
	}
}

func TestExtractToolFailures_MaxCount(t *testing.T) {
	c := NewCompactor(DefaultConfig(), nil)

	msgs := make([]provider.Message, 20)
	for i := range msgs {
		msgs[i] = provider.Message{Role: "tool", Content: fmt.Sprintf("Error: failure %d", i)}
	}

	failures := c.extractToolFailures(msgs, 5)
	if len(failures) != 5 {
		t.Fatalf("expected at most 5 failures, got %d", len(failures))
	}
}

func TestExtractFileInfo(t *testing.T) {
	c := NewCompactor(DefaultConfig(), nil)

	msgs := []provider.Message{
		{
			Role: "assistant",
			ToolCalls: []provider.ToolCall{
				{Function: &struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: "read_file", Arguments: `{"path": "/src/main.go"}`}},
				{Function: &struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: "edit_file", Arguments: `{"path": "/src/main.go"}`}},
				{Function: &struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: "read_file", Arguments: `{"path": "/src/util.go"}`}},
			},
		},
		{
			Role: "assistant",
			ToolCalls: []provider.ToolCall{
				{Function: &struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: "create_file", Arguments: `{"file_path": "/src/new.go"}`}},
			},
		},
	}

	readFiles, modifiedFiles := c.extractFileInfo(msgs)

	if len(readFiles) != 2 {
		t.Errorf("expected 2 read files, got %d: %v", len(readFiles), readFiles)
	}
	if len(modifiedFiles) != 2 {
		t.Errorf("expected 2 modified files, got %d: %v", len(modifiedFiles), modifiedFiles)
	}
}

func TestBuildMetadataBlock(t *testing.T) {
	c := NewCompactor(DefaultConfig(), nil)

	t.Run("all fields", func(t *testing.T) {
		result := c.buildMetadataBlock(
			[]string{"Error: crash"},
			[]string{"/a.go"},
			[]string{"/b.go"},
		)
		if !strings.Contains(result, "tool failures") {
			t.Error("should contain tool failures section")
		}
		if !strings.Contains(result, "<read-files>") {
			t.Error("should contain read-files tag")
		}
		if !strings.Contains(result, "<modified-files>") {
			t.Error("should contain modified-files tag")
		}
	})

	t.Run("empty", func(t *testing.T) {
		result := c.buildMetadataBlock(nil, nil, nil)
		if result != "" {
			t.Errorf("expected empty string for no metadata, got %q", result)
		}
	})
}

func TestCompact_StructuredSummary(t *testing.T) {
	mp := &mockProvider{
		chatFunc: func(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
			// Verify structured prompt is used.
			prompt := req.Messages[0].Content
			if !strings.Contains(prompt, "## Goal") {
				t.Error("summary prompt should contain structured sections")
			}
			if !strings.Contains(prompt, "## Progress") {
				t.Error("summary prompt should contain Progress section")
			}
			return &provider.ChatResponse{Content: "## Goal\nTest goal\n## Progress\nDone"}, nil
		},
	}

	config := DefaultConfig()
	config.KeepRecentCount = 2
	c := NewCompactor(config, mp)

	messages := []provider.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "user", Content: "task"},
		{Role: "assistant", Content: "done"},
	}

	result, err := c.Compact(context.Background(), messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have: system + summary + 2 kept
	if len(result) != 4 {
		t.Errorf("expected 4 messages, got %d", len(result))
	}
	if !strings.Contains(result[1].Content, "## Goal") {
		t.Error("summary should contain structured content")
	}
}

func TestCompact_IncrementalSummary(t *testing.T) {
	mp := &mockProvider{
		chatFunc: func(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
			prompt := req.Messages[0].Content
			// When previous summary exists, incremental prompt should be used.
			if !strings.Contains(prompt, "Previous summary:") {
				t.Error("should use incremental prompt when previous summary exists")
			}
			if !strings.Contains(prompt, "old goal content") {
				t.Error("previous summary content should be in the prompt")
			}
			return &provider.ChatResponse{Content: "## Goal\nUpdated goal"}, nil
		},
	}

	config := DefaultConfig()
	config.KeepRecentCount = 2
	c := NewCompactor(config, mp)

	messages := []provider.Message{
		{Role: "system", Content: "sys"},
		// This is a previous compaction summary (will be detected).
		{Role: "user", Content: "[Previous conversation summary]\nold goal content"},
		{Role: "assistant", Content: "continuing"},
		{Role: "user", Content: "more work"},
		{Role: "assistant", Content: "done"},
	}

	result, err := c.Compact(context.Background(), messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) < 3 {
		t.Errorf("expected at least 3 messages, got %d", len(result))
	}
}

func TestCompact_ToolFailuresInPrompt(t *testing.T) {
	mp := &mockProvider{
		chatFunc: func(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
			prompt := req.Messages[0].Content
			if !strings.Contains(prompt, "tool failures") {
				t.Error("prompt should contain tool failure metadata")
			}
			if !strings.Contains(prompt, "Error: crash") {
				t.Error("prompt should contain the actual failure message")
			}
			return &provider.ChatResponse{Content: "summary"}, nil
		},
	}

	config := DefaultConfig()
	config.KeepRecentCount = 2
	c := NewCompactor(config, mp)

	messages := []provider.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "do something"},
		{Role: "assistant", Content: "calling tool",
			ToolCalls: []provider.ToolCall{{ID: "tc1", Function: &struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}{Name: "run", Arguments: `{}`}}}},
		{Role: "tool", Content: "Error: crash", ToolCallID: "tc1"},
		{Role: "user", Content: "try again"},
		{Role: "assistant", Content: "ok"},
	}

	_, err := c.Compact(context.Background(), messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompact_FileInfoInPrompt(t *testing.T) {
	mp := &mockProvider{
		chatFunc: func(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
			prompt := req.Messages[0].Content
			if !strings.Contains(prompt, "<read-files>") {
				t.Error("prompt should contain read-files tag")
			}
			if !strings.Contains(prompt, "main.go") {
				t.Error("prompt should contain the read file path")
			}
			return &provider.ChatResponse{Content: "summary"}, nil
		},
	}

	config := DefaultConfig()
	config.KeepRecentCount = 2
	c := NewCompactor(config, mp)

	messages := []provider.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "read the file"},
		{Role: "assistant", Content: "",
			ToolCalls: []provider.ToolCall{{
				ID: "tc1",
				Function: &struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{Name: "read_file", Arguments: `{"path": "main.go"}`},
			}}},
		{Role: "tool", Content: "file content here", ToolCallID: "tc1"},
		{Role: "user", Content: "thanks"},
		{Role: "assistant", Content: "you're welcome"},
	}

	_, err := c.Compact(context.Background(), messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAdaptiveChunking(t *testing.T) {
	config := DefaultConfig()
	config.MaxContextTokens = 1000
	config.AdaptiveChunkMinRatio = 0.15 // 150 tokens min
	config.AdaptiveChunkMaxRatio = 0.40 // 400 tokens max
	config.ChunkMaxTokens = 500         // fixed fallback

	c := NewCompactor(config, nil)

	// Create messages totaling ~600 tokens (12 small messages ~50 tokens each)
	msgs := make([]provider.Message, 12)
	for i := range msgs {
		msgs[i] = provider.Message{
			Role:    "user",
			Content: strings.Repeat("word ", 10), // ~50 tokens
		}
	}

	chunks := c.chunkMessages(msgs)

	// With adaptive: total 600 / 4 = 150, clamped to [150, 400] → ~150 per chunk
	// Should produce about 4 chunks.
	if len(chunks) < 2 {
		t.Errorf("adaptive chunking should produce multiple chunks, got %d", len(chunks))
	}
	if len(chunks) > 12 {
		t.Errorf("too many chunks: %d", len(chunks))
	}
}

func TestAdaptiveChunking_Disabled(t *testing.T) {
	config := DefaultConfig()
	config.AdaptiveChunkMinRatio = 0 // disabled
	config.ChunkMaxTokens = 9999999  // effectively no split

	c := NewCompactor(config, nil)

	msgs := make([]provider.Message, 5)
	for i := range msgs {
		msgs[i] = provider.Message{Role: "user", Content: "hello"}
	}

	chunks := c.chunkMessages(msgs)
	if len(chunks) != 1 {
		t.Errorf("with huge ChunkMaxTokens, should produce 1 chunk, got %d", len(chunks))
	}
}

func TestWithContextWindow(t *testing.T) {
	config := DefaultConfig()
	original := NewCompactor(config, nil)

	t.Run("adapts config for large context window", func(t *testing.T) {
		adapted := original.WithContextWindow(200000)
		if adapted.config.MaxContextTokens != 200000 {
			t.Errorf("MaxContextTokens = %d, want 200000", adapted.config.MaxContextTokens)
		}
		// ReserveTokens should scale proportionally: 10000/48000 * 200000 ≈ 41666
		ratio := float64(10000) / float64(48000)
		expectedReserve := int(float64(200000) * ratio)
		if adapted.config.ReserveTokens != expectedReserve {
			t.Errorf("ReserveTokens = %d, want %d", adapted.config.ReserveTokens, expectedReserve)
		}
		// ChunkMaxTokens should be max(original, contextWindow/3)
		expectedChunk := 200000 / 3
		if adapted.config.ChunkMaxTokens != expectedChunk {
			t.Errorf("ChunkMaxTokens = %d, want %d", adapted.config.ChunkMaxTokens, expectedChunk)
		}
		// MaxRequestBytes should scale proportionally: 65536 * (200000/48000) ≈ 273066
		scale := float64(200000) / float64(48000)
		expectedMaxReqBytes := int(float64(65536) * scale)
		if adapted.config.MaxRequestBytes != expectedMaxReqBytes {
			t.Errorf("MaxRequestBytes = %d, want %d", adapted.config.MaxRequestBytes, expectedMaxReqBytes)
		}
	})

	t.Run("does not modify original compactor", func(t *testing.T) {
		_ = original.WithContextWindow(200000)
		if original.config.MaxContextTokens != 48000 {
			t.Errorf("original MaxContextTokens changed to %d, want 48000", original.config.MaxContextTokens)
		}
	})

	t.Run("no-op for smaller window", func(t *testing.T) {
		adapted := original.WithContextWindow(30000)
		if adapted.config.MaxContextTokens != 48000 {
			t.Errorf("MaxContextTokens = %d, want 48000 (should not shrink)", adapted.config.MaxContextTokens)
		}
	})

	t.Run("no-op for zero window", func(t *testing.T) {
		adapted := original.WithContextWindow(0)
		if adapted.config.MaxContextTokens != 48000 {
			t.Errorf("MaxContextTokens = %d, want 48000", adapted.config.MaxContextTokens)
		}
	})

	t.Run("adapted compactor NeedsCompaction uses new threshold", func(t *testing.T) {
		adapted := original.WithContextWindow(200000)
		// Threshold = MaxContextTokens - ReserveTokens = 200000 - 41666 = 158334
		// Create messages at ~40k tokens (above original threshold 38000, below adapted 158334)
		// Use exactly MaxMessageCount messages to avoid message-count trigger
		msgs := make([]provider.Message, 40) // MaxMessageCount = 40
		for i := range msgs {
			// ~333 tokens each × 40 = ~13320 + role overhead (4*40=160) ≈ 13480 tokens
			// Actually we need more: lets use ~1000 tokens each → 40000 total
			msgs[i] = provider.Message{Role: "user", Content: strings.Repeat("x", 3000)} // ~1000 tokens each
		}
		// 40 * 1000 + 40*4 = ~40160 tokens — above original 38000 but below adapted 158334
		if adapted.NeedsCompaction(msgs) {
			t.Error("adapted compactor should NOT need compaction at ~40k tokens with 200k window")
		}
		if !original.NeedsCompaction(msgs) {
			t.Error("original compactor SHOULD need compaction at ~40k tokens with 48k window")
		}
	})
}

// TestBudgetMessages_ProtectsUserAnswer verifies that BudgetMessages does NOT
// drop or truncate the user's latest message and the LLM's preceding question
// when the tools+system overhead consumes most of the budget.
//
// Reproduces: user answers a question from the LLM, but BudgetMessages with
// protectedTail=0 drops the LLM's question, causing the LLM to re-ask.
func TestBudgetMessages_ProtectsUserAnswer(t *testing.T) {
	config := DefaultConfig()
	config.MaxRequestBytes = 65536 // 64 KB budget

	c := NewCompactor(config, nil)

	// Simulate: [system(16KB)] + [summary] + [tool exchange] + [LLM question] + [user answer]
	llmQuestion := "Based on the analysis above, I need to know: what programming language are you using? Please provide the exact version number as well."
	userAnswer := "Python 3.12"

	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: strings.Repeat("x", 16000)}, // large system prompt
		{Role: provider.RoleAssistant, Content: "[Previous conversation summary]\n" + strings.Repeat("y", 2000)},
		{Role: provider.RoleUser, Content: "please help me set up the project"},
		{Role: provider.RoleAssistant, Content: strings.Repeat("z", 3000) + "\n\n" + llmQuestion},
		{Role: provider.RoleUser, Content: userAnswer},
	}

	// Use large tools overhead to simulate a full tool registry (~47 KB)
	toolsOverhead := 47000

	// With protectedTail=2, the user's answer + LLM's question must be preserved
	result := c.BudgetMessages(messages, toolsOverhead, 2)

	// Verify the user's answer is present and intact
	found := false
	for _, msg := range result {
		if msg.Role == provider.RoleUser && strings.Contains(msg.Content, userAnswer) {
			found = true
		}
	}
	if !found {
		t.Error("user's answer was dropped or truncated — this causes the LLM to re-ask the same question")
		for i, msg := range result {
			t.Logf("  [%d] %s (len=%d): %s", i, msg.Role, len(msg.Content), msg.Content[:min(80, len(msg.Content))])
		}
	}

	// Verify the LLM's question tail (the important part) is preserved
	questionFound := false
	for _, msg := range result {
		if msg.Role == provider.RoleAssistant && strings.Contains(msg.Content, "what programming language") {
			questionFound = true
		}
	}
	if !questionFound {
		t.Error("LLM's question was dropped or its tail was truncated — this causes the LLM to re-ask")
		for i, msg := range result {
			t.Logf("  [%d] %s (len=%d): %s", i, msg.Role, len(msg.Content), msg.Content[:min(80, len(msg.Content))])
		}
	}
}

// TestBudgetMessages_ZeroProtectedTail_StillKeepsMinimum verifies the
// minimum conversation space floor (8 KB) works when protectedTail=0.
func TestBudgetMessages_ZeroProtectedTail_StillKeepsMinimum(t *testing.T) {
	config := DefaultConfig()
	config.MaxRequestBytes = 65536

	c := NewCompactor(config, nil)

	userMsg := "my API key is sk-abc123"
	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: strings.Repeat("x", 16000)},
		{Role: provider.RoleAssistant, Content: strings.Repeat("y", 5000)},
		{Role: provider.RoleUser, Content: "setup request"},
		{Role: provider.RoleAssistant, Content: "I need your API key to proceed. Please provide it."},
		{Role: provider.RoleUser, Content: userMsg},
	}

	// Even with protectedTail=0, the minimum 8 KB floor should keep the recent messages
	result := c.BudgetMessages(messages, 47000, 0)

	found := false
	for _, msg := range result {
		if msg.Role == provider.RoleUser && strings.Contains(msg.Content, userMsg) {
			found = true
		}
	}
	if !found {
		t.Error("user's message was dropped even with 8 KB minimum floor")
		for i, msg := range result {
			t.Logf("  [%d] %s (len=%d)", i, msg.Role, len(msg.Content))
		}
	}
}

// TestAdaptForModel_MaxRequestBytes_MiniMax verifies that for a MiniMax-M2.5
// context window (204800 tokens), MaxRequestBytes scales to ~280 KB instead
// of the default 65 KB.  The 65 KB default caused catastrophic context loss
// when a single tool result (64 KB) + tools JSON (19 KB) + system prompt
// (16 KB) exceeded the budget, causing BudgetMessages to drop ALL
// historical context including compaction summaries.
func TestAdaptForModel_MaxRequestBytes_MiniMax(t *testing.T) {
	config := DefaultConfig()
	config.AdaptForModel(204800) // MiniMax M2.5

	scale := float64(204800) / float64(48000)
	expectedMaxReqBytes := int(float64(65536) * scale)

	if config.MaxRequestBytes != expectedMaxReqBytes {
		t.Errorf("MaxRequestBytes = %d, want %d (scale=%.2f)",
			config.MaxRequestBytes, expectedMaxReqBytes, scale)
	}

	// The scaled budget should comfortably fit:
	// tools JSON (19 KB) + system prompt (16 KB) + tool result (64 KB) + conversation
	minRequired := 19000 + 16000 + 65536 + 20000 // ~120 KB minimum
	if config.MaxRequestBytes < minRequired {
		t.Errorf("MaxRequestBytes %d is too small, need at least %d for typical MiniMax usage",
			config.MaxRequestBytes, minRequired)
	}
	t.Logf("MiniMax M2.5: MaxRequestBytes scaled from 65536 → %d (%.1f KB)",
		config.MaxRequestBytes, float64(config.MaxRequestBytes)/1024)
}

// TestAdaptForModel_MaxMessageCount_MiniMax verifies that MaxMessageCount
// scales super-linearly with the context window.  The default 40 messages
// for a 48000-token model is about 20 tool-call iterations.  For MiniMax
// M2.5 (204800 tokens), scaling to ~427 prevents premature compaction based
// solely on message count when the context window is only ~66% utilized
// (e.g. 316 messages at 107K/162K tokens).
func TestAdaptForModel_MaxMessageCount_MiniMax(t *testing.T) {
	config := DefaultConfig()
	if config.MaxMessageCount != 40 {
		t.Fatalf("unexpected default MaxMessageCount: %d", config.MaxMessageCount)
	}

	config.AdaptForModel(204800) // MiniMax M2.5

	scale := float64(204800) / float64(48000)
	expected := int(float64(40) * scale * 2.5) // ≈ 427

	if config.MaxMessageCount != expected {
		t.Errorf("MaxMessageCount = %d, want %d (scale=%.2f)",
			config.MaxMessageCount, expected, scale)
	}

	// Real-world scenario: 316 messages at 107K tokens (66% of threshold)
	// should NOT trigger message-count compaction.
	if config.MaxMessageCount <= 316 {
		t.Errorf("MaxMessageCount %d should be > 316 for MiniMax M2.5 (was triggering premature compaction)",
			config.MaxMessageCount)
	}
	t.Logf("MiniMax M2.5: MaxMessageCount scaled from 40 → %d (scale=%.2f)",
		config.MaxMessageCount, scale)
}

// TestBudgetMessages_Phase4_TruncatesProtectedTail verifies that Phase 4
// truncates oversized protected-tail tool results when no other option remains.
// This prevents an oversized request from being sent while preserving as
// much useful content as possible (head+tail of the tool result).
func TestBudgetMessages_Phase4_TruncatesProtectedTail(t *testing.T) {
	config := DefaultConfig()
	// Use a small budget to trigger Phase 4 easily
	config.MaxRequestBytes = 10000

	c := NewCompactor(config, nil)

	// Create messages where the protected tail tool result alone exceeds budget
	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: "You are a helpful assistant."},
		{Role: provider.RoleAssistant, Content: "summary of previous work"},
		// Protected tail: tool call + tool result
		{Role: provider.RoleAssistant, Content: "", ToolCalls: []provider.ToolCall{{ID: "tc1", Function: &struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: "read_file", Arguments: `{"path":"big.md"}`}}}},
		{Role: provider.RoleTool, Content: strings.Repeat("X", 50000), ToolCallID: "tc1"}, // 50KB tool result
	}

	// protectedTail=2 means the last 2 messages (assistant tool_call + tool result) are protected
	result := c.BudgetMessages(messages, 0, 2)

	// Find the tool result in the output
	var toolMsg *provider.Message
	for i := range result {
		if result[i].Role == provider.RoleTool {
			toolMsg = &result[i]
			break
		}
	}

	if toolMsg == nil {
		t.Fatal("tool result message was completely removed — Phase 4 should truncate, not remove")
	}

	// The tool result should be truncated (much smaller than original 50000 bytes)
	if len(toolMsg.Content) >= 50000 {
		t.Errorf("tool result was not truncated: len=%d, expected much less than 50000", len(toolMsg.Content))
	}

	// Should contain the truncation marker
	if !strings.Contains(toolMsg.Content, "truncated to fit context budget") {
		t.Error("truncated tool result should contain truncation notice")
	}

	// Should preserve both head and tail (head+tail truncation)
	if !strings.HasPrefix(toolMsg.Content, "XXX") {
		t.Error("truncated tool result should preserve the head")
	}
	if !strings.HasSuffix(toolMsg.Content, "XXX") {
		t.Error("truncated tool result should preserve the tail")
	}

	t.Logf("Phase 4 truncated tool result from 50000 to %d bytes", len(toolMsg.Content))
}

// TestBudgetMessages_LargeContextWindow_NoContextLoss verifies that with a
// properly adapted context window (e.g. MiniMax 204800 tokens), a typical
// conversation including a large tool result does NOT trigger catastrophic
// context loss.  This is the end-to-end regression test for the MaxRequestBytes
// scaling fix.
func TestBudgetMessages_LargeContextWindow_NoContextLoss(t *testing.T) {
	config := DefaultConfig()
	config.AdaptForModel(204800) // MiniMax M2.5 → MaxRequestBytes ≈ 280 KB

	c := NewCompactor(config, nil)

	// Simulate a realistic MiniMax session:
	// - System prompt: 16 KB
	// - Compaction summary: 3 KB
	// - A few conversation rounds: ~5 KB
	// - Tool call + large tool result: ~64 KB
	// Total messages content: ~88 KB, well within the ~280 KB budget
	compactionSummary := "[Compaction summary] The user asked to analyze a document about TLS inspection. " + strings.Repeat("Previous context details. ", 100)

	messages := []provider.Message{
		{Role: provider.RoleSystem, Content: strings.Repeat("system prompt content ", 800)}, // ~16 KB
		{Role: provider.RoleAssistant, Content: compactionSummary},                          // ~3 KB
		{Role: provider.RoleUser, Content: "please read the full document and analyze it"},
		{Role: provider.RoleAssistant, Content: "I'll read the document for you." + strings.Repeat(" analysis ", 200)},
		{Role: provider.RoleUser, Content: "continue"},
		{Role: provider.RoleAssistant, Content: "Let me read the file.", ToolCalls: []provider.ToolCall{{ID: "tc1", Function: &struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{Name: "read_file", Arguments: `{"path":"doc.md"}`}}}},
		{Role: provider.RoleTool, Content: strings.Repeat("document content ", 4000), ToolCallID: "tc1"}, // ~64 KB tool result
	}

	toolsOverhead := 19444 // realistic tools JSON size
	result := c.BudgetMessages(messages, toolsOverhead, 2)

	// The compaction summary MUST be preserved — losing it means the LLM
	// forgets the entire task
	summaryFound := false
	for _, msg := range result {
		if strings.Contains(msg.Content, "Compaction summary") {
			summaryFound = true
			break
		}
	}
	if !summaryFound {
		t.Error("CRITICAL: compaction summary was dropped — LLM will forget the task entirely")
		t.Logf("MaxRequestBytes = %d", config.MaxRequestBytes)
		totalBytes := 0
		for i, msg := range result {
			totalBytes += len(msg.Content)
			t.Logf("  [%d] %s (len=%d): %.80s", i, msg.Role, len(msg.Content), msg.Content)
		}
		t.Logf("  total content bytes: %d", totalBytes)
	}

	// All original messages should be kept (budget is large enough)
	if len(result) < len(messages) {
		t.Errorf("messages were dropped: got %d, want %d — budget should be large enough",
			len(result), len(messages))
	}
}
