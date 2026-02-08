// Package tools defines the Tool interface and related types for the Mote agent runtime.
package tools

import (
	"context"
	"encoding/json"
)

// Context keys for passing execution context to tools.
type contextKey string

const (
	sessionIDKey contextKey = "session_id"
	agentIDKey   contextKey = "agent_id"
)

// WithSessionID returns a new context with the session ID attached.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey, sessionID)
}

// SessionIDFromContext retrieves the session ID from the context, if present.
func SessionIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(sessionIDKey).(string)
	return id, ok
}

// WithAgentID returns a new context with the agent ID attached.
func WithAgentID(ctx context.Context, agentID string) context.Context {
	return context.WithValue(ctx, agentIDKey, agentID)
}

// AgentIDFromContext retrieves the agent ID from the context, if present.
func AgentIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(agentIDKey).(string)
	return id, ok
}

// Tool defines the interface that all tools must implement.
// A tool is a capability that an AI agent can invoke to interact with external systems.
type Tool interface {
	// Name returns the unique identifier for this tool.
	Name() string

	// Description returns a human-readable description of what this tool does.
	Description() string

	// Parameters returns the JSON Schema for the tool's input parameters.
	// The schema follows the JSON Schema Draft-07 specification.
	Parameters() map[string]any

	// Execute runs the tool with the given arguments and returns a result.
	// The context can be used for timeout/cancellation control.
	Execute(ctx context.Context, args map[string]any) (ToolResult, error)
}

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	// Content is the main output of the tool, typically text.
	Content string `json:"content"`

	// IsError indicates whether this result represents an error condition.
	IsError bool `json:"is_error"`

	// Metadata contains optional additional information about the execution.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewSuccessResult creates a successful tool result with the given content.
func NewSuccessResult(content string) ToolResult {
	return ToolResult{
		Content: content,
		IsError: false,
	}
}

// NewErrorResult creates an error tool result with the given error message.
func NewErrorResult(errMsg string) ToolResult {
	return ToolResult{
		Content: errMsg,
		IsError: true,
	}
}

// NewResultWithMetadata creates a successful tool result with content and metadata.
func NewResultWithMetadata(content string, metadata map[string]any) ToolResult {
	return ToolResult{
		Content:  content,
		IsError:  false,
		Metadata: metadata,
	}
}

// MarshalJSON implements custom JSON marshaling for ToolResult.
func (r ToolResult) MarshalJSON() ([]byte, error) {
	type resultAlias ToolResult
	return json.Marshal(resultAlias(r))
}

// String returns a string representation of the ToolResult.
func (r ToolResult) String() string {
	if r.IsError {
		return "[error] " + r.Content
	}
	return r.Content
}

// BaseTool provides a convenient base implementation for tools.
// Embed this struct and override Execute to create simple tools.
type BaseTool struct {
	ToolName        string
	ToolDescription string
	ToolParameters  map[string]any
}

// Name returns the tool name.
func (t *BaseTool) Name() string {
	return t.ToolName
}

// Description returns the tool description.
func (t *BaseTool) Description() string {
	return t.ToolDescription
}

// Parameters returns the tool parameters schema.
func (t *BaseTool) Parameters() map[string]any {
	if t.ToolParameters == nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	return t.ToolParameters
}
