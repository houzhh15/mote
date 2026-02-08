package hooks

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// Manager manages hook registration and execution.
type Manager struct {
	registry *Registry
	executor *Executor
}

// NewManager creates a new hook manager.
func NewManager() *Manager {
	return &Manager{
		registry: NewRegistry(),
		executor: NewExecutor(),
	}
}

// ManagerOption configures the manager.
type ManagerOption func(*Manager)

// WithRegistry sets a custom registry.
func WithRegistry(r *Registry) ManagerOption {
	return func(m *Manager) {
		m.registry = r
	}
}

// WithExecutor sets a custom executor.
func WithExecutor(e *Executor) ManagerOption {
	return func(m *Manager) {
		m.executor = e
	}
}

// NewManagerWithOptions creates a new hook manager with options.
func NewManagerWithOptions(opts ...ManagerOption) *Manager {
	m := NewManager()
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Register registers a handler for the given hook type.
func (m *Manager) Register(hookType HookType, handler *Handler) error {
	return m.registry.Register(hookType, handler)
}

// Unregister removes a handler from the given hook type.
func (m *Manager) Unregister(hookType HookType, handlerID string) error {
	return m.registry.Unregister(hookType, handlerID)
}

// Trigger triggers a hook and returns the result.
// It builds a context, retrieves handlers, and executes them.
func (m *Manager) Trigger(ctx context.Context, hookCtx *Context) (*Result, error) {
	if hookCtx == nil {
		hookCtx = NewContext(HookBeforeMessage)
	}

	if !IsValidHookType(hookCtx.Type) {
		return nil, ErrHookTypeInvalid
	}

	handlers := m.registry.GetHandlers(hookCtx.Type)
	if len(handlers) == 0 {
		return ContinueResult(), nil
	}

	log.Debug().
		Str("hook_type", string(hookCtx.Type)).
		Int("handler_count", len(handlers)).
		Msg("triggering hook")

	result := m.executor.Execute(ctx, handlers, hookCtx)

	return result, nil
}

// TriggerBeforeMessage triggers a before_message hook.
func (m *Manager) TriggerBeforeMessage(ctx context.Context, content, role, from string) (*Result, error) {
	hookCtx := NewContext(HookBeforeMessage)
	hookCtx.Message = &MessageContext{
		Content: content,
		Role:    role,
		From:    from,
	}
	return m.Trigger(ctx, hookCtx)
}

// TriggerAfterMessage triggers an after_message hook.
func (m *Manager) TriggerAfterMessage(ctx context.Context, content, role string) (*Result, error) {
	hookCtx := NewContext(HookAfterMessage)
	hookCtx.Message = &MessageContext{
		Content: content,
		Role:    role,
	}
	return m.Trigger(ctx, hookCtx)
}

// TriggerBeforeToolCall triggers a before_tool_call hook.
func (m *Manager) TriggerBeforeToolCall(ctx context.Context, toolID, toolName string, params map[string]any) (*Result, error) {
	hookCtx := NewContext(HookBeforeToolCall)
	hookCtx.ToolCall = &ToolCallContext{
		ID:       toolID,
		ToolName: toolName,
		Params:   params,
	}
	return m.Trigger(ctx, hookCtx)
}

// TriggerAfterToolCall triggers an after_tool_call hook.
func (m *Manager) TriggerAfterToolCall(ctx context.Context, toolID, toolName string, params map[string]any, result any, toolErr string, duration time.Duration) (*Result, error) {
	hookCtx := NewContext(HookAfterToolCall)
	hookCtx.ToolCall = &ToolCallContext{
		ID:       toolID,
		ToolName: toolName,
		Params:   params,
		Result:   result,
		Error:    toolErr,
		Duration: duration,
	}
	return m.Trigger(ctx, hookCtx)
}

// TriggerSessionCreate triggers a session_create hook.
func (m *Manager) TriggerSessionCreate(ctx context.Context, sessionID string, createdAt time.Time, metadata map[string]any) (*Result, error) {
	hookCtx := NewContext(HookSessionCreate)
	hookCtx.Session = &SessionContext{
		ID:        sessionID,
		CreatedAt: createdAt,
		Metadata:  metadata,
	}
	return m.Trigger(ctx, hookCtx)
}

// TriggerSessionEnd triggers a session_end hook.
func (m *Manager) TriggerSessionEnd(ctx context.Context, sessionID string) (*Result, error) {
	hookCtx := NewContext(HookSessionEnd)
	hookCtx.Session = &SessionContext{
		ID: sessionID,
	}
	return m.Trigger(ctx, hookCtx)
}

// TriggerBeforeResponse triggers a before_response hook.
func (m *Manager) TriggerBeforeResponse(ctx context.Context, content string, tokensUsed int, model string) (*Result, error) {
	hookCtx := NewContext(HookBeforeResponse)
	hookCtx.Response = &ResponseContext{
		Content:    content,
		TokensUsed: tokensUsed,
		Model:      model,
	}
	return m.Trigger(ctx, hookCtx)
}

// TriggerAfterResponse triggers an after_response hook.
func (m *Manager) TriggerAfterResponse(ctx context.Context, content string, tokensUsed int, model string) (*Result, error) {
	hookCtx := NewContext(HookAfterResponse)
	hookCtx.Response = &ResponseContext{
		Content:    content,
		TokensUsed: tokensUsed,
		Model:      model,
	}
	return m.Trigger(ctx, hookCtx)
}

// TriggerStartup triggers a startup hook.
func (m *Manager) TriggerStartup(ctx context.Context) (*Result, error) {
	return m.Trigger(ctx, NewContext(HookStartup))
}

// TriggerShutdown triggers a shutdown hook.
func (m *Manager) TriggerShutdown(ctx context.Context) (*Result, error) {
	return m.Trigger(ctx, NewContext(HookShutdown))
}

// ListHandlers returns all handlers for the given hook type.
func (m *Manager) ListHandlers(hookType HookType) []*Handler {
	return m.registry.GetHandlers(hookType)
}

// HasHandlers returns true if there are any handlers for the given hook type.
func (m *Manager) HasHandlers(hookType HookType) bool {
	return m.registry.HasHandlers(hookType)
}

// Clear removes all registered handlers.
func (m *Manager) Clear() {
	m.registry.Clear()
}

// Close releases resources.
func (m *Manager) Close() error {
	m.registry.Clear()
	return nil
}
