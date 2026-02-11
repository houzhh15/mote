// Package ollama implements the Provider interface for Ollama.
package ollama

import "time"

// Default configuration values.
const (
	DefaultEndpoint  = "http://localhost:11434"
	DefaultModel     = "llama3.2"
	DefaultTimeout   = 5 * time.Minute
	DefaultKeepAlive = "5m"
)

// Config holds Ollama provider configuration.
type Config struct {
	Endpoint  string        `mapstructure:"endpoint"`
	Model     string        `mapstructure:"model"`
	Timeout   time.Duration `mapstructure:"timeout"`
	KeepAlive string        `mapstructure:"keep_alive"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Endpoint:  DefaultEndpoint,
		Model:     DefaultModel,
		Timeout:   DefaultTimeout,
		KeepAlive: DefaultKeepAlive,
	}
}

// ollamaRequest represents an Ollama chat request.
type ollamaRequest struct {
	Model     string          `json:"model"`
	Messages  []ollamaMessage `json:"messages"`
	Stream    bool            `json:"stream"`
	Tools     []ollamaTool    `json:"tools,omitempty"`
	Options   *ollamaOptions  `json:"options,omitempty"`
	KeepAlive string          `json:"keep_alive,omitempty"`
}

// ollamaMessage represents a message in Ollama format.
type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	Images    []string         `json:"images,omitempty"` // Base64 encoded images
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

// ollamaTool represents a tool definition in Ollama format.
type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

// ollamaToolFunction represents a function tool definition.
type ollamaToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

// ollamaToolCall represents a tool call in Ollama format.
type ollamaToolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"` // Ollama expects JSON object, not string
	} `json:"function"`
}

// ollamaOptions represents model options.
type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	TopK        int     `json:"top_k,omitempty"`
}

// ollamaResponse represents an Ollama chat response.
type ollamaResponse struct {
	Model     string        `json:"model"`
	CreatedAt string        `json:"created_at"`
	Message   ollamaMessage `json:"message"`
	Done      bool          `json:"done"`

	// Timing information (only present when done=true)
	TotalDuration      int64 `json:"total_duration,omitempty"`
	LoadDuration       int64 `json:"load_duration,omitempty"`
	PromptEvalCount    int   `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"`
	EvalCount          int   `json:"eval_count,omitempty"`
	EvalDuration       int64 `json:"eval_duration,omitempty"`
}

// ollamaModelsResponse represents the response from /api/tags.
type ollamaModelsResponse struct {
	Models []ollamaModelInfo `json:"models"`
}

// ollamaModelInfo represents information about a model.
type ollamaModelInfo struct {
	Name       string `json:"name"`
	ModifiedAt string `json:"modified_at"`
	Size       int64  `json:"size"`
	Digest     string `json:"digest"`
}

// ollamaErrorResponse represents an error response from Ollama.
type ollamaErrorResponse struct {
	Error string `json:"error"`
}
