// Package vllm implements the Provider interface for vLLM.
// vLLM provides an OpenAI-compatible chat completions API.
// API docs: https://docs.vllm.ai/en/latest/serving/openai_api.html
package vllm

import (
	"encoding/json"
	"time"
)

// Default configuration values.
const (
	DefaultEndpoint  = "http://localhost:8000"
	DefaultModel     = "" // vLLM serves a single model; auto-detect via /v1/models
	DefaultMaxTokens = 4096
	DefaultTimeout   = 5 * time.Minute
)

// Config holds vLLM provider configuration.
type Config struct {
	APIKey    string        `mapstructure:"api_key"`    // Optional API key (if vLLM started with --api-key)
	Endpoint  string        `mapstructure:"endpoint"`   // vLLM server URL (default: http://localhost:8000)
	Model     string        `mapstructure:"model"`      // Model name (auto-detected if empty)
	MaxTokens int           `mapstructure:"max_tokens"` // Max output tokens
	Timeout   time.Duration `mapstructure:"timeout"`    // Request timeout
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Endpoint:  DefaultEndpoint,
		MaxTokens: DefaultMaxTokens,
		Timeout:   DefaultTimeout,
	}
}

// --- OpenAI-compatible request/response types ---

// chatRequest represents an OpenAI-compatible chat completion request.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []chatTool    `json:"tools,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// chatMessage represents a message in OpenAI format.
type chatMessage struct {
	Role       string         `json:"role"`
	Content    *string        `json:"content"` // Pointer to allow explicit null
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// chatTool represents a tool definition in OpenAI format.
type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

// chatFunction represents a function tool definition.
type chatFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// chatToolCall represents a tool call in OpenAI format.
type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// chatResponse represents an OpenAI-compatible chat completion response.
type chatResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []chatChoice   `json:"choices"`
	Usage   *chatUsage     `json:"usage,omitempty"`
	Error   *chatErrorInfo `json:"error,omitempty"`
}

// chatChoice represents a choice in an OpenAI-compatible response.
type chatChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// chatUsage represents token usage in an OpenAI-compatible response.
type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// chatErrorInfo represents an error in the response.
type chatErrorInfo struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

// chatStreamChunk represents a streaming response chunk (SSE).
type chatStreamChunk struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []chatStreamChoice `json:"choices"`
	Usage   *chatUsage         `json:"usage,omitempty"`
	Error   *chatErrorInfo     `json:"error,omitempty"`
}

// chatStreamChoice represents a streaming choice.
type chatStreamChoice struct {
	Index        int             `json:"index"`
	Delta        chatStreamDelta `json:"delta"`
	FinishReason string          `json:"finish_reason,omitempty"`
}

// chatStreamDelta represents the delta content in a streaming chunk.
type chatStreamDelta struct {
	Role      string         `json:"role,omitempty"`
	Content   string         `json:"content,omitempty"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
}

// modelsResponse represents the response from /v1/models.
type modelsResponse struct {
	Object string      `json:"object"`
	Data   []modelInfo `json:"data"`
}

// modelInfo represents a model entry from the API.
type modelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}
