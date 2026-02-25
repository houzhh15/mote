// Package glm implements the Provider interface for GLM (智谱AI).
// GLM provides an OpenAI-compatible chat completions API.
// API docs: https://docs.bigmodel.cn/cn/api/introduction
package glm

import (
	"encoding/json"
	"time"
)

// Default configuration values.
const (
	// CodingPlan 按时计量端点（非按量付费 /api/paas/v4）。
	// 参见 https://docs.bigmodel.cn/cn/coding-plan/faq
	DefaultEndpoint  = "https://open.bigmodel.cn/api/coding/paas/v4"
	DefaultModel     = "glm-z1-air"
	DefaultTimeout   = 5 * time.Minute
	DefaultMaxTokens = 16384
)

// Config holds GLM provider configuration.
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

// AvailableModels lists the models available on the GLM CodingPlan platform.
var AvailableModels = []string{
	// 旗舰模型
	"glm-5",
	"glm-4.7",
	"glm-4.7-flash",
	"glm-4.7-flashx",
	"glm-4.6",
	"glm-4.6v",
	"glm-4.5-air",
	"glm-4.5-airx",
	"glm-4.5-flash",
	// GLM-Z1 推理系列
	"glm-z1-air",
	"glm-z1-airx",
	"glm-z1-flash",
	"glm-z1-flashx",
	// 经济模型
	"glm-4-flash-250414",
	"glm-4-flashx-250414",
}

// ModelInfo holds metadata for a GLM model.
type ModelInfo struct {
	DisplayName        string
	ContextWindow      int
	MaxOutput          int
	SupportsTools      bool
	SupportsReasoning  bool // Whether the model supports thinking parameter
	SupportsToolStream bool // Whether the model supports tool_stream parameter
	Description        string
}

// ModelMetadata maps model IDs to their metadata.
var ModelMetadata = map[string]ModelInfo{
	"glm-5": {
		DisplayName:        "GLM-5",
		ContextWindow:      200000,
		MaxOutput:          131072,
		SupportsTools:      true,
		SupportsReasoning:  true,
		SupportsToolStream: true,
		Description:        "智谱旗舰模型，200K 上下文，128K 输出，支持深度推理与工具调用",
	},
	"glm-4.7": {
		DisplayName:        "GLM-4.7",
		ContextWindow:      128000,
		MaxOutput:          16384,
		SupportsTools:      true,
		SupportsReasoning:  true,
		SupportsToolStream: true,
		Description:        "高性能模型，128K 上下文，支持推理与工具调用",
	},
	"glm-4.7-flash": {
		DisplayName:        "GLM-4.7 Flash",
		ContextWindow:      128000,
		MaxOutput:          16384,
		SupportsTools:      true,
		SupportsReasoning:  true,
		SupportsToolStream: false,
		Description:        "高速模型，128K 上下文，极速响应",
	},
	"glm-4.7-flashx": {
		DisplayName:        "GLM-4.7 FlashX",
		ContextWindow:      128000,
		MaxOutput:          16384,
		SupportsTools:      true,
		SupportsReasoning:  true,
		SupportsToolStream: false,
		Description:        "极速版，128K 上下文，最低延迟",
	},
	"glm-4.6": {
		DisplayName:        "GLM-4.6",
		ContextWindow:      128000,
		MaxOutput:          16384,
		SupportsTools:      true,
		SupportsReasoning:  false,
		SupportsToolStream: true,
		Description:        "稳定高性能模型，128K 上下文",
	},
	"glm-4.5-air": {
		DisplayName:        "GLM-4.5 Air",
		ContextWindow:      128000,
		MaxOutput:          16384,
		SupportsTools:      true,
		SupportsReasoning:  true,
		SupportsToolStream: false,
		Description:        "轻量高效模型，支持推理",
	},
	"glm-4.5-airx": {
		DisplayName:        "GLM-4.5 AirX",
		ContextWindow:      128000,
		MaxOutput:          16384,
		SupportsTools:      true,
		SupportsReasoning:  true,
		SupportsToolStream: false,
		Description:        "轻量极速版，支持推理",
	},
	"glm-4.5-flash": {
		DisplayName:        "GLM-4.5 Flash",
		ContextWindow:      128000,
		MaxOutput:          16384,
		SupportsTools:      true,
		SupportsReasoning:  true,
		SupportsToolStream: false,
		Description:        "高速轻量模型，支持推理",
	},
	"glm-4-flash-250414": {
		DisplayName:        "GLM-4 Flash (250414)",
		ContextWindow:      128000,
		MaxOutput:          4096,
		SupportsTools:      true,
		SupportsReasoning:  false,
		SupportsToolStream: false,
		Description:        "经典高速模型，128K 上下文",
	},
	"glm-4-flashx-250414": {
		DisplayName:        "GLM-4 FlashX (250414)",
		ContextWindow:      128000,
		MaxOutput:          4096,
		SupportsTools:      true,
		SupportsReasoning:  false,
		SupportsToolStream: false,
		Description:        "经典极速版，128K 上下文",
	},
	"glm-4.6v": {
		DisplayName:        "GLM-4.6V",
		ContextWindow:      128000,
		MaxOutput:          16384,
		SupportsTools:      true,
		SupportsReasoning:  false,
		SupportsToolStream: true,
		Description:        "多模态视觉模型，128K 上下文，支持图像理解",
	},
	"glm-z1-air": {
		DisplayName:        "GLM-Z1 Air",
		ContextWindow:      128000,
		MaxOutput:          16384,
		SupportsTools:      true,
		SupportsReasoning:  true,
		SupportsToolStream: false,
		Description:        "高性价比推理模型，128K 上下文，适合日常编码",
	},
	"glm-z1-airx": {
		DisplayName:        "GLM-Z1 AirX",
		ContextWindow:      32000,
		MaxOutput:          16384,
		SupportsTools:      true,
		SupportsReasoning:  true,
		SupportsToolStream: false,
		Description:        "极速推理模型，32K 上下文，最低延迟",
	},
	"glm-z1-flash": {
		DisplayName:        "GLM-Z1 Flash",
		ContextWindow:      128000,
		MaxOutput:          16384,
		SupportsTools:      true,
		SupportsReasoning:  true,
		SupportsToolStream: false,
		Description:        "免费推理模型，128K 上下文",
	},
	"glm-z1-flashx": {
		DisplayName:        "GLM-Z1 FlashX",
		ContextWindow:      128000,
		MaxOutput:          16384,
		SupportsTools:      true,
		SupportsReasoning:  true,
		SupportsToolStream: false,
		Description:        "高速低价推理模型，128K 上下文",
	},
}

// --- OpenAI-compatible request/response types ---

// chatRequest represents an OpenAI-compatible chat completion request.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	Tools       []chatTool    `json:"tools,omitempty"`
	ToolChoice  interface{}   `json:"tool_choice,omitempty"` // "auto" | "none" | specific tool
	Thinking    *thinking     `json:"thinking,omitempty"`    // GLM thinking/reasoning mode
	ToolStream  bool          `json:"tool_stream,omitempty"` // Stream tool calls (GLM-5/4.7/4.6)
	DoSample    *bool         `json:"do_sample,omitempty"`   // Deterministic output when false
}

// thinking represents the GLM thinking/reasoning configuration.
type thinking struct {
	Type          string `json:"type"`                     // "enabled" or "disabled"
	ClearThinking bool   `json:"clear_thinking,omitempty"` // Whether to clear thinking in response
}

// chatMessage represents a message in OpenAI format.
// Supports both plain string content and multipart content (for vision).
type chatMessage struct {
	Role         string         `json:"-"`
	Content      *string        `json:"-"` // Normal string content
	ContentParts []contentPart  `json:"-"` // Multipart content (vision: text + images)
	ToolCalls    []chatToolCall `json:"-"`
	ToolCallID   string         `json:"-"`
}

// contentPart represents a part of multipart content (for vision/image support).
type contentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *visionImageURL `json:"image_url,omitempty"`
}

// visionImageURL represents an image URL in multipart content.
type visionImageURL struct {
	URL string `json:"url"`
}

// MarshalJSON implements custom JSON marshaling for chatMessage.
// When ContentParts is set (vision mode), content is serialized as an array.
// Otherwise, content is serialized as a string or null.
func (m chatMessage) MarshalJSON() ([]byte, error) {
	type alias struct {
		Role       string         `json:"role"`
		Content    interface{}    `json:"content"`
		ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
		ToolCallID string         `json:"tool_call_id,omitempty"`
	}
	a := alias{
		Role:       m.Role,
		ToolCalls:  m.ToolCalls,
		ToolCallID: m.ToolCallID,
	}
	if len(m.ContentParts) > 0 {
		a.Content = m.ContentParts
	} else {
		a.Content = m.Content // *string → null or "string"
	}
	return json.Marshal(a)
}

// UnmarshalJSON implements custom JSON unmarshaling for chatMessage.
// Content is always parsed as a string (API responses always return string content).
func (m *chatMessage) UnmarshalJSON(data []byte) error {
	type alias struct {
		Role       string         `json:"role"`
		Content    *string        `json:"content"`
		ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
		ToolCallID string         `json:"tool_call_id,omitempty"`
	}
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	m.Role = a.Role
	m.Content = a.Content
	m.ToolCalls = a.ToolCalls
	m.ToolCallID = a.ToolCallID
	return nil
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
	Role             string         `json:"role,omitempty"`
	Content          string         `json:"content,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"` // Thinking content when thinking is enabled
	ToolCalls        []chatToolCall `json:"tool_calls,omitempty"`
}
