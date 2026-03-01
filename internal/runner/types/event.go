// Package types 定义 runner 和 orchestrator 之间共享的类型
package types

import (
	"mote/internal/provider"
	"mote/internal/storage"
)

// EventType represents the type of event emitted during execution.
type EventType int

const (
	// EventTypeContent indicates content being streamed from the model.
	EventTypeContent EventType = iota
	// EventTypeToolCall indicates the model wants to call a tool.
	EventTypeToolCall
	// EventTypeToolResult indicates a tool execution result.
	EventTypeToolResult
	// EventTypeDone indicates the run completed successfully.
	EventTypeDone
	// EventTypeError indicates an error occurred.
	EventTypeError
	// EventTypeHeartbeat indicates a keepalive heartbeat during long operations.
	EventTypeHeartbeat
	// EventTypeTruncated indicates the response was truncated due to max_tokens limit.
	EventTypeTruncated
	// EventTypeThinking indicates agent thinking/reasoning (temporary display).
	EventTypeThinking
	// EventTypeToolCallUpdate indicates tool call progress update.
	EventTypeToolCallUpdate
	// EventTypePause indicates execution has been paused before tool execution.
	EventTypePause
	// EventTypePauseTimeout indicates a pause has timed out.
	EventTypePauseTimeout
	// EventTypePauseResumed indicates execution has resumed after a pause.
	EventTypePauseResumed
	// EventTypeApprovalRequest indicates a tool call requires user approval.
	EventTypeApprovalRequest
	// EventTypeApprovalResolved indicates an approval request has been resolved.
	EventTypeApprovalResolved
	// EventTypePDAProgress indicates PDA step execution progress.
	EventTypePDAProgress
)

// Usage represents token usage information.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ToolCallUpdateEvent represents a tool call progress update.
type ToolCallUpdateEvent struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Status     string `json:"status"` // e.g., "started", "running", "completed"
	Arguments  string `json:"arguments,omitempty"`
}

// ToolResultEvent represents the result of a tool execution.
type ToolResultEvent struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Output     string `json:"output"`
	IsError    bool   `json:"is_error,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// PDAProgressEvent represents PDA step execution progress.
type PDAProgressEvent struct {
	AgentName     string           `json:"agent_name"`
	StepIndex     int              `json:"step_index"`
	TotalSteps    int              `json:"total_steps"`
	StepLabel     string           `json:"step_label"`
	StepType      string           `json:"step_type"`
	Phase         string           `json:"phase"` // "started", "completed", "failed"
	StackDepth    int              `json:"stack_depth"`
	ExecutedSteps []string         `json:"executed_steps,omitempty"`
	TotalTokens   int              `json:"total_tokens,omitempty"`
	Model         string           `json:"model,omitempty"`
	ParentSteps   []ParentStepInfo `json:"parent_steps,omitempty"`
}

// ParentStepInfo describes a parent frame's step progress in the PDA call stack.
type ParentStepInfo struct {
	AgentName  string `json:"agent_name"`
	StepIndex  int    `json:"step_index"`
	TotalSteps int    `json:"total_steps"`
	StepLabel  string `json:"step_label"`
}

// PauseEventData contains pause-specific information.
type PauseEventData struct {
	SessionID    string     `json:"session_id"`
	PendingTools []ToolInfo `json:"pending_tools,omitempty"`
	HasUserInput bool       `json:"has_user_input,omitempty"`
}

// ToolInfo contains information about a tool.
type ToolInfo struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// Event represents an event emitted during agent execution.
type Event struct {
	Type             EventType            `json:"type"`
	Content          string               `json:"content,omitempty"`
	Thinking         string               `json:"thinking,omitempty"`
	ToolCall         *storage.ToolCall    `json:"tool_call,omitempty"`
	ToolCallUpdate   *ToolCallUpdateEvent `json:"tool_call_update,omitempty"`
	ToolResult       *ToolResultEvent     `json:"tool_result,omitempty"`
	Usage            *Usage               `json:"usage,omitempty"`
	Error            error                `json:"-"`
	ErrorMsg         string               `json:"error,omitempty"`
	Iteration        int                  `json:"iteration,omitempty"`
	SessionID        string               `json:"session_id,omitempty"`
	TruncatedReason  string               `json:"truncated_reason,omitempty"`
	PendingToolCalls int                  `json:"pending_tool_calls,omitempty"`
	PauseData        *PauseEventData      `json:"pause_data,omitempty"`
	PDAProgress      *PDAProgressEvent    `json:"pda_progress,omitempty"`

	// Multi-agent delegate identity
	AgentName  string `json:"agent_name,omitempty"`
	AgentDepth int    `json:"agent_depth,omitempty"`
}

// NewContentEvent creates a content event.
func NewContentEvent(content string) Event {
	return Event{
		Type:    EventTypeContent,
		Content: content,
	}
}

// NewErrorEvent creates an error event.
func NewErrorEvent(err error) Event {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return Event{
		Type:     EventTypeError,
		Error:    err,
		ErrorMsg: msg,
	}
}

// NewToolCallEvent creates a tool call event.
func NewToolCallEvent(toolCall *storage.ToolCall) Event {
	return Event{
		Type:     EventTypeToolCall,
		ToolCall: toolCall,
	}
}

// NewToolResultEvent creates a tool result event.
func NewToolResultEvent(callID, toolName, output string, isError bool, durationMs int64) Event {
	return Event{
		Type: EventTypeToolResult,
		ToolResult: &ToolResultEvent{
			ToolCallID: callID,
			ToolName:   toolName,
			Output:     output,
			IsError:    isError,
			DurationMs: durationMs,
		},
	}
}

// NewDoneEvent creates a done event with optional usage.
func NewDoneEvent(usage *Usage) Event {
	return Event{
		Type:  EventTypeDone,
		Usage: usage,
	}
}

// ToProviderUsage converts types.Usage to provider.Usage
func (u *Usage) ToProviderUsage() *provider.Usage {
	if u == nil {
		return nil
	}
	return &provider.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
}

// FromProviderUsage converts provider.Usage to types.Usage
func FromProviderUsage(u *provider.Usage) *Usage {
	if u == nil {
		return nil
	}
	return &Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
}
