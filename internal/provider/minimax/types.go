package minimax

import "time"

// Default configuration values.
const (
	DefaultEndpoint  = "https://api.minimaxi.com/v1"
	DefaultModel     = "MiniMax-M2.5"
	DefaultTimeout   = 5 * time.Minute
	DefaultMaxTokens = 16384
)

// Config holds MiniMax provider configuration.
type Config struct {
	APIKey    string        `mapstructure:"api_key"`
	Endpoint  string        `mapstructure:"endpoint"`
	Model     string        `mapstructure:"model"`
	MaxTokens int           `mapstructure:"max_tokens"`
	Timeout   time.Duration `mapstructure:"timeout"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Endpoint:  DefaultEndpoint,
		Model:     DefaultModel,
		MaxTokens: DefaultMaxTokens,
		Timeout:   DefaultTimeout,
	}
}

// AvailableModels lists the models available on the MiniMax platform.
var AvailableModels = []string{
	"MiniMax-M2.5",
	"MiniMax-M2.5-highspeed",
	"MiniMax-M2.1",
	"MiniMax-M2.1-highspeed",
	"MiniMax-M2",
}

// ModelInfo holds metadata for a MiniMax model.
type ModelInfo struct {
	DisplayName       string
	ContextWindow     int
	MaxOutput         int
	SupportsTools     bool
	SupportsReasoning bool // Whether the model supports reasoning_split parameter
	Description       string
}

// ModelMetadata maps model IDs to their metadata.
var ModelMetadata = map[string]ModelInfo{
	"MiniMax-M2.5": {
		DisplayName:       "MiniMax M2.5",
		ContextWindow:     204800,
		MaxOutput:         16384,
		SupportsTools:     true,
		SupportsReasoning: true,
		Description:       "顶尖性能与极致性价比，轻松驾驭复杂任务 (~60 tps)",
	},
	"MiniMax-M2.5-highspeed": {
		DisplayName:       "MiniMax M2.5 Highspeed",
		ContextWindow:     204800,
		MaxOutput:         16384,
		SupportsTools:     true,
		SupportsReasoning: false, // Highspeed models do not support reasoning_split
		Description:       "M2.5 极速版：效果不变，更快更敏捷 (~100 tps)",
	},
	"MiniMax-M2.1": {
		DisplayName:   "MiniMax M2.1",
		ContextWindow: 204800,
		MaxOutput:     16384,
		SupportsTools: true,
		Description:   "强大多语言编程能力，全面升级编程体验 (~60 tps)",
	},
	"MiniMax-M2.1-highspeed": {
		DisplayName:   "MiniMax M2.1 Highspeed",
		ContextWindow: 204800,
		MaxOutput:     16384,
		SupportsTools: true,
		Description:   "M2.1 极速版：更快更敏捷 (~100 tps)",
	},
	"MiniMax-M2": {
		DisplayName:   "MiniMax M2",
		ContextWindow: 204800,
		MaxOutput:     16384,
		SupportsTools: true,
		Description:   "专为高效编码与 Agent 工作流而生",
	},
}

// --- OpenAI-compatible request/response types ---

// chatRequest represents an OpenAI-compatible chat completion request.
type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Stream         bool          `json:"stream,omitempty"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	Temperature    *float64      `json:"temperature,omitempty"`
	Tools          []chatTool    `json:"tools,omitempty"`
	ReasoningSplit bool          `json:"reasoning_split,omitempty"` // Separate thinking into reasoning_details field
}

// chatMessage represents a message in OpenAI format.
type chatMessage struct {
	Role       string         `json:"role"`
	Content    *string        `json:"content"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

// strPtr returns a pointer to a string. Used for chatMessage.Content.
func strPtr(s string) *string {
	return &s
}

// chatTool represents a tool definition in OpenAI format.
type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

// chatFunction represents a function tool definition.
type chatFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

// chatToolCall represents a tool call in OpenAI format.
type chatToolCall struct {
	Index    int    `json:"index"`
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
	Error   *chatRespError `json:"error,omitempty"`
}

// chatChoice represents a choice in the response.
type chatChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// chatUsage represents token usage information.
type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// chatRespError represents an error response from the API.
type chatRespError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// --- Streaming types (SSE) ---

// chatStreamChunk represents a streaming chunk in OpenAI SSE format.
type chatStreamChunk struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []chatStreamChoice `json:"choices"`
	Usage   *chatUsage         `json:"usage,omitempty"`
	Error   *chatRespError     `json:"error,omitempty"`
}

// chatStreamChoice represents a streaming choice.
type chatStreamChoice struct {
	Index        int             `json:"index"`
	Delta        chatStreamDelta `json:"delta"`
	FinishReason string          `json:"finish_reason,omitempty"`
}

// chatStreamDelta represents the delta content in a streaming chunk.
type chatStreamDelta struct {
	Role             string            `json:"role,omitempty"`
	Content          string            `json:"content,omitempty"`
	ReasoningContent string            `json:"reasoning_content,omitempty"` // Thinking content when reasoning_split=True
	ReasoningDetails []reasoningDetail `json:"reasoning_details,omitempty"` // Structured reasoning details
	ToolCalls        []chatToolCall    `json:"tool_calls,omitempty"`
}

// reasoningDetail represents a single reasoning detail entry.
type reasoningDetail struct {
	Text string `json:"text,omitempty"`
}
