package memory

import (
	"context"

	"github.com/rs/zerolog"
)

// MemoryHooks provides lifecycle hooks for memory auto-capture and recall.
// It integrates with the agent runner to automatically inject relevant memories
// before processing and capture important information after conversations.
type MemoryHooks struct {
	capture *CaptureEngine
	recall  *RecallEngine
	logger  zerolog.Logger
}

// MemoryHooksOptions holds options for creating MemoryHooks.
type MemoryHooksOptions struct {
	Capture *CaptureEngine
	Recall  *RecallEngine
	Logger  zerolog.Logger
}

// NewMemoryHooks creates a new MemoryHooks instance.
func NewMemoryHooks(opts MemoryHooksOptions) *MemoryHooks {
	return &MemoryHooks{
		capture: opts.Capture,
		recall:  opts.Recall,
		logger:  opts.Logger,
	}
}

// BeforeRun is called before the agent processes a prompt.
// It recalls relevant memories and returns formatted context to inject.
// Returns empty string if no relevant memories are found.
func (h *MemoryHooks) BeforeRun(ctx context.Context, prompt string) (string, error) {
	if h.recall == nil {
		return "", nil
	}

	context, err := h.recall.Recall(ctx, prompt)
	if err != nil {
		h.logger.Warn().Err(err).Msg("memory recall failed")
		// Don't fail the run, just return empty context
		return "", nil
	}

	if context != "" {
		h.logger.Debug().
			Str("prompt", truncate(prompt, 50)).
			Int("context_len", len(context)).
			Msg("injected memory context")
	}

	return context, nil
}

// AfterRun is called after the agent completes processing.
// It analyzes the conversation and captures relevant information.
func (h *MemoryHooks) AfterRun(ctx context.Context, messages []Message, success bool) error {
	if h.capture == nil {
		return nil
	}

	// Only capture on successful runs
	if !success {
		h.logger.Debug().Msg("skipping capture for failed run")
		return nil
	}

	captured, err := h.capture.Capture(ctx, messages)
	if err != nil {
		h.logger.Warn().Err(err).Msg("memory capture failed")
		// Don't fail the run, just log the error
		return nil
	}

	if captured > 0 {
		h.logger.Info().Int("count", captured).Msg("auto-captured memories")
	}

	return nil
}

// ResetSession resets the capture session counter.
// Should be called at the start of a new conversation session.
func (h *MemoryHooks) ResetSession() {
	if h.capture != nil {
		h.capture.ResetSession()
	}
}

// SetSessionID sets the current session ID for tracking captures.
func (h *MemoryHooks) SetSessionID(id string) {
	if h.capture != nil {
		h.capture.SetSessionID(id)
	}
}

// SetEnabled enables or disables both capture and recall.
func (h *MemoryHooks) SetEnabled(enabled bool) {
	if h.capture != nil {
		h.capture.config.Enabled = enabled
	}
	if h.recall != nil {
		h.recall.SetEnabled(enabled)
	}
}

// IsCaptureEnabled returns whether capture is enabled.
func (h *MemoryHooks) IsCaptureEnabled() bool {
	return h.capture != nil && h.capture.config.Enabled
}

// IsRecallEnabled returns whether recall is enabled.
func (h *MemoryHooks) IsRecallEnabled() bool {
	return h.recall != nil && h.recall.GetConfig().Enabled
}

// SaveSessionEnd saves session end content to the daily memory log.
// This is called by the session_end hook to persist session context.
func (h *MemoryHooks) SaveSessionEnd(ctx context.Context, content, section string) error {
	if h.capture == nil {
		return nil
	}

	// Use the capture engine's memory index to append to daily log
	memoryIndex := h.capture.GetMemoryIndex()
	if memoryIndex == nil {
		return nil
	}

	return memoryIndex.AppendDailyLog(ctx, content, section)
}
