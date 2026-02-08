package runner

import (
	"testing"

	"mote/internal/provider"
)

func TestHistoryManager(t *testing.T) {
	t.Run("NewHistoryManager defaults", func(t *testing.T) {
		hm := NewHistoryManager(0, 0)
		if hm.maxMessages != 100 {
			t.Errorf("expected default maxMessages 100, got %d", hm.maxMessages)
		}
		if hm.maxTokens != 100000 {
			t.Errorf("expected default maxTokens 100000, got %d", hm.maxTokens)
		}
	})

	t.Run("NewHistoryManager with values", func(t *testing.T) {
		hm := NewHistoryManager(50, 50000)
		if hm.maxMessages != 50 {
			t.Errorf("expected maxMessages 50, got %d", hm.maxMessages)
		}
		if hm.maxTokens != 50000 {
			t.Errorf("expected maxTokens 50000, got %d", hm.maxTokens)
		}
	})

	t.Run("EstimateTokens", func(t *testing.T) {
		hm := NewHistoryManager(100, 100000)

		tests := []struct {
			content  string
			expected int
		}{
			{"", 0},
			{"hello", 2},       // 5 chars -> (5+2)/3 = 2
			{"hello world", 4}, // 11 chars -> (11+2)/3 = 4
		}

		for _, tt := range tests {
			got := hm.EstimateTokens(tt.content)
			if got != tt.expected {
				t.Errorf("EstimateTokens(%q) = %d, want %d", tt.content, got, tt.expected)
			}
		}
	})

	t.Run("EstimateMessagesTokens", func(t *testing.T) {
		hm := NewHistoryManager(100, 100000)
		messages := []provider.Message{
			{Role: provider.RoleUser, Content: "hello"},
			{Role: provider.RoleAssistant, Content: "world"},
		}

		tokens := hm.EstimateMessagesTokens(messages)
		// 2 messages * 4 overhead + content tokens
		if tokens < 10 {
			t.Errorf("expected at least 10 tokens, got %d", tokens)
		}
	})

	t.Run("Compress not needed", func(t *testing.T) {
		hm := NewHistoryManager(100, 100000)
		messages := []provider.Message{
			{Role: provider.RoleSystem, Content: "You are helpful."},
			{Role: provider.RoleUser, Content: "Hello"},
			{Role: provider.RoleAssistant, Content: "Hi there!"},
		}

		compressed, changed := hm.Compress(messages)
		if changed {
			t.Error("expected no compression needed")
		}
		if len(compressed) != len(messages) {
			t.Errorf("expected %d messages, got %d", len(messages), len(compressed))
		}
	})

	t.Run("Compress needed by message count", func(t *testing.T) {
		hm := NewHistoryManager(5, 100000)

		var messages []provider.Message
		messages = append(messages, provider.Message{Role: provider.RoleSystem, Content: "System"})
		for i := 0; i < 10; i++ {
			messages = append(messages, provider.Message{Role: provider.RoleUser, Content: "User message"})
			messages = append(messages, provider.Message{Role: provider.RoleAssistant, Content: "Assistant message"})
		}

		compressed, changed := hm.Compress(messages)
		if !changed {
			t.Error("expected compression to occur")
		}
		if len(compressed) > 5 {
			t.Errorf("expected at most 5 messages, got %d", len(compressed))
		}
		// System message should be preserved
		if compressed[0].Role != provider.RoleSystem {
			t.Error("expected system message to be preserved first")
		}
	})

	t.Run("Compress preserves system messages", func(t *testing.T) {
		hm := NewHistoryManager(3, 100000)

		messages := []provider.Message{
			{Role: provider.RoleSystem, Content: "System 1"},
			{Role: provider.RoleSystem, Content: "System 2"},
			{Role: provider.RoleUser, Content: "User 1"},
			{Role: provider.RoleAssistant, Content: "Assistant 1"},
			{Role: provider.RoleUser, Content: "User 2"},
			{Role: provider.RoleAssistant, Content: "Assistant 2"},
		}

		compressed, changed := hm.Compress(messages)
		if !changed {
			t.Error("expected compression to occur")
		}

		// Count system messages
		systemCount := 0
		for _, msg := range compressed {
			if msg.Role == provider.RoleSystem {
				systemCount++
			}
		}
		if systemCount != 2 {
			t.Errorf("expected 2 system messages, got %d", systemCount)
		}
	})

	t.Run("ShouldCompress", func(t *testing.T) {
		hm := NewHistoryManager(3, 100)

		small := []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		}
		if hm.ShouldCompress(small) {
			t.Error("should not need compression for small history")
		}

		large := make([]provider.Message, 10)
		for i := range large {
			large[i] = provider.Message{Role: provider.RoleUser, Content: "Message"}
		}
		if !hm.ShouldCompress(large) {
			t.Error("should need compression for large history")
		}
	})

	t.Run("Compress empty messages", func(t *testing.T) {
		hm := NewHistoryManager(100, 100000)
		compressed, changed := hm.Compress([]provider.Message{})
		if changed {
			t.Error("expected no change for empty messages")
		}
		if len(compressed) != 0 {
			t.Errorf("expected empty result, got %d", len(compressed))
		}
	})
}

func TestRunnerErrors(t *testing.T) {
	if ErrMaxIterations.Error() == "" {
		t.Error("ErrMaxIterations should have a message")
	}
	if ErrContextCanceled.Error() == "" {
		t.Error("ErrContextCanceled should have a message")
	}
	if ErrNoProvider.Error() == "" {
		t.Error("ErrNoProvider should have a message")
	}
	if ErrNoMessages.Error() == "" {
		t.Error("ErrNoMessages should have a message")
	}
	if ErrSessionNotFound.Error() == "" {
		t.Error("ErrSessionNotFound should have a message")
	}
}
