package provider

import "encoding/json"

// Message represents a chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall represents a tool/function call.
type ToolCall struct {
	ID        string `json:"id"`
	Index     int    `json:"index,omitempty"`
	Type      string `json:"type,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Function  *struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function,omitempty"`
}

// Tool represents a tool definition.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes a function tool.
type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ChatRequest represents a chat completion request.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []Tool    `json:"tools,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// ChatResponse represents a chat completion response.
type ChatResponse struct {
	Content      string     `json:"content,omitempty"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	Usage        *Usage     `json:"usage,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`
}

// Usage represents token usage statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatEvent represents a streaming chat event.
type ChatEvent struct {
	Type         string    `json:"type"` // content, tool_call, done, error
	Delta        string    `json:"delta,omitempty"`
	ToolCall     *ToolCall `json:"tool_call,omitempty"`
	Usage        *Usage    `json:"usage,omitempty"`
	FinishReason string    `json:"finish_reason,omitempty"` // stop, tool_calls, length
	Error        error     `json:"-"`
}

// Event types.
const (
	EventTypeContent  = "content"
	EventTypeToolCall = "tool_call"
	EventTypeDone     = "done"
	EventTypeError    = "error"
)

// Role constants.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// FinishReason constants.
const (
	FinishReasonStop      = "stop"
	FinishReasonToolCalls = "tool_calls"
	FinishReasonLength    = "length"
)
