package context

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"mote/internal/compaction"
	"mote/internal/memory"
	"mote/internal/provider"
	"mote/internal/storage"
)

// Config holds configuration for the Context Manager
type Config struct {
	MaxContextTokens       int
	TriggerThreshold       float64
	KeepRecentCount        int
	TargetCompressionRatio float64
}

// DefaultConfig returns default configuration
func DefaultConfig() Config {
	return Config{
		MaxContextTokens:       100000,
		TriggerThreshold:       0.8,
		KeepRecentCount:        20,
		TargetCompressionRatio: 0.3,
	}
}

// Manager manages conversation context lifecycle
type Manager struct {
	db           *storage.DB
	compactor    *compaction.Compactor
	memory       *memory.MemoryIndex
	tokenCounter *compaction.TokenCounter
	config       Config
}

// NewManager creates a new Context Manager
func NewManager(db *storage.DB, compactor *compaction.Compactor, mem *memory.MemoryIndex, config Config) *Manager {
	return &Manager{
		db:           db,
		compactor:    compactor,
		memory:       mem,
		tokenCounter: compaction.NewTokenCounter(),
		config:       config,
	}
}

// SetMemory sets the memory index for saving compression summaries.
func (m *Manager) SetMemory(mem *memory.MemoryIndex) {
	m.memory = mem
}

// BuildContext constructs the context to send to LLM
func (m *Manager) BuildContext(
	ctx context.Context,
	sessionID string,
	systemPrompt string,
	userInput string,
) ([]provider.Message, error) {
	var messages []provider.Message

	// 1. Add system prompt
	messages = append(messages, provider.Message{
		Role:    provider.RoleSystem,
		Content: systemPrompt,
	})

	// 2. Load latest compressed context if exists
	compressedCtx, err := m.LoadLatestContext(sessionID)
	if err != nil {
		slog.Warn("failed to load compressed context, using full history", "error", err)
		return m.buildFromFullHistory(sessionID, systemPrompt, userInput)
	}

	if compressedCtx != nil {
		// 3. Append compressed context
		messages = append(messages, compressedCtx...)

		// 4. Load new messages after compression
		newMsgs, err := m.getNewMessages(sessionID, compressedCtx)
		if err != nil {
			return nil, fmt.Errorf("get new messages: %w", err)
		}
		messages = append(messages, newMsgs...)

		slog.Info("context: loaded compressed context",
			"session_id", sessionID,
			"compressed_messages", len(compressedCtx),
			"new_messages", len(newMsgs))
	} else {
		// No compressed context, load all history
		historyMsgs, err := m.db.GetMessages(sessionID, 0)
		if err != nil {
			return nil, fmt.Errorf("get messages: %w", err)
		}
		for _, msg := range historyMsgs {
			messages = append(messages, m.toProviderMessage(msg))
		}
	}

	// 5. Add current user input
	messages = append(messages, provider.Message{
		Role:    provider.RoleUser,
		Content: userInput,
	})

	// 6. Check if compression is needed
	if m.NeedsCompression(messages) {
		slog.Info("context: compression needed", "session_id", sessionID)
		if err := m.DoCompression(ctx, sessionID, messages); err != nil {
			slog.Warn("context: compression failed", "error", err)
		}
	}

	return messages, nil
}

// LoadLatestContext loads the latest compressed context
func (m *Manager) LoadLatestContext(sessionID string) ([]provider.Message, error) {
	ctx, err := m.db.GetLatestContext(sessionID)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		return nil, nil
	}

	var messages []provider.Message

	// Add summary message
	messages = append(messages, provider.Message{
		Role:    provider.RoleAssistant,
		Content: fmt.Sprintf("[Previous conversation summary]\n%s", ctx.Summary),
	})

	// Add kept messages by loading them from database
	if len(ctx.KeptMessageIDs) > 0 {
		allMsgs, err := m.db.GetMessages(ctx.SessionID, 0)
		if err != nil {
			return nil, fmt.Errorf("get messages: %w", err)
		}

		// Build a map for quick lookup
		msgMap := make(map[string]*storage.Message)
		for _, msg := range allMsgs {
			msgMap[msg.ID] = msg
		}

		// Add kept messages in order, but skip leading tool results
		// that would appear right after the summary assistant message
		// (their original assistant+tool_calls was compacted away).
		// This prevents "tool call result does not follow tool call" errors.
		skippingLeadingTools := true
		for _, msgID := range ctx.KeptMessageIDs {
			if msg, ok := msgMap[msgID]; ok {
				if skippingLeadingTools && msg.Role == string(provider.RoleTool) {
					continue // Skip orphaned tool results at the start
				}
				skippingLeadingTools = false
				messages = append(messages, m.toProviderMessage(msg))
			}
		}
	}

	return messages, nil
}

// NeedsCompression checks if compression is needed
func (m *Manager) NeedsCompression(messages []provider.Message) bool {
	tokens := m.tokenCounter.EstimateMessages(messages)
	threshold := int(float64(m.config.MaxContextTokens) * m.config.TriggerThreshold)
	return tokens > threshold
}

// SaveContext saves a compressed context
func (m *Manager) SaveContext(
	ctx context.Context,
	sessionID string,
	summary string,
	keptMessageIDs []string,
	totalTokens int,
	originalTokens int,
) error {
	maxVersion, err := m.db.GetMaxContextVersion(sessionID)
	if err != nil {
		return fmt.Errorf("get max version: %w", err)
	}

	newCtx := &storage.Context{
		SessionID:      sessionID,
		Version:        maxVersion + 1,
		Summary:        summary,
		KeptMessageIDs: keptMessageIDs,
		TotalTokens:    totalTokens,
		OriginalTokens: originalTokens,
		CreatedAt:      time.Now(),
	}

	if err := m.db.SaveContext(newCtx); err != nil {
		return fmt.Errorf("save context: %w", err)
	}

	slog.Info("context: saved compressed context",
		"session_id", sessionID,
		"version", newCtx.Version,
		"compression_ratio", float64(totalTokens)/float64(originalTokens))

	if m.memory != nil {
		if err := m.saveSummaryToMemory(ctx, sessionID, summary, newCtx.Version, originalTokens-totalTokens); err != nil {
			slog.Warn("failed to save summary to memory", "error", err)
		}
	}

	return nil
}

func (m *Manager) saveSummaryToMemory(
	ctx context.Context,
	sessionID string,
	summary string,
	version int,
	tokensSaved int,
) error {
	// Skip saving empty summaries to avoid creating empty memory entries
	if strings.TrimSpace(summary) == "" {
		return nil
	}

	entry := memory.MemoryEntry{
		Content:   summary,
		Source:    "context_compression",
		SessionID: sessionID,
		Metadata: map[string]any{
			"context_version": version,
			"tokens_saved":    tokensSaved,
		},
		CreatedAt: time.Now(),
	}

	return m.memory.Add(ctx, entry)
}

func (m *Manager) getNewMessages(sessionID string, compressedMsgs []provider.Message) ([]provider.Message, error) {
	ctx, err := m.db.GetLatestContext(sessionID)
	if err != nil || ctx == nil {
		return nil, err
	}

	allMsgs, err := m.db.GetMessages(sessionID, 0)
	if err != nil {
		return nil, err
	}

	var newMsgs []provider.Message
	skippingLeadingTools := true
	for _, msg := range allMsgs {
		if msg.CreatedAt.After(ctx.CreatedAt) {
			// Skip leading tool results whose assistant+tool_calls may have been
			// compacted away, preventing "tool call result does not follow tool call".
			if skippingLeadingTools && msg.Role == string(provider.RoleTool) {
				continue
			}
			skippingLeadingTools = false
			newMsgs = append(newMsgs, m.toProviderMessage(msg))
		}
	}

	return newMsgs, nil
}

func (m *Manager) buildFromFullHistory(sessionID, systemPrompt, userInput string) ([]provider.Message, error) {
	var messages []provider.Message

	messages = append(messages, provider.Message{
		Role:    provider.RoleSystem,
		Content: systemPrompt,
	})

	historyMsgs, err := m.db.GetMessages(sessionID, 0)
	if err != nil {
		return nil, fmt.Errorf("get messages: %w", err)
	}

	for _, msg := range historyMsgs {
		messages = append(messages, m.toProviderMessage(msg))
	}

	messages = append(messages, provider.Message{
		Role:    provider.RoleUser,
		Content: userInput,
	})

	return messages, nil
}

// DoCompression performs compression on the messages and persists the result
func (m *Manager) DoCompression(
	ctx context.Context,
	sessionID string,
	messages []provider.Message,
) error {
	// 1. Call compactor to compress
	compressed := m.compactor.CompactWithFallback(ctx, messages, nil)

	// 2. Extract summary and kept message IDs
	summary, keptMsgIDs, err := m.extractCompressionResult(ctx, sessionID, compressed)
	if err != nil {
		return fmt.Errorf("extract compression result: %w", err)
	}

	// 3. Calculate tokens
	originalTokens := m.tokenCounter.EstimateMessages(messages)
	compressedTokens := m.tokenCounter.EstimateMessages(compressed)

	// 4. Save to contexts table
	if err := m.SaveContext(ctx, sessionID, summary, keptMsgIDs, compressedTokens, originalTokens); err != nil {
		return fmt.Errorf("save context: %w", err)
	}

	slog.Info("context: compression completed",
		"session_id", sessionID,
		"original_tokens", originalTokens,
		"compressed_tokens", compressedTokens,
		"saved_tokens", originalTokens-compressedTokens)

	return nil
}

// extractCompressionResult extracts summary and kept message IDs from compressed messages
func (m *Manager) extractCompressionResult(
	ctx context.Context,
	sessionID string,
	compressed []provider.Message,
) (string, []string, error) {
	var summary string
	var keptMsgIDs []string

	// The first assistant message with non-empty content is typically the summary.
	// Skip tool-call-only assistant messages (Content == "") to avoid extracting
	// an empty summary.
	for _, msg := range compressed {
		if msg.Role == provider.RoleAssistant && summary == "" && strings.TrimSpace(msg.Content) != "" {
			summary = msg.Content
			break
		}
	}

	// Get all messages from DB to match compressed ones
	allMsgs, err := m.db.GetMessages(sessionID, 0)
	if err != nil {
		return "", nil, fmt.Errorf("get all messages: %w", err)
	}

	// Match kept messages by content (simplified approach)
	lastMatchIndex := -1
	for _, compMsg := range compressed {
		if compMsg.Role == provider.RoleSystem {
			continue
		}
		// Skip summary message (it's the one we extracted above)
		if compMsg.Role == provider.RoleAssistant && compMsg.Content == summary {
			continue
		}

		for i := lastMatchIndex + 1; i < len(allMsgs); i++ {
			dbMsg := allMsgs[i]
			if dbMsg.Role == string(compMsg.Role) && dbMsg.Content == compMsg.Content {
				keptMsgIDs = append(keptMsgIDs, dbMsg.ID)
				lastMatchIndex = i
				break
			}
		}
	}

	return summary, keptMsgIDs, nil
}

func (m *Manager) toProviderMessage(msg *storage.Message) provider.Message {
	pMsg := provider.Message{
		Role:       msg.Role,
		Content:    msg.Content,
		ToolCallID: msg.ToolCallID,
	}

	if len(msg.ToolCalls) > 0 {
		pMsg.ToolCalls = make([]provider.ToolCall, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			pMsg.ToolCalls[i] = provider.ToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: &struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{
					Name:      tc.GetName(),
					Arguments: tc.GetArguments(),
				},
			}
		}
	}
	return pMsg
}
