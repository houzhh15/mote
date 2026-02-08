package runner

import "mote/internal/storage"

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

// Event represents an event emitted during agent execution.
type Event struct {
	// Type indicates the kind of event.
	Type EventType `json:"type"`

	// Content contains the text content for content events.
	Content string `json:"content,omitempty"`

	// ToolCall contains the tool call information for tool_call events.
	ToolCall *storage.ToolCall `json:"tool_call,omitempty"`

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

// NewHeartbeatEvent creates a new heartbeat event to keep connection alive.
func NewHeartbeatEvent() Event {
	return Event{
		Type: EventTypeHeartbeat,
	}
}
