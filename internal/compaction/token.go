package compaction

import (
	"mote/internal/provider"
)

// TokenCounter estimates token counts for text and messages.
type TokenCounter struct{}

// NewTokenCounter creates a new TokenCounter.
func NewTokenCounter() *TokenCounter {
	return &TokenCounter{}
}

// EstimateText estimates the token count for a given text.
// This uses a simple heuristic: approximately 3 characters per token,
// which works reasonably well for mixed English/Chinese content.
func (tc *TokenCounter) EstimateText(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + 2) / 3
}

// EstimateMessages estimates the total token count for a slice of messages.
// It accounts for:
// - Content tokens
// - Role overhead (~4 tokens per message)
// - Tool call arguments
func (tc *TokenCounter) EstimateMessages(messages []provider.Message) int {
	total := 0
	for _, msg := range messages {
		// Count content tokens
		total += tc.EstimateText(msg.Content)
		// Add role overhead (~4 tokens per message for role, separators, etc.)
		total += 4
		// Count tool call tokens
		for _, toolCall := range msg.ToolCalls {
			// Count arguments
			total += tc.EstimateText(toolCall.Arguments)
			// Count function details if present
			if toolCall.Function != nil {
				total += tc.EstimateText(toolCall.Function.Name)
				total += tc.EstimateText(toolCall.Function.Arguments)
			}
		}
	}
	return total
}
