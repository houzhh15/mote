// Package vllm implements the Provider interface for vLLM.
// vLLM provides an OpenAI-compatible chat completions API for locally
// hosted models with high-throughput serving.
// API docs: https://docs.vllm.ai/en/latest/serving/openai_api.html
package vllm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"mote/internal/provider"
	"mote/pkg/logger"
)

// Compile-time interface checks.
var (
	_ provider.Provider             = (*VLLMProvider)(nil)
	_ provider.HealthCheckable      = (*VLLMProvider)(nil)
	_ provider.ConnectionResettable = (*VLLMProvider)(nil)
)

// Error definitions.
var (
	ErrConnectionFailed = errors.New("failed to connect to vLLM server")
	ErrModelNotFound    = errors.New("model not found")
	ErrInvalidResponse  = errors.New("invalid response from vLLM")
	ErrRequestTimeout   = errors.New("request timeout")
)

// VLLMProvider implements the Provider interface for vLLM.
type VLLMProvider struct {
	apiKey       string
	endpoint     string
	model        string
	maxTokens    int
	httpClient   *http.Client // For non-streaming requests (has overall timeout)
	streamClient *http.Client // For streaming requests (no body read timeout)

	// Cached model list
	modelsCache []string
	modelsMu    sync.RWMutex
	modelsTime  time.Time
}

// NewVLLMProvider creates a new vLLM provider.
func NewVLLMProvider(cfg Config) provider.Provider {
	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultEndpoint
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}

	// Strip trailing /v1 or /v1/ to avoid path duplication (/v1/v1/chat/completions)
	normalized := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	normalized = strings.TrimSuffix(normalized, "/v1")

	return &VLLMProvider{
		apiKey:    cfg.APIKey,
		endpoint:  normalized,
		model:     cfg.Model,
		maxTokens: cfg.MaxTokens,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		// streamClient has NO overall timeout — http.Client.Timeout includes
		// response body read time, which kills long-running SSE streams.
		// Instead, use Transport-level timeouts for connection/TLS only.
		streamClient: &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: cfg.Timeout,
				IdleConnTimeout:       90 * time.Second,
			},
		},
	}
}

// Factory creates a ProviderFactory for vLLM.
func Factory(endpoint string, maxTokens int, apiKey string) provider.ProviderFactory {
	return func(model string) (provider.Provider, error) {
		cfg := Config{
			APIKey:    apiKey,
			Endpoint:  endpoint,
			Model:     model,
			MaxTokens: maxTokens,
			Timeout:   DefaultTimeout,
		}
		return NewVLLMProvider(cfg), nil
	}
}

// ResetConnections closes all idle HTTP connections, forcing fresh TCP
// connections on the next request.
func (p *VLLMProvider) ResetConnections() {
	p.httpClient.CloseIdleConnections()
	p.streamClient.CloseIdleConnections()
	logger.Info().Msg("vLLM connections reset")
}

// ListModels returns the list of available vLLM models by querying the server.
// Unlike cloud providers, vLLM typically serves one model at a time,
// so we discover models dynamically via /v1/models.
func ListModels(endpoint, apiKey string) []string {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	norm := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	norm = strings.TrimSuffix(norm, "/v1")
	url := norm + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to create vLLM models request")
		return nil
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to fetch vLLM models")
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Warn().Int("status", resp.StatusCode).Msg("vLLM models endpoint returned non-200")
		return nil
	}

	var modelsResp modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		logger.Warn().Err(err).Msg("Failed to decode vLLM models response")
		return nil
	}

	models := make([]string, 0, len(modelsResp.Data))
	for _, m := range modelsResp.Data {
		models = append(models, m.ID)
	}
	return models
}

// Name returns the provider name.
func (p *VLLMProvider) Name() string {
	return "vllm"
}

// Models returns the list of available models.
func (p *VLLMProvider) Models() []string {
	p.modelsMu.RLock()
	// Return cached if less than 5 minutes old
	if time.Since(p.modelsTime) < 5*time.Minute && len(p.modelsCache) > 0 {
		models := p.modelsCache
		p.modelsMu.RUnlock()
		return models
	}
	p.modelsMu.RUnlock()

	// Fetch fresh model list
	models := ListModels(p.endpoint, p.apiKey)
	if len(models) == 0 && p.model != "" {
		// Fallback to configured model
		models = []string{p.model}
	}

	if len(models) > 0 {
		p.modelsMu.Lock()
		p.modelsCache = models
		p.modelsTime = time.Now()
		p.modelsMu.Unlock()
	}

	return models
}

// Chat sends a chat completion request and returns the response.
func (p *VLLMProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	chatReq := p.buildRequest(req, false)

	logger.Debug().Str("model", chatReq.Model).Msg("vLLM Chat request")

	resp, err := p.doRequest(ctx, "/v1/chat/completions", chatReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.Error().Int("status", resp.StatusCode).Str("body", string(body)).Msg("vLLM error response")
		return nil, p.handleErrorResponse(resp.StatusCode, body)
	}

	if len(body) == 0 {
		logger.Warn().Int("status", resp.StatusCode).Msg("vLLM returned empty body")
		return nil, &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   "vLLM 返回了空响应",
			Provider:  "vllm",
			Retryable: true,
		}
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		logger.Error().Err(err).Str("body", string(body)).Msg("Failed to parse vLLM response")
		return nil, ErrInvalidResponse
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("vllm API error: [%s] %s", chatResp.Error.Type, chatResp.Error.Message)
	}

	return p.convertResponse(&chatResp), nil
}

// Stream sends a streaming chat completion request.
func (p *VLLMProvider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.ChatEvent, error) {
	chatReq := p.buildRequest(req, true)

	logger.Debug().Str("model", chatReq.Model).
		Int("message_count", len(chatReq.Messages)).
		Msg("vLLM Stream request")

	resp, err := p.doStreamRequest(ctx, "/v1/chat/completions", chatReq)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, p.handleErrorResponse(resp.StatusCode, body)
	}

	return ProcessStream(resp.Body), nil
}

// buildRequest converts a provider.ChatRequest to an OpenAI-compatible request.
func (p *VLLMProvider) buildRequest(req provider.ChatRequest, stream bool) *chatRequest {
	model := req.Model
	if model == "" {
		model = p.model
	}

	// Strip "vllm:" prefix if present
	if strings.HasPrefix(model, "vllm:") {
		model = strings.TrimPrefix(model, "vllm:")
	}

	// If model is still empty, try to auto-detect from /v1/models
	if model == "" {
		models := p.Models()
		if len(models) > 0 {
			model = models[0]
		}
	}

	hasTools := len(req.Tools) > 0

	chatReq := &chatRequest{
		Model:    model,
		Messages: make([]chatMessage, 0, len(req.Messages)),
		Stream:   stream,
	}

	// Set max tokens
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = p.maxTokens
	}
	if maxTokens > 0 {
		chatReq.MaxTokens = maxTokens
	}

	// Set temperature
	if req.Temperature > 0 {
		temp := req.Temperature
		chatReq.Temperature = &temp
	}

	// Convert messages
	for _, msg := range req.Messages {
		// Skip tool-related messages if no tools requested
		if !hasTools {
			if msg.Role == "tool" {
				continue
			}
			if msg.Role == "assistant" && len(msg.ToolCalls) > 0 && msg.Content == "" {
				continue
			}
		}

		content := msg.Content
		chatMsg := chatMessage{
			Role:       msg.Role,
			Content:    &content,
			ToolCallID: msg.ToolCallID,
		}

		// Convert tool calls
		if hasTools {
			for _, tc := range msg.ToolCalls {
				name := tc.Name
				args := tc.Arguments
				if tc.Function != nil {
					name = tc.Function.Name
					args = tc.Function.Arguments
				}
				chatMsg.ToolCalls = append(chatMsg.ToolCalls, chatToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{
						Name:      name,
						Arguments: args,
					},
				})
			}
		}

		chatReq.Messages = append(chatReq.Messages, chatMsg)
	}

	// Handle attachments - append text to last user message
	if len(req.Attachments) > 0 && len(chatReq.Messages) > 0 {
		for i := len(chatReq.Messages) - 1; i >= 0; i-- {
			if chatReq.Messages[i].Role == "user" {
				for _, att := range req.Attachments {
					if att.Type == "text" {
						contentText := fmt.Sprintf("\n\n--- File: %s ---\n```%s\n%s\n```",
							att.Filename,
							att.Metadata["language"],
							att.Text)
						if chatReq.Messages[i].Content != nil {
							newContent := *chatReq.Messages[i].Content + contentText
							chatReq.Messages[i].Content = &newContent
						} else {
							chatReq.Messages[i].Content = &contentText
						}
					}
				}
				break
			}
		}
	}

	// Convert tools
	if hasTools {
		for _, tool := range req.Tools {
			chatReq.Tools = append(chatReq.Tools, chatTool{
				Type: tool.Type,
				Function: chatFunction{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters:  tool.Function.Parameters,
				},
			})
		}
	}

	return chatReq
}

// doRequest sends an HTTP request to the vLLM API.
func (p *VLLMProvider) doRequest(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	url := p.endpoint + path

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrRequestTimeout
		}
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}

	return resp, nil
}

// doStreamRequest sends a streaming HTTP request to the vLLM API.
func (p *VLLMProvider) doStreamRequest(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	url := p.endpoint + path

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.streamClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrRequestTimeout
		}
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}

	return resp, nil
}

// handleErrorResponse converts an HTTP error response to an appropriate error.
func (p *VLLMProvider) handleErrorResponse(statusCode int, body []byte) error {
	var errResp chatResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil {
		lowerMsg := strings.ToLower(errResp.Error.Message)

		// Context window exceeded
		if strings.Contains(lowerMsg, "context length") ||
			strings.Contains(lowerMsg, "too many tokens") ||
			strings.Contains(lowerMsg, "maximum context") {
			return &provider.ProviderError{
				Code:      provider.ErrCodeContextWindowExceeded,
				Message:   errResp.Error.Message,
				Provider:  "vllm",
				Retryable: true,
			}
		}

		// Model not found
		if statusCode == http.StatusNotFound ||
			strings.Contains(lowerMsg, "model") && strings.Contains(lowerMsg, "not found") {
			return fmt.Errorf("%w: %s", ErrModelNotFound, errResp.Error.Message)
		}

		// Auth failed
		if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
			return &provider.ProviderError{
				Code:      provider.ErrCodeAuthFailed,
				Message:   errResp.Error.Message,
				Provider:  "vllm",
				Retryable: false,
			}
		}

		return fmt.Errorf("vllm error: [%s] %s", errResp.Error.Type, errResp.Error.Message)
	}

	switch statusCode {
	case http.StatusNotFound:
		return ErrModelNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return &provider.ProviderError{
			Code:      provider.ErrCodeAuthFailed,
			Message:   "vLLM 认证失败，请检查 API Key 配置",
			Provider:  "vllm",
			Retryable: false,
		}
	case http.StatusServiceUnavailable, http.StatusBadGateway:
		return &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   "vLLM 服务暂不可用",
			Provider:  "vllm",
			Retryable: true,
		}
	default:
		return fmt.Errorf("vllm returned status %d: %s", statusCode, string(body))
	}
}

// convertResponse converts an OpenAI-compatible response to a provider response.
func (p *VLLMProvider) convertResponse(resp *chatResponse) *provider.ChatResponse {
	result := &provider.ChatResponse{
		FinishReason: provider.FinishReasonStop,
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		if choice.Message.Content != nil {
			result.Content = *choice.Message.Content
		}

		// Convert tool calls
		for _, tc := range choice.Message.ToolCalls {
			result.ToolCalls = append(result.ToolCalls, provider.ToolCall{
				ID:        tc.ID,
				Type:      "function",
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}

		// Map finish reason
		switch choice.FinishReason {
		case "stop":
			result.FinishReason = provider.FinishReasonStop
		case "tool_calls":
			result.FinishReason = provider.FinishReasonToolCalls
		case "length":
			result.FinishReason = provider.FinishReasonLength
		}
	}

	if len(result.ToolCalls) > 0 {
		result.FinishReason = provider.FinishReasonToolCalls
	}

	// Convert usage
	if resp.Usage != nil {
		result.Usage = &provider.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	return result
}

// Ping checks if the vLLM server is available.
func (p *VLLMProvider) Ping(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	url := p.endpoint + "/v1/models"
	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, url, nil)
	if err != nil {
		return &provider.ProviderError{
			Code:      provider.ErrCodeNetworkError,
			Message:   fmt.Sprintf("创建请求失败: %v", err),
			Provider:  "vllm",
			Retryable: true,
		}
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   "vLLM 服务未运行或无法连接",
			Provider:  "vllm",
			Retryable: true,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   fmt.Sprintf("vLLM 服务返回异常状态码: %d", resp.StatusCode),
			Provider:  "vllm",
			Retryable: true,
		}
	}

	return nil
}

// GetState returns the current state of the vLLM provider.
func (p *VLLMProvider) GetState() provider.ProviderState {
	state := provider.ProviderState{
		Name:      "vllm",
		LastCheck: time.Now(),
	}

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
