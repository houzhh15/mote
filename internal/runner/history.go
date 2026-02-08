package runner

import (
	"mote/internal/provider"
)

// HistoryManager manages conversation history with token and message limits.
type HistoryManager struct {
	maxMessages int
	maxTokens   int
}

// NewHistoryManager creates a new HistoryManager.
func NewHistoryManager(maxMessages, maxTokens int) *HistoryManager {
	if maxMessages <= 0 {
		maxMessages = 100
	}
	if maxTokens <= 0 {
		maxTokens = 100000
	}
	return &HistoryManager{
		maxMessages: maxMessages,
		maxTokens:   maxTokens,
	}
}

// EstimateTokens estimates the token count for content.
// This uses a simple heuristic: ~4 characters per token for English.
func (hm *HistoryManager) EstimateTokens(content string) int {
	// Rough estimation: 1 token ≈ 4 characters for English
	// For Chinese/Japanese, it's closer to 1 token ≈ 1.5 characters
	// We use a middle-ground estimate
	return (len(content) + 2) / 3
}

// EstimateMessagesTokens estimates total tokens for a message slice.
func (hm *HistoryManager) EstimateMessagesTokens(messages []provider.Message) int {
	total := 0
	for _, msg := range messages {
		// Count content tokens
		total += hm.EstimateTokens(msg.Content)
		// Count role overhead (~4 tokens per message)
		total += 4
		// Count tool calls
		for _, tc := range msg.ToolCalls {
			total += hm.EstimateTokens(tc.Arguments)
			if tc.Function != nil {
				total += hm.EstimateTokens(tc.Function.Name)
				total += hm.EstimateTokens(tc.Function.Arguments)
			}
		}
	}
	return total
}

// Compress compresses the message history to fit within limits.
// It preserves system messages and the most recent messages.
// Returns the compressed messages and whether compression occurred.
func (hm *HistoryManager) Compress(messages []provider.Message) ([]provider.Message, bool) {
	if len(messages) == 0 {
		return messages, false
	}

	// Check if compression is needed
	tokens := hm.EstimateMessagesTokens(messages)
	if len(messages) <= hm.maxMessages && tokens <= hm.maxTokens {
		return messages, false
	}

	// Separate system messages and conversation messages
	var systemMsgs []provider.Message
	var convMsgs []provider.Message

	for _, msg := range messages {
		if msg.Role == provider.RoleSystem {
			systemMsgs = append(systemMsgs, msg)
		} else {
			convMsgs = append(convMsgs, msg)
		}
	}

	// Calculate available slots for conversation
	systemTokens := hm.EstimateMessagesTokens(systemMsgs)
	availableTokens := hm.maxTokens - systemTokens
	availableMessages := hm.maxMessages - len(systemMsgs)

	if availableMessages <= 0 {
		availableMessages = 2 // Keep at least the last exchange
	}
	if availableTokens <= 0 {
		availableTokens = 1000 // Reserve some tokens for response
	}

	// Keep the most recent messages that fit
	var keptMsgs []provider.Message
	for i := len(convMsgs) - 1; i >= 0; i-- {
		msg := convMsgs[i]
		msgTokens := hm.EstimateTokens(msg.Content) + 4

		if len(keptMsgs) >= availableMessages {
			break
		}
		if hm.EstimateMessagesTokens(keptMsgs)+msgTokens > availableTokens {
			break
		}

		keptMsgs = append([]provider.Message{msg}, keptMsgs...)
	}

	// Combine system + kept messages
	result := make([]provider.Message, 0, len(systemMsgs)+len(keptMsgs))
	result = append(result, systemMsgs...)
	result = append(result, keptMsgs...)

	return result, true
}

// ShouldCompress checks if the message history needs compression.
func (hm *HistoryManager) ShouldCompress(messages []provider.Message) bool {
	tokens := hm.EstimateMessagesTokens(messages)
	return len(messages) > hm.maxMessages || tokens > hm.maxTokens
}
