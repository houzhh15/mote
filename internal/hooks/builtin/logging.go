// Package builtin provides built-in hook handlers for common use cases.
package builtin

import (
	"context"
	"fmt"
	"time"

	"mote/internal/hooks"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// LoggingHook provides logging functionality for hook events.
type LoggingHook struct {
	logger zerolog.Logger
	level  zerolog.Level
}

// LoggingConfig configures the logging hook.
type LoggingConfig struct {
	// Level is the log level to use (default: debug)
	Level zerolog.Level
	// Logger is an optional custom logger (default: global logger)
	Logger *zerolog.Logger
}

// NewLoggingHook creates a new logging hook with the given configuration.
func NewLoggingHook(cfg LoggingConfig) *LoggingHook {
	logger := log.Logger
	if cfg.Logger != nil {
		logger = *cfg.Logger
	}
	level := cfg.Level
	if level == 0 {
		level = zerolog.DebugLevel
	}
	return &LoggingHook{
		logger: logger,
		level:  level,
	}
}

// Handler returns a hook handler that logs events.
func (h *LoggingHook) Handler(id string) *hooks.Handler {
	return &hooks.Handler{
		ID:          id,
		Priority:    100, // High priority to log early
		Source:      "_builtin",
		Description: "Logs hook events",
		Enabled:     true,
		Handler:     h.handle,
	}
}

func (h *LoggingHook) handle(_ context.Context, hookCtx *hooks.Context) (*hooks.Result, error) {
	event := h.logger.WithLevel(h.level)

	event = event.
		Str("hook_type", string(hookCtx.Type)).
		Time("timestamp", hookCtx.Timestamp)

	switch hookCtx.Type {
	case hooks.HookBeforeMessage, hooks.HookAfterMessage:
		if hookCtx.Message != nil {
			event = event.
				Str("role", hookCtx.Message.Role).
				Str("from", hookCtx.Message.From).
				Int("content_length", len(hookCtx.Message.Content))
		}

	case hooks.HookBeforeToolCall, hooks.HookAfterToolCall:
		if hookCtx.ToolCall != nil {
			event = event.
				Str("tool_id", hookCtx.ToolCall.ID).
				Str("tool_name", hookCtx.ToolCall.ToolName).
				Int("param_count", len(hookCtx.ToolCall.Params))
			if hookCtx.Type == hooks.HookAfterToolCall {
				event = event.
					Dur("duration", hookCtx.ToolCall.Duration).
					Bool("has_error", hookCtx.ToolCall.Error != "")
			}
		}

	case hooks.HookSessionCreate, hooks.HookSessionEnd:
		if hookCtx.Session != nil {
			event = event.
				Str("session_id", hookCtx.Session.ID)
			if hookCtx.Type == hooks.HookSessionCreate {
				event = event.Time("created_at", hookCtx.Session.CreatedAt)
			}
		}

	case hooks.HookBeforeResponse, hooks.HookAfterResponse:
		if hookCtx.Response != nil {
			event = event.
				Int("tokens_used", hookCtx.Response.TokensUsed).
				Str("model", hookCtx.Response.Model).
				Int("content_length", len(hookCtx.Response.Content))
		}

	case hooks.HookStartup:
		event = event.Str("event", "startup")

	case hooks.HookShutdown:
		event = event.Str("event", "shutdown")
	}

	event.Msg("hook triggered")

	return hooks.ContinueResult(), nil
}

// RegisterLoggingHooks registers logging hooks for all hook types.
func RegisterLoggingHooks(manager *hooks.Manager, cfg LoggingConfig) error {
	hook := NewLoggingHook(cfg)

	for _, hookType := range hooks.AllHookTypes() {
		id := fmt.Sprintf("builtin:logging:%s", hookType)
		handler := hook.Handler(id)
		if err := manager.Register(hookType, handler); err != nil {
			return fmt.Errorf("failed to register logging hook for %s: %w", hookType, err)
		}
	}

	return nil
}

// LoggingStats tracks logging statistics.
type LoggingStats struct {
	EventCount   int64
	LastEventAt  time.Time
	EventsByType map[hooks.HookType]int64
}

// NewLoggingStats creates a new stats tracker.
func NewLoggingStats() *LoggingStats {
	return &LoggingStats{
		EventsByType: make(map[hooks.HookType]int64),
	}
}
