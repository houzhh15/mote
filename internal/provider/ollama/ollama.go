// Package ollama implements the Provider interface for Ollama.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"mote/internal/provider"
	"mote/pkg/logger"
)

// Error definitions.
var (
	ErrConnectionFailed = errors.New("failed to connect to Ollama server")
	ErrModelNotFound    = errors.New("model not found")
	ErrInvalidResponse  = errors.New("invalid response from Ollama")
	ErrRequestTimeout   = errors.New("request timeout")
)

// OllamaProvider implements the Provider interface for Ollama.
type OllamaProvider struct {
	endpoint   string
	model      string
	httpClient *http.Client
	keepAlive  string

	// Cached model list
	modelsCache []string
	modelsMu    sync.RWMutex
	modelsTime  time.Time
}

// NewOllamaProvider creates a new Ollama provider.
func NewOllamaProvider(cfg Config) provider.Provider {
	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultEndpoint
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.KeepAlive == "" {
		cfg.KeepAlive = DefaultKeepAlive
	}

	return &OllamaProvider{
		endpoint: cfg.Endpoint,
		model:    cfg.Model,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		keepAlive: cfg.KeepAlive,
	}
}

// Name returns the provider name.
func (p *OllamaProvider) Name() string {
	return "ollama"
}

// Models returns the list of available models.
func (p *OllamaProvider) Models() []string {
	p.modelsMu.RLock()
	// Return cached if less than 5 minutes old
	if time.Since(p.modelsTime) < 5*time.Minute && len(p.modelsCache) > 0 {
		models := p.modelsCache
		p.modelsMu.RUnlock()
		return models
	}
	p.modelsMu.RUnlock()

	// Fetch fresh model list
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	models, err := p.fetchModels(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to fetch Ollama models, returning cached")
		p.modelsMu.RLock()
		defer p.modelsMu.RUnlock()
		return p.modelsCache
	}

	p.modelsMu.Lock()
	p.modelsCache = models
	p.modelsTime = time.Now()
	p.modelsMu.Unlock()

	return models
}

// Chat sends a chat completion request and returns the response.
func (p *OllamaProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	// Build Ollama request
	ollamaReq := p.buildRequest(req, false)

	// Debug: log the model being used
	logger.Debug().Str("model", ollamaReq.Model).Str("req_model", req.Model).Msg("Ollama Chat request")

	// Send request
	resp, err := p.doRequest(ctx, "/api/chat", ollamaReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for error response
	if resp.StatusCode != http.StatusOK {
		logger.Error().Int("status", resp.StatusCode).Str("body", string(body)).Msg("Ollama error response")
		return nil, p.handleErrorResponse(resp.StatusCode, body)
	}

	// Parse response
	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		logger.Error().Err(err).Str("body", string(body)).Msg("Failed to parse Ollama response")
		return nil, ErrInvalidResponse
	}

	// Convert to provider response
	return p.convertResponse(&ollamaResp), nil
}

// Stream sends a streaming chat completion request.
func (p *OllamaProvider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.ChatEvent, error) {
	// Build Ollama request with streaming enabled
	ollamaReq := p.buildRequest(req, true)

	// Send request
	resp, err := p.doRequest(ctx, "/api/chat", ollamaReq)
	if err != nil {
		return nil, err
	}

	// Check for error response
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, p.handleErrorResponse(resp.StatusCode, body)
	}

	// Process stream
	return ProcessStream(resp.Body), nil
}

// buildRequest converts a provider.ChatRequest to an Ollama request.
// NOTE: Currently, most Ollama models don't support tools, so we filter out
// tool-related messages and don't send tools in the request.
func (p *OllamaProvider) buildRequest(req provider.ChatRequest, stream bool) *ollamaRequest {
	model := req.Model
	if model == "" {
		model = p.model
	}

	// Strip "ollama:" prefix if present (defensive - should be handled by runner)
	if len(model) > 7 && model[:7] == "ollama:" {
		model = model[7:]
	}

	// Enable tools if provided
	hasTools := len(req.Tools) > 0

	ollamaReq := &ollamaRequest{
		Model:     model,
		Messages:  make([]ollamaMessage, 0, len(req.Messages)),
		Stream:    stream,
		KeepAlive: p.keepAlive,
	}

	// Convert messages
	for _, msg := range req.Messages {
		// If no tools requested, skip tool messages and tool_calls
		// This handles the case when switching to a non-tool-supporting model
		// with existing tool history
		if !hasTools {
			// Skip tool role messages entirely
			if msg.Role == "tool" {
				continue
			}
			// For assistant messages with tool_calls but no content, skip
			if msg.Role == "assistant" && len(msg.ToolCalls) > 0 && msg.Content == "" {
				continue
			}
		}

		ollamaMsg := ollamaMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}

		// Only convert tool calls if tools are requested
		if hasTools {
			for _, tc := range msg.ToolCalls {
				ollamaTC := ollamaToolCall{
					ID:   tc.ID,
					Type: "function",
				}
				ollamaTC.Function.Name = tc.Name

				// Get arguments string
				var argsStr string
				if tc.Function != nil {
					ollamaTC.Function.Name = tc.Function.Name
					argsStr = tc.Function.Arguments
				} else {
					argsStr = tc.Arguments
				}

				// Parse arguments string to JSON object (Ollama expects object, not string)
				if argsStr != "" {
					var argsMap map[string]interface{}
					if err := json.Unmarshal([]byte(argsStr), &argsMap); err != nil {
						// If parsing fails, use empty object
						ollamaTC.Function.Arguments = make(map[string]interface{})
					} else {
						ollamaTC.Function.Arguments = argsMap
					}
				} else {
					ollamaTC.Function.Arguments = make(map[string]interface{})
				}

				ollamaMsg.ToolCalls = append(ollamaMsg.ToolCalls, ollamaTC)
			}
		}

		ollamaReq.Messages = append(ollamaReq.Messages, ollamaMsg)
	}

	// Convert tools if provided
	if hasTools {
		for _, tool := range req.Tools {
			ollamaReq.Tools = append(ollamaReq.Tools, ollamaTool{
				Type: tool.Type,
				Function: ollamaToolFunction{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters:  tool.Function.Parameters,
				},
			})
		}
	}

	// Set options if temperature or max_tokens specified
	if req.Temperature > 0 || req.MaxTokens > 0 {
		ollamaReq.Options = &ollamaOptions{
			Temperature: req.Temperature,
			NumPredict:  req.MaxTokens,
		}
	}

	return ollamaReq
}

// doRequest sends an HTTP request to the Ollama API.
func (p *OllamaProvider) doRequest(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	url := p.endpoint + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrRequestTimeout
		}
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}

	return resp, nil
}

// handleErrorResponse converts an error response to an appropriate error.
func (p *OllamaProvider) handleErrorResponse(statusCode int, body []byte) error {
	var errResp ollamaErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
		// Check for specific error messages
		if statusCode == http.StatusNotFound {
			return fmt.Errorf("%w: %s", ErrModelNotFound, errResp.Error)
		}
		return fmt.Errorf("ollama error: %s", errResp.Error)
	}

	switch statusCode {
	case http.StatusNotFound:
		return ErrModelNotFound
	case http.StatusServiceUnavailable:
		return ErrConnectionFailed
	default:
		return fmt.Errorf("ollama returned status %d: %s", statusCode, string(body))
	}
}

// convertResponse converts an Ollama response to a provider response.
func (p *OllamaProvider) convertResponse(resp *ollamaResponse) *provider.ChatResponse {
	result := &provider.ChatResponse{
		Content:      resp.Message.Content,
		FinishReason: provider.FinishReasonStop,
	}

	// Convert tool calls
	for _, tc := range resp.Message.ToolCalls {
		// Convert arguments map back to string for provider interface
		var argsStr string
		if tc.Function.Arguments != nil {
			if argsBytes, err := json.Marshal(tc.Function.Arguments); err == nil {
				argsStr = string(argsBytes)
			}
		}
		result.ToolCalls = append(result.ToolCalls, provider.ToolCall{
			ID:        tc.ID,
			Type:      "function",
			Name:      tc.Function.Name,
			Arguments: argsStr,
		})
	}

	if len(result.ToolCalls) > 0 {
		result.FinishReason = provider.FinishReasonToolCalls
	}

	// Convert usage (approximate from eval counts)
	if resp.PromptEvalCount > 0 || resp.EvalCount > 0 {
		result.Usage = &provider.Usage{
			PromptTokens:     resp.PromptEvalCount,
			CompletionTokens: resp.EvalCount,
			TotalTokens:      resp.PromptEvalCount + resp.EvalCount,
		}
	}

	return result
}

// fetchModels fetches the list of available models from Ollama.
func (p *OllamaProvider) fetchModels(ctx context.Context) ([]string, error) {
	url := p.endpoint + "/api/tags"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch models: status %d", resp.StatusCode)
	}

	var modelsResp ollamaModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, fmt.Errorf("failed to decode models response: %w", err)
	}

	models := make([]string, 0, len(modelsResp.Models))
	for _, m := range modelsResp.Models {
		models = append(models, m.Name)
	}

	return models, nil
}

// Ping checks if the Ollama server is available.
// Implements provider.HealthCheckable interface.
func (p *OllamaProvider) Ping(ctx context.Context) error {
	// Create a context with timeout for the health check
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	url := p.endpoint + "/api/tags"
	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, url, nil)
	if err != nil {
		return &provider.ProviderError{
			Code:      provider.ErrCodeNetworkError,
			Message:   fmt.Sprintf("创建请求失败: %v", err),
			Provider:  "ollama",
			Retryable: true,
		}
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   "Ollama 服务未运行或无法连接",
			Provider:  "ollama",
			Retryable: true,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   fmt.Sprintf("Ollama 服务返回异常状态码: %d", resp.StatusCode),
			Provider:  "ollama",
			Retryable: true,
		}
	}

	return nil
}

// GetState returns the current state of the Ollama provider.
// Implements provider.HealthCheckable interface.
func (p *OllamaProvider) GetState() provider.ProviderState {
	state := provider.ProviderState{
		Name:      "ollama",
		LastCheck: time.Now(),
	}

	// Perform a quick health check
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := p.Ping(ctx); err != nil {
		state.Status = provider.StatusUnavailable
		if pe, ok := err.(*provider.ProviderError); ok {
			state.LastError = pe.Message
		} else {
			state.LastError = err.Error()
		}
		return state
	}

	state.Status = provider.StatusConnected
	state.Models = p.Models()
	return state
}

// classifyError converts a generic error to a ProviderError with appropriate code.
func (p *OllamaProvider) classifyError(err error) *provider.ProviderError {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, ErrConnectionFailed):
		return &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   "无法连接到 Ollama 服务，请确保 Ollama 已启动",
			Provider:  "ollama",
			Retryable: true,
		}
	case errors.Is(err, ErrModelNotFound):
		return &provider.ProviderError{
			Code:      provider.ErrCodeModelNotFound,
			Message:   "模型不存在，请使用 ollama pull 命令下载",
			Provider:  "ollama",
			Retryable: false,
		}
	case errors.Is(err, ErrRequestTimeout):
		return &provider.ProviderError{
			Code:      provider.ErrCodeTimeout,
			Message:   "请求超时",
			Provider:  "ollama",
			Retryable: true,
		}
	case errors.Is(err, ErrInvalidResponse):
		return &provider.ProviderError{
			Code:      provider.ErrCodeInvalidRequest,
			Message:   "Ollama 返回了无效的响应",
			Provider:  "ollama",
			Retryable: false,
		}
	default:
		return &provider.ProviderError{
			Code:      provider.ErrCodeUnknown,
			Message:   err.Error(),
			Provider:  "ollama",
			Retryable: false,
		}
	}
}
