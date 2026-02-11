// Package builtin provides built-in hook handlers for common use cases.
package builtin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mote/internal/hooks"
	"mote/internal/memory"

	"github.com/rs/zerolog"
)

// MemoryHookBridge provides memory integration hooks for auto-recall and auto-capture.
// It directly integrates with CaptureEngine and RecallEngine without an intermediate layer.
type MemoryHookBridge struct {
	captureEngine *memory.CaptureEngine
	recallEngine  *memory.RecallEngine
	logger        zerolog.Logger
}

// MemoryHookConfig configures the memory hook bridge.
type MemoryHookConfig struct {
	CaptureEngine *memory.CaptureEngine
	RecallEngine  *memory.RecallEngine
	Logger        zerolog.Logger
}

// NewMemoryHookBridge creates a new memory hook bridge.
func NewMemoryHookBridge(cfg MemoryHookConfig) *MemoryHookBridge {
	return &MemoryHookBridge{
		captureEngine: cfg.CaptureEngine,
		recallEngine:  cfg.RecallEngine,
		logger:        cfg.Logger,
	}
}

// BeforeMessageHandler returns a hook handler for before_message events.
// It injects relevant memories into the message context.
func (m *MemoryHookBridge) BeforeMessageHandler(id string) *hooks.Handler {
	return &hooks.Handler{
		ID:          id,
		Priority:    50, // Medium priority
		Source:      "_builtin",
		Description: "Auto-recalls relevant memories before processing message",
		Enabled:     true,
		Handler:     m.handleBeforeMessage,
	}
}

// AfterMessageHandler returns a hook handler for after_message events.
// It captures important information from the conversation.
func (m *MemoryHookBridge) AfterMessageHandler(id string) *hooks.Handler {
	return &hooks.Handler{
		ID:          id,
		Priority:    -50, // Low priority (run after others)
		Source:      "_builtin",
		Description: "Auto-captures important information after message processing",
		Enabled:     true,
		Handler:     m.handleAfterMessage,
	}
}

// handleBeforeMessage processes before_message hooks for memory recall.
func (m *MemoryHookBridge) handleBeforeMessage(ctx context.Context, hookCtx *hooks.Context) (*hooks.Result, error) {
	if m.recallEngine == nil || hookCtx.Message == nil {
		return nil, nil
	}

	// Only process user messages
	if hookCtx.Message.Role != "user" {
		return nil, nil
	}

	// Recall relevant memories
	memoryContext, err := m.recallEngine.Recall(ctx, hookCtx.Message.Content)
	if err != nil {
		m.logger.Warn().Err(err).Msg("memory recall failed in hook")
		return nil, nil // Don't fail the hook
	}

	if memoryContext == "" {
		return nil, nil
	}

	m.logger.Debug().
		Str("prompt", truncate(hookCtx.Message.Content, 50)).
		Int("context_len", len(memoryContext)).
		Msg("injected memory context")

	// Return the memory context as data for injection
	return &hooks.Result{
		Continue: true,
		Modified: true,
		Data: map[string]any{
			"memory_context": memoryContext,
		},
	}, nil
}

// handleAfterMessage processes after_message hooks for memory capture.
func (m *MemoryHookBridge) handleAfterMessage(ctx context.Context, hookCtx *hooks.Context) (*hooks.Result, error) {
	if m.captureEngine == nil {
		return nil, nil
	}

	// Collect messages from context
	var messages []memory.Message
	if hookCtx.Message != nil {
		messages = append(messages, memory.Message{
			Role:    hookCtx.Message.Role,
			Content: hookCtx.Message.Content,
		})
	}

	// Check if there's a response in context
	if hookCtx.Response != nil && hookCtx.Response.Content != "" {
		messages = append(messages, memory.Message{
			Role:    "assistant",
			Content: hookCtx.Response.Content,
		})
	}

	if len(messages) == 0 {
		return nil, nil
	}

	// Capture relevant memories
	captured, err := m.captureEngine.Capture(ctx, messages)
	if err != nil {
		m.logger.Warn().Err(err).Msg("memory capture failed in hook")
		return nil, nil // Don't fail the hook
	}

	if captured > 0 {
		m.logger.Info().Int("count", captured).Msg("auto-captured memories")
	}

	return nil, nil
}

// SessionCreateHandler returns a hook handler for session_create events.
// It resets the capture session counter.
func (m *MemoryHookBridge) SessionCreateHandler(id string) *hooks.Handler {
	return &hooks.Handler{
		ID:          id,
		Priority:    0,
		Source:      "_builtin",
		Description: "Resets memory capture counter for new session",
		Enabled:     true,
		Handler:     m.handleSessionCreate,
	}
}

func (m *MemoryHookBridge) handleSessionCreate(_ context.Context, hookCtx *hooks.Context) (*hooks.Result, error) {
	if m.captureEngine == nil {
		return nil, nil
	}

	sessionID := ""
	if hookCtx.Session != nil {
		sessionID = hookCtx.Session.ID
	}

	m.captureEngine.SetSessionID(sessionID)
	m.captureEngine.ResetSession()
	return nil, nil
}

// SessionEndHandler returns a hook handler for session_end events.
// It saves the session context to a daily memory file.
func (m *MemoryHookBridge) SessionEndHandler(id string) *hooks.Handler {
	return &hooks.Handler{
		ID:          id,
		Priority:    -100, // Very low priority (run last)
		Source:      "_builtin",
		Description: "Saves session context to memory on session end",
		Enabled:     true,
		Handler:     m.handleSessionEnd,
	}
}

func (m *MemoryHookBridge) handleSessionEnd(ctx context.Context, hookCtx *hooks.Context) (*hooks.Result, error) {
	if m.captureEngine == nil {
		return nil, nil
	}

	sessionID := ""
	if hookCtx.Session != nil {
		sessionID = hookCtx.Session.ID
	}

	// Get recent messages from session data if available
	var messages []memory.Message
	if hookCtx.Data != nil {
		if recentMsgs, ok := hookCtx.Data["recent_messages"].([]memory.Message); ok {
			messages = recentMsgs
		}
	}

	// Build session summary content
	if len(messages) == 0 {
		m.logger.Debug().Str("session", sessionID).Msg("no messages to save for session end")
		return nil, nil
	}

	content := m.buildSessionSummary(hookCtx.Session, messages)
	section := "会话结束"

	// Save to daily log via CaptureEngine's memory index
	memoryIndex := m.captureEngine.GetMemoryIndex()
	if memoryIndex == nil {
		return nil, nil
	}

	if err := memoryIndex.AppendDailyLog(ctx, content, section); err != nil {
		m.logger.Warn().Err(err).Str("session", sessionID).Msg("failed to save session end to memory")
		return nil, nil // Don't fail the hook
	}

	m.logger.Info().Str("session", sessionID).Int("messages", len(messages)).Msg("saved session end to memory")
	return nil, nil
}

// buildSessionSummary builds a summary content for the session.
func (m *MemoryHookBridge) buildSessionSummary(session *hooks.SessionContext, messages []memory.Message) string {
	var builder strings.Builder

	// Add session metadata
	if session != nil {
		builder.WriteString(fmt.Sprintf("**Session ID**: %s\n", session.ID))
		if !session.CreatedAt.IsZero() {
			builder.WriteString(fmt.Sprintf("**Started**: %s\n", session.CreatedAt.Format(time.RFC3339)))
		}
		builder.WriteString(fmt.Sprintf("**Ended**: %s\n\n", time.Now().Format(time.RFC3339)))
	}

	// Add conversation summary (last N messages)
	maxMessages := 10
	if len(messages) < maxMessages {
		maxMessages = len(messages)
	}
	startIdx := len(messages) - maxMessages

	builder.WriteString("### 对话摘要\n\n")
	for i := startIdx; i < len(messages); i++ {
		msg := messages[i]
		role := msg.Role
		if role == "user" {
			role = "用户"
		} else if role == "assistant" {
			role = "助手"
		}
		// Truncate long messages
		content := msg.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		builder.WriteString(fmt.Sprintf("**%s**: %s\n\n", role, content))
	}

	return builder.String()
}

// truncate truncates a string to maxLen characters with "..." suffix.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
