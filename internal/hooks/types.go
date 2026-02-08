// Package hooks provides the hook system for Mote Agent Runtime.
// Hooks allow intercepting and modifying agent behavior at key lifecycle points.
package hooks

import (
	"context"
	"time"
)

// HookType represents the type of hook event.
type HookType string

// Hook type constants.
const (
	// Message lifecycle
	HookBeforeMessage HookType = "before_message"
	HookAfterMessage  HookType = "after_message"

	// Tool call lifecycle
	HookBeforeToolCall HookType = "before_tool_call"
	HookAfterToolCall  HookType = "after_tool_call"

	// Session lifecycle
	HookSessionCreate HookType = "session_create"
	HookSessionEnd    HookType = "session_end"

	// Agent lifecycle
	HookStartup  HookType = "startup"
	HookShutdown HookType = "shutdown"

	// Response lifecycle
	HookBeforeResponse HookType = "before_response"
	HookAfterResponse  HookType = "after_response"

	// Memory lifecycle
	HookBeforeMemoryWrite HookType = "before_memory_write"
	HookAfterMemoryWrite  HookType = "after_memory_write"

	// Prompt lifecycle
	HookPromptBuild HookType = "prompt_build"

	// Error handling
	HookOnError HookType = "on_error"
)

// AllHookTypes returns all supported hook types.
func AllHookTypes() []HookType {
	return []HookType{
		HookBeforeMessage,
		HookAfterMessage,
		HookBeforeToolCall,
		HookAfterToolCall,
		HookSessionCreate,
		HookSessionEnd,
		HookStartup,
		HookShutdown,
		HookBeforeResponse,
		HookAfterResponse,
		HookBeforeMemoryWrite,
		HookAfterMemoryWrite,
		HookPromptBuild,
		HookOnError,
	}
}

// IsValidHookType checks if the given type is a valid hook type.
func IsValidHookType(t HookType) bool {
	for _, ht := range AllHookTypes() {
		if ht == t {
			return true
		}
	}
	return false
}

// HandlerFunc is the function signature for hook handlers.
type HandlerFunc func(ctx context.Context, hookCtx *Context) (*Result, error)

// Handler represents a registered hook handler.
type Handler struct {
	ID          string      `json:"id"`
	Priority    int         `json:"priority"`              // Higher = earlier execution, default 0
	Source      string      `json:"source"`                // "builtin" | skillID
	Handler     HandlerFunc `json:"-"`                     // The actual handler function
	Description string      `json:"description,omitempty"` // Human-readable description
	Enabled     bool        `json:"enabled"`               // Whether the handler is enabled
	ScriptPath  string      `json:"script_path,omitempty"` // Path to external script handler
	Async       bool        `json:"async,omitempty"`       // Whether to run asynchronously
}

// Context represents the context passed to hook handlers.
type Context struct {
	Type      HookType  `json:"type"`
	Timestamp time.Time `json:"timestamp"`

	// Optional context (populated based on HookType)
	Session  *SessionContext  `json:"session,omitempty"`
	Message  *MessageContext  `json:"message,omitempty"`
	ToolCall *ToolCallContext `json:"tool_call,omitempty"`
	Response *ResponseContext `json:"response,omitempty"`
	Memory   *MemoryContext   `json:"memory,omitempty"`
	Prompt   *PromptContext   `json:"prompt,omitempty"`
	Error    *ErrorContext    `json:"error,omitempty"`

	// Custom data passing between handlers
	Data map[string]any `json:"data,omitempty"`
}

// SessionContext contains session-related context.
type SessionContext struct {
	ID        string         `json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// MessageContext contains message-related context.
type MessageContext struct {
	Content  string         `json:"content"`
	Role     string         `json:"role"`
	From     string         `json:"from,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ToolCallContext contains tool call-related context.
type ToolCallContext struct {
	ID       string         `json:"id"`
	ToolName string         `json:"tool_name"`
	Params   map[string]any `json:"params"`
	Result   any            `json:"result,omitempty"`
	Error    string         `json:"error,omitempty"`
	Duration time.Duration  `json:"duration,omitempty"`
}

// ResponseContext contains response-related context.
type ResponseContext struct {
	Content    string `json:"content"`
	TokensUsed int    `json:"tokens_used,omitempty"`
	Model      string `json:"model,omitempty"`
}

// MemoryContext contains memory operation-related context.
type MemoryContext struct {
	Key       string         `json:"key"`                 // Memory key
	Content   string         `json:"content"`             // Memory content
	Metadata  map[string]any `json:"metadata,omitempty"`  // Additional metadata
	Operation string         `json:"operation,omitempty"` // "write" | "delete" | "update"
}

// PromptContext contains prompt building-related context.
type PromptContext struct {
	SystemPrompt string   `json:"system_prompt"`         // Base system prompt
	UserPrompt   string   `json:"user_prompt,omitempty"` // User prompt if available
	Injections   []string `json:"injections,omitempty"`  // Injected prompt segments
}

// ErrorContext contains error-related context.
type ErrorContext struct {
	Code    string `json:"code"`             // Error code
	Message string `json:"message"`          // Error message
	Source  string `json:"source,omitempty"` // Error source (tool name, handler ID, etc.)
}

// Result represents the result returned by a hook handler.
type Result struct {
	Continue bool           `json:"continue"`       // Whether to continue executing subsequent handlers
	Modified bool           `json:"modified"`       // Whether the context was modified
	Data     map[string]any `json:"data,omitempty"` // Modified data
	Error    error          `json:"-"`              // Error information (not serialized)
}

// ContinueResult creates a result that allows the chain to continue.
func ContinueResult() *Result {
	return &Result{
		Continue: true,
		Modified: false,
	}
}

// StopResult creates a result that stops the chain execution.
func StopResult() *Result {
	return &Result{
		Continue: false,
		Modified: false,
	}
}

// ModifiedResult creates a result with modified data that allows continuation.
func ModifiedResult(data map[string]any) *Result {
	return &Result{
		Continue: true,
		Modified: true,
		Data:     data,
	}
}

// ErrorResult creates a result with an error that stops the chain.
func ErrorResult(err error) *Result {
	return &Result{
		Continue: false,
		Modified: false,
		Error:    err,
	}
}

// NewContext creates a new hook context with the given type.
func NewContext(hookType HookType) *Context {
	return &Context{
		Type:      hookType,
		Timestamp: time.Now(),
		Data:      make(map[string]any),
	}
}

// WithSession adds session context to the hook context.
func (c *Context) WithSession(session *SessionContext) *Context {
	c.Session = session
	return c
}

// WithMessage adds message context to the hook context.
func (c *Context) WithMessage(message *MessageContext) *Context {
	c.Message = message
	return c
}

// WithToolCall adds tool call context to the hook context.
func (c *Context) WithToolCall(toolCall *ToolCallContext) *Context {
	c.ToolCall = toolCall
	return c
}

// WithResponse adds response context to the hook context.
func (c *Context) WithResponse(response *ResponseContext) *Context {
	c.Response = response
	return c
}

// WithMemory adds memory context to the hook context.
func (c *Context) WithMemory(memory *MemoryContext) *Context {
	c.Memory = memory
	return c
}

// WithPrompt adds prompt context to the hook context.
func (c *Context) WithPrompt(prompt *PromptContext) *Context {
	c.Prompt = prompt
	return c
}

// WithError adds error context to the hook context.
func (c *Context) WithError(errCtx *ErrorContext) *Context {
	c.Error = errCtx
	return c
}

// SetData sets a custom data value in the context.
func (c *Context) SetData(key string, value any) {
	if c.Data == nil {
		c.Data = make(map[string]any)
	}
	c.Data[key] = value
}

// GetData retrieves a custom data value from the context.
func (c *Context) GetData(key string) (any, bool) {
	if c.Data == nil {
		return nil, false
	}
	v, ok := c.Data[key]
	return v, ok
}
