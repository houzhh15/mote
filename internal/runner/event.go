package runner

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
	// User can choose to continue the conversation.
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
)

// String returns the string representation of the event type.
func (t EventType) String() string {
	switch t {
	case EventTypeContent:
		return "content"
	case EventTypeToolCall:
		return "tool_call"
	case EventTypeToolResult:
		return "tool_result"
	case EventTypeDone:
		return "done"
	case EventTypeError:
		return "error"
	case EventTypeHeartbeat:
		return "heartbeat"
	case EventTypeTruncated:
		return "truncated"
	case EventTypeThinking:
		return "thinking"
	case EventTypeToolCallUpdate:
		return "tool_call_update"
	case EventTypePause:
		return "pause"
	case EventTypePauseTimeout:
		return "pause_timeout"
	case EventTypePauseResumed:
		return "pause_resumed"
	default:
		return "unknown"
	}
}

// Usage represents token usage information.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// PauseEventData contains pause-specific information.
type PauseEventData struct {
	// SessionID is the ID of the paused session.
	SessionID string `json:"session_id"`

	// PendingTools is a list of tool calls that were about to be executed.
	PendingTools []ToolInfo `json:"pending_tools,omitempty"`

	// HasUserInput indicates if user provided input during pause resume.
	HasUserInput bool `json:"has_user_input,omitempty"`
}

// Event represents an event emitted during agent execution.
type Event struct {
	// Type indicates the kind of event.
	Type EventType `json:"type"`

	// Content contains the text content for content events.
	Content string `json:"content,omitempty"`

	// Thinking contains the thinking/reasoning text for thinking events.
	// This is temporary display content that should be cleared when other output arrives.
	Thinking string `json:"thinking,omitempty"`

	// ToolCall contains the tool call information for tool_call events.
	ToolCall *storage.ToolCall `json:"tool_call,omitempty"`

	// ToolCallUpdate contains tool call progress information for tool_call_update events.
	ToolCallUpdate *ToolCallUpdateEvent `json:"tool_call_update,omitempty"`

	// ToolResult contains the tool execution result for tool_result events.
	ToolResult *ToolResultEvent `json:"tool_result,omitempty"`

	// Usage contains token usage information, typically on done events.
	Usage *Usage `json:"usage,omitempty"`

	// Error contains the error for error events.
	Error error `json:"-"`

	// ErrorMsg contains the error message for serialization.
	ErrorMsg string `json:"error,omitempty"`

	// Iteration indicates the current iteration number.
	Iteration int `json:"iteration,omitempty"`

	// SessionID is the ID of the session this event belongs to.
	SessionID string `json:"session_id,omitempty"`

	// TruncatedReason contains the reason for truncation (e.g., "length" for max_tokens limit).
	// Only set for EventTypeTruncated events.
	TruncatedReason string `json:"truncated_reason,omitempty"`

	// PendingToolCalls indicates the number of pending tool calls when truncated.
	PendingToolCalls int `json:"pending_tool_calls,omitempty"`

	// PauseData contains pause-specific information for pause events.
	PauseData *PauseEventData `json:"pause_data,omitempty"`
}

// ToolResultEvent represents the result of a tool execution.
type ToolResultEvent struct {
	// ToolCallID is the ID of the tool call this result is for.
	ToolCallID string `json:"tool_call_id"`

	// ToolName is the name of the tool that was executed.
	ToolName string `json:"tool_name"`

	// Output is the result output from the tool.
	Output string `json:"output"`

	// IsError indicates if the tool execution resulted in an error.
	IsError bool `json:"is_error,omitempty"`

	// DurationMs is the time taken to execute the tool in milliseconds.
	DurationMs int64 `json:"duration_ms,omitempty"`
}

// ToolCallUpdateEvent represents a tool call progress update.
type ToolCallUpdateEvent struct {
	// ToolCallID is the ID of the tool call being updated.
	ToolCallID string `json:"tool_call_id"`

	// ToolName is the name of the tool being called.
	ToolName string `json:"tool_name"`

	// Status is the current status of the tool call (e.g., "running", "completed").
	Status string `json:"status,omitempty"`

	// Arguments contains the tool call arguments (may be partial during streaming).
	Arguments string `json:"arguments,omitempty"`
}

// NewContentEvent creates a new content event.
func NewContentEvent(content string) Event {
	return Event{
		Type:    EventTypeContent,
		Content: content,
	}
}

// NewToolCallEvent creates a new tool call event.
func NewToolCallEvent(tc *storage.ToolCall) Event {
	return Event{
		Type:     EventTypeToolCall,
		ToolCall: tc,
	}
}

// NewToolResultEvent creates a new tool result event.
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

// NewDoneEvent creates a new done event with optional usage info.
func NewDoneEvent(usage *Usage) Event {
	return Event{
		Type:  EventTypeDone,
		Usage: usage,
	}
}

// NewErrorEvent creates a new error event.
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

// NewTruncatedEvent creates a new truncated event when response is cut off due to token limits.
func NewTruncatedEvent(reason string, pendingToolCalls int, usage *Usage) Event {
	return Event{
		Type:             EventTypeTruncated,
		TruncatedReason:  reason,
		PendingToolCalls: pendingToolCalls,
		Usage:            usage,
	}
}

// NewHeartbeatEvent creates a new heartbeat event to keep connection alive.
func NewHeartbeatEvent() Event {
	return Event{
		Type: EventTypeHeartbeat,
	}
}

// NewPauseEvent creates a new pause event when execution is paused before tool execution.
func NewPauseEvent(toolCalls []provider.ToolCall) Event {
	tools := make([]ToolInfo, len(toolCalls))
	for i, tc := range toolCalls {
		// Arguments is string in provider.ToolCall, store as string in map
		var args map[string]any
		if tc.Arguments != "" {
			args = map[string]any{"raw": tc.Arguments}
		}
		tools[i] = ToolInfo{
			ID:        tc.ID,
			Name:      tc.Name,
			Arguments: args,
		}
	}

	return Event{
		Type: EventTypePause,
		PauseData: &PauseEventData{
			PendingTools: tools,
		},
	}
}

// NewPauseTimeoutEvent creates a new pause timeout event.
func NewPauseTimeoutEvent(sessionID string) Event {
	return Event{
		Type: EventTypePauseTimeout,
		PauseData: &PauseEventData{
			SessionID: sessionID,
		},
	}
}

// NewPauseResumedEvent creates a new pause resumed event.
func NewPauseResumedEvent(sessionID string, hasUserInput bool) Event {
	return Event{
		Type: EventTypePauseResumed,
		PauseData: &PauseEventData{
			SessionID:    sessionID,
			HasUserInput: hasUserInput,
		},
	}
}
