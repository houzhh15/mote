package compaction

import (
	"context"
	"fmt"
	"strings"

	"mote/internal/provider"
)

const summaryPrompt = `Summarize the following conversation history concisely, preserving key information, decisions, and context that would be important for continuing the conversation. Focus on:
1. Main topics discussed
2. Key decisions or conclusions reached
3. Important facts or data mentioned
4. Any pending tasks or questions

Conversation to summarize:
%s

Provide a concise summary:`

// Compactor handles compression of conversation history.
type Compactor struct {
	config          CompactionConfig
	provider        provider.Provider
	counter         *TokenCounter
	compactionCount map[string]int // session → compaction count
}

// NewCompactor creates a new Compactor.
func NewCompactor(config CompactionConfig, prov provider.Provider) *Compactor {
	return &Compactor{
		config:          config,
		provider:        prov,
		counter:         NewTokenCounter(),
		compactionCount: make(map[string]int),
	}
}

// GetCompactionCount returns the compaction count for a session.
func (c *Compactor) GetCompactionCount(sessionID string) int {
	if c.compactionCount == nil {
		return 0
	}
	return c.compactionCount[sessionID]
}

// IncrementCompactionCount increments the compaction count for a session.
func (c *Compactor) IncrementCompactionCount(sessionID string) {
	if c.compactionCount == nil {
		c.compactionCount = make(map[string]int)
	}
	c.compactionCount[sessionID]++
}

// NeedsCompaction checks if the message history needs compression.
func (c *Compactor) NeedsCompaction(messages []provider.Message) bool {
	tokens := c.counter.EstimateMessages(messages)
	threshold := int(float64(c.config.MaxContextTokens) * c.config.TriggerThreshold)
	return tokens > threshold
}

// resolveProvider returns the override provider if non-nil, otherwise falls back to c.provider.
func (c *Compactor) resolveProvider(override provider.Provider) provider.Provider {
	if override != nil {
		return override
	}
	return c.provider
}

// adjustKeepBoundary adjusts a split index so that tool call pairs
// (assistant with tool_calls → one or more tool result messages) are never split.
// Given convMsgs and a proposed splitIdx (messages[:splitIdx] are compacted,
// messages[splitIdx:] are kept), it moves splitIdx earlier if the first kept
// message is a tool result whose corresponding assistant tool_call would be
// compacted away.
func adjustKeepBoundary(convMsgs []provider.Message, splitIdx int) int {
	if splitIdx <= 0 || splitIdx >= len(convMsgs) {
		return splitIdx
	}

	// Walk backward from splitIdx while the message at splitIdx is role=tool,
	// meaning we'd be keeping orphan tool results.
	for splitIdx > 0 && convMsgs[splitIdx].Role == provider.RoleTool {
		splitIdx--
	}

	// Now splitIdx should point to either:
	// - an assistant message (possibly with tool_calls) — include it in kept
	// - a user message — safe boundary
	// - 0 — can't go further back
	return splitIdx
}

// Compact compresses the message history using LLM summarization.
// An optional provider override can be passed; if nil, the default provider is used.
func (c *Compactor) Compact(ctx context.Context, messages []provider.Message, provOverride ...provider.Provider) ([]provider.Message, error) {
	var override provider.Provider
	if len(provOverride) > 0 {
		override = provOverride[0]
	}
	prov := c.resolveProvider(override)
	if prov == nil {
		return nil, ErrNoProvider
	}

	// Separate system and conversation messages
	systemMsgs, convMsgs := c.separateMessages(messages)

	// If not enough messages to compact, return as-is
	if len(convMsgs) <= c.config.KeepRecentCount {
		return messages, ErrMessagesTooShort
	}

	// Keep recent messages, adjusting boundary to avoid splitting tool call pairs
	keepCount := c.config.KeepRecentCount
	if keepCount > len(convMsgs) {
		keepCount = len(convMsgs)
	}
	// Initial split index
	rawSplitIdx := len(convMsgs) - keepCount

	// Adjust boundary to avoid splitting tool call pairs
	splitIdx := adjustKeepBoundary(convMsgs, rawSplitIdx)

	keptMsgs := convMsgs[splitIdx:]
	toCompact := convMsgs[:splitIdx]

	// Chunk messages for summarization
	chunks := c.chunkMessages(toCompact)

	// Generate summary for each chunk
	var summaries []string
	for _, chunk := range chunks {
		summary, err := c.summarizeChunk(ctx, chunk, prov)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSummaryFailed, err)
		}
		summaries = append(summaries, summary)
	}

	// Merge summaries if multiple
	finalSummary := strings.Join(summaries, "\n\n")

	// Build result: system + summary + kept messages
	result := make([]provider.Message, 0, len(systemMsgs)+1+len(keptMsgs))
	result = append(result, systemMsgs...)
	if finalSummary != "" {
		result = append(result, provider.Message{
			Role:    provider.RoleAssistant,
			Content: fmt.Sprintf("[Previous conversation summary]\n%s", finalSummary),
		})
	}
	result = append(result, keptMsgs...)

	return result, nil
}

// CompactWithFallback attempts Compact and falls back to truncation on error.
// An optional provider override can be passed to use the session's provider.
func (c *Compactor) CompactWithFallback(ctx context.Context, messages []provider.Message, provOverride ...provider.Provider) []provider.Message {
	result, err := c.Compact(ctx, messages, provOverride...)
	if err == nil && c.hasConversationContent(result) {
		return result
	}

	// Fallback to simple truncation
	truncated := c.truncate(messages)
	if c.hasConversationContent(truncated) {
		return truncated
	}

	// Safety net: if both compact and truncate produced invalid results,
	// return the original messages unchanged.  Sending too many tokens is
	// better than sending an invalid (empty) request.
	return messages
}

// hasConversationContent checks that a message list contains at least one
// user or assistant message, which is the minimum for a valid LLM request.
func (c *Compactor) hasConversationContent(messages []provider.Message) bool {
	for _, m := range messages {
		if m.Role == provider.RoleUser || m.Role == provider.RoleAssistant {
			return true
		}
	}
	return false
}

// separateMessages splits messages into system and conversation messages.
func (c *Compactor) separateMessages(messages []provider.Message) ([]provider.Message, []provider.Message) {
	var systemMsgs, convMsgs []provider.Message
	for _, msg := range messages {
		if msg.Role == provider.RoleSystem {
			systemMsgs = append(systemMsgs, msg)
		} else {
			convMsgs = append(convMsgs, msg)
		}
	}
	return systemMsgs, convMsgs
}

// chunkMessages splits messages into chunks based on token limit.
func (c *Compactor) chunkMessages(messages []provider.Message) [][]provider.Message {
	if len(messages) == 0 {
		return nil
	}

	var chunks [][]provider.Message
	var currentChunk []provider.Message
	currentTokens := 0

	for _, msg := range messages {
		msgTokens := c.counter.EstimateMessages([]provider.Message{msg})
		if currentTokens+msgTokens > c.config.ChunkMaxTokens && len(currentChunk) > 0 {
			chunks = append(chunks, currentChunk)
			currentChunk = nil
			currentTokens = 0
		}
		currentChunk = append(currentChunk, msg)
		currentTokens += msgTokens
	}

	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}

// summarizeChunk generates a summary for a chunk of messages.
func (c *Compactor) summarizeChunk(ctx context.Context, chunk []provider.Message, prov provider.Provider) (string, error) {
	// Format messages for summarization
	var sb strings.Builder
	for _, msg := range chunk {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}

	prompt := fmt.Sprintf(summaryPrompt, sb.String())

	req := provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: prompt},
		},
		MaxTokens: c.config.SummaryMaxTokens,
	}

	resp, err := prov.Chat(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// truncateMessageContent truncates a single message's content to fit within
// maxTokens, appending a truncation notice.  Returns a shallow copy.
func (c *Compactor) truncateMessageContent(msg provider.Message, maxTokens int) provider.Message {
	msgTokens := c.counter.EstimateMessages([]provider.Message{msg})
	if msgTokens <= maxTokens || len(msg.Content) == 0 {
		return msg
	}

	// Rough ratio: cut content proportionally.
	ratio := float64(maxTokens) / float64(msgTokens)
	if ratio > 0.95 {
		return msg
	}
	cutLen := int(float64(len(msg.Content)) * ratio)
	if cutLen < 200 {
		cutLen = 200
	}
	if cutLen >= len(msg.Content) {
		return msg
	}

	truncated := msg
	truncated.Content = msg.Content[:cutLen] + "\n\n[... content truncated due to length ...]"
	return truncated
}

// truncate performs simple truncation keeping recent messages.
// It guarantees the result always contains at least the most recent user
// message round (user + optional assistant/tool follow-ups) so the request
// sent to the provider is never empty.
//
// Strategy:
//  1. Walk backward through conversation messages, keeping as many as fit.
//  2. If a single message is too large to fit, truncate its content instead
//     of discarding it entirely.  This handles the common case of HTTP tool
//     results containing huge web pages.
//  3. As a last resort, force-keep the most recent round with truncation.
func (c *Compactor) truncate(messages []provider.Message) []provider.Message {
	systemMsgs, convMsgs := c.separateMessages(messages)

	if len(convMsgs) == 0 {
		return messages
	}

	// Calculate available space
	systemTokens := c.counter.EstimateMessages(systemMsgs)
	availableTokens := c.config.MaxContextTokens - systemTokens
	if availableTokens < 0 {
		availableTokens = 1000
	}

	// Maximum tokens a single message is allowed to consume.
	// This prevents one huge tool result from eating all available space.
	maxSingleMsgTokens := availableTokens / 2

	// Keep most recent messages that fit (walk backward).
	// Oversized messages are truncated to maxSingleMsgTokens before the
	// fit check so they don't block subsequent (older) messages.
	keptTokens := 0
	splitIdx := len(convMsgs)
	truncatedMap := make(map[int]provider.Message) // index → truncated copy

	for i := len(convMsgs) - 1; i >= 0; i-- {
		msg := convMsgs[i]
		msgTokens := c.counter.EstimateMessages([]provider.Message{msg})

		// If this single message exceeds the cap, truncate its content.
		if msgTokens > maxSingleMsgTokens {
			msg = c.truncateMessageContent(msg, maxSingleMsgTokens)
			msgTokens = c.counter.EstimateMessages([]provider.Message{msg})
			truncatedMap[i] = msg
		}

		if keptTokens+msgTokens > availableTokens {
			break
		}
		keptTokens += msgTokens
		splitIdx = i
	}

	// Adjust boundary to avoid splitting tool call pairs
	splitIdx = adjustKeepBoundary(convMsgs, splitIdx)

	// Build kept messages, using truncated copies where available.
	var keptMsgs []provider.Message
	for i := splitIdx; i < len(convMsgs); i++ {
		if tm, ok := truncatedMap[i]; ok {
			keptMsgs = append(keptMsgs, tm)
		} else {
			keptMsgs = append(keptMsgs, convMsgs[i])
		}
	}

	// Safety: if nothing fits even after truncation, force-keep the most
	// recent complete conversation round with aggressive truncation.
	if len(keptMsgs) == 0 {
		roundStart := len(convMsgs) - 1
		for roundStart > 0 && convMsgs[roundStart].Role != provider.RoleUser {
			roundStart--
		}
		round := convMsgs[roundStart:]

		maxPerMsg := availableTokens / len(round)
		if maxPerMsg < 50 {
			maxPerMsg = 50
		}
		for _, m := range round {
			keptMsgs = append(keptMsgs, c.truncateMessageContent(m, maxPerMsg))
		}
	}

	result := make([]provider.Message, 0, len(systemMsgs)+len(keptMsgs))
	result = append(result, systemMsgs...)
	result = append(result, keptMsgs...)
	return result
}
