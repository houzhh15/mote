package compaction

import (
	"context"
	"fmt"
	"strings"

	"mote/internal/memory"
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

const extractKeyInfoPrompt = `Extract key facts, decisions, and learnings from this conversation that should be remembered long-term. Format as bullet points. Be concise and focus only on truly important information.

Conversation:
%s

Key information to remember:`

// MemoryFlusher is an interface for flushing memories before compaction.
type MemoryFlusher interface {
	AppendDailyLog(ctx context.Context, content, section string) error
}

// Compactor handles compression of conversation history.
type Compactor struct {
	config          CompactionConfig
	provider        provider.Provider
	counter         *TokenCounter
	memoryFlusher   MemoryFlusher  // P1: Optional memory flusher for pre-compaction save
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

// Compact compresses the message history using LLM summarization.
func (c *Compactor) Compact(ctx context.Context, messages []provider.Message) ([]provider.Message, error) {
	if c.provider == nil {
		return nil, ErrNoProvider
	}

	// Separate system and conversation messages
	systemMsgs, convMsgs := c.separateMessages(messages)

	// If not enough messages to compact, return as-is
	if len(convMsgs) <= c.config.KeepRecentCount {
		return messages, ErrMessagesTooShort
	}

	// Keep recent messages
	keepCount := c.config.KeepRecentCount
	if keepCount > len(convMsgs) {
		keepCount = len(convMsgs)
	}
	keptMsgs := convMsgs[len(convMsgs)-keepCount:]
	toCompact := convMsgs[:len(convMsgs)-keepCount]

	// Chunk messages for summarization
	chunks := c.chunkMessages(toCompact)

	// Generate summary for each chunk
	var summaries []string
	for _, chunk := range chunks {
		summary, err := c.summarizeChunk(ctx, chunk)
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
func (c *Compactor) CompactWithFallback(ctx context.Context, messages []provider.Message) []provider.Message {
	result, err := c.Compact(ctx, messages)
	if err == nil {
		return result
	}

	// Fallback to simple truncation
	return c.truncate(messages)
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
func (c *Compactor) summarizeChunk(ctx context.Context, chunk []provider.Message) (string, error) {
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

	resp, err := c.provider.Chat(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// truncate performs simple truncation keeping recent messages.
func (c *Compactor) truncate(messages []provider.Message) []provider.Message {
	systemMsgs, convMsgs := c.separateMessages(messages)

	// Calculate available space
	systemTokens := c.counter.EstimateMessages(systemMsgs)
	availableTokens := c.config.MaxContextTokens - systemTokens
	if availableTokens < 0 {
		availableTokens = 1000
	}

	// Keep most recent messages that fit
	var keptMsgs []provider.Message
	for i := len(convMsgs) - 1; i >= 0; i-- {
		msg := convMsgs[i]
		msgTokens := c.counter.EstimateMessages([]provider.Message{msg})
		if c.counter.EstimateMessages(keptMsgs)+msgTokens > availableTokens {
			break
		}
		keptMsgs = append([]provider.Message{msg}, keptMsgs...)
	}

	result := make([]provider.Message, 0, len(systemMsgs)+len(keptMsgs))
	result = append(result, systemMsgs...)
	result = append(result, keptMsgs...)
	return result
}

// SetMemoryFlusher sets the memory flusher for pre-compaction saves.
func (c *Compactor) SetMemoryFlusher(flusher MemoryFlusher) {
	c.memoryFlusher = flusher
}

// SetMemoryIndex sets a MemoryIndex as the memory flusher.
func (c *Compactor) SetMemoryIndex(m *memory.MemoryIndex) {
	c.memoryFlusher = m
}

// CompactWithFlush extracts key info and saves to memory before compacting.
func (c *Compactor) CompactWithFlush(ctx context.Context, messages []provider.Message) ([]provider.Message, error) {
	// First, flush important info to memory if configured
	if c.memoryFlusher != nil && c.config.FlushOnCompact {
		_ = c.flushToMemory(ctx, messages) // Ignore error - compaction should still proceed
	}

	// Then do normal compaction
	return c.Compact(ctx, messages)
}

// flushToMemory extracts key information and saves it to the daily log.
func (c *Compactor) flushToMemory(ctx context.Context, messages []provider.Message) error {
	if c.provider == nil {
		return nil
	}

	// Separate conversation messages (skip system)
	_, convMsgs := c.separateMessages(messages)
	if len(convMsgs) < 3 {
		return nil // Not enough content to extract
	}

	// Format messages for extraction
	var sb strings.Builder
	for _, msg := range convMsgs {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}

	prompt := fmt.Sprintf(extractKeyInfoPrompt, sb.String())

	req := provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: prompt},
		},
		MaxTokens: c.config.SummaryMaxTokens,
	}

	resp, err := c.provider.Chat(ctx, req)
	if err != nil {
		return fmt.Errorf("extract key info: %w", err)
	}

	if resp.Content == "" {
		return nil
	}

	// Save to daily log
	return c.memoryFlusher.AppendDailyLog(ctx, resp.Content, "压缩前提取")
}
