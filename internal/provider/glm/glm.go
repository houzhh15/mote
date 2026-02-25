// Package glm implements the Provider interface for GLM (智谱AI).
// GLM provides an OpenAI-compatible chat completions API.
// API docs: https://docs.bigmodel.cn/cn/api/introduction
package glm

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
	"time"

	"mote/internal/provider"
	"mote/pkg/logger"
)

// Compile-time interface checks.
var (
	_ provider.Provider        = (*GLMProvider)(nil)
	_ provider.HealthCheckable = (*GLMProvider)(nil)
)

// Error definitions.
var (
	ErrConnectionFailed = errors.New("failed to connect to GLM API")
	ErrModelNotFound    = errors.New("model not found")
	ErrInvalidResponse  = errors.New("invalid response from GLM")
	ErrRequestTimeout   = errors.New("request timeout")
	ErrAuthFailed       = errors.New("authentication failed")
)

// GLMProvider implements the Provider interface for GLM (智谱AI).
type GLMProvider struct {
	apiKey       string
	endpoint     string
	model        string
	maxTokens    int
	httpClient   *http.Client // For non-streaming requests (has overall timeout)
	streamClient *http.Client // For streaming requests (no body read timeout)
}

// NewGLMProvider creates a new GLM provider.
func NewGLMProvider(cfg Config) provider.Provider {
	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultEndpoint
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}

	return &GLMProvider{
		apiKey:    cfg.APIKey,
		endpoint:  cfg.Endpoint,
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
				ResponseHeaderTimeout: cfg.Timeout, // Timeout for headers only, not body
				IdleConnTimeout:       90 * time.Second,
			},
		},
	}
}

// Factory creates a ProviderFactory for GLM.
func Factory(apiKey string, maxTokens int) provider.ProviderFactory {
	return func(model string) (provider.Provider, error) {
		cfg := Config{
			APIKey:    apiKey,
			Endpoint:  DefaultEndpoint,
			Model:     model,
			MaxTokens: maxTokens,
			Timeout:   DefaultTimeout,
		}
		return NewGLMProvider(cfg), nil
	}
}

// FactoryWithEndpoint creates a ProviderFactory for GLM with a custom endpoint.
func FactoryWithEndpoint(apiKey string, maxTokens int, endpoint string) provider.ProviderFactory {
	return func(model string) (provider.Provider, error) {
		cfg := Config{
			APIKey:    apiKey,
			Endpoint:  endpoint,
			Model:     model,
			MaxTokens: maxTokens,
			Timeout:   DefaultTimeout,
		}
		return NewGLMProvider(cfg), nil
	}
}

// ListModels returns the list of available GLM models.
func ListModels() []string {
	return AvailableModels
}

// Name returns the provider name.
func (p *GLMProvider) Name() string {
	return "glm"
}

// Models returns the list of available models.
func (p *GLMProvider) Models() []string {
	return AvailableModels
}

// MaxOutput returns the maximum output tokens for the given model.
// Implements provider.MaxOutputProvider.
func (p *GLMProvider) MaxOutput(model string) int {
	if model == "" {
		model = p.model
	}
	model = strings.TrimPrefix(model, "glm:")
	if meta, ok := ModelMetadata[model]; ok {
		return meta.MaxOutput
	}
	return 0
}

// ContextWindow returns the context window size for the given model.
// Implements provider.ContextWindowProvider.
func (p *GLMProvider) ContextWindow(model string) int {
	if model == "" {
		model = p.model
	}
	// Strip provider prefix (session stores "glm:model-name")
	model = strings.TrimPrefix(model, "glm:")
	if meta, ok := ModelMetadata[model]; ok {
		return meta.ContextWindow
	}
	return 0
}

// Chat sends a chat completion request and returns the response.
func (p *GLMProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	chatReq := p.buildRequest(req, false)

	logger.Debug().Str("model", chatReq.Model).Msg("GLM Chat request")

	resp, err := p.doRequest(ctx, "/chat/completions", chatReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.Error().Int("status", resp.StatusCode).Str("body", string(body)).Msg("GLM error response")
		return nil, p.handleErrorResponse(resp.StatusCode, body)
	}

	if len(body) == 0 {
		logger.Warn().Int("status", resp.StatusCode).Msg("GLM Chat returned empty body")
		return nil, &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   "GLM Chat returned empty response (HTTP 200)",
			Provider:  "glm",
			Retryable: true,
		}
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		logger.Error().Err(err).Str("body", string(body)).Msg("Failed to parse GLM response")
		return nil, ErrInvalidResponse
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("glm API error: [%s] %s", chatResp.Error.Type, chatResp.Error.Message)
	}

	return p.convertResponse(&chatResp), nil
}

// Stream sends a streaming chat completion request.
func (p *GLMProvider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.ChatEvent, error) {
	chatReq := p.buildRequest(req, true)

	logger.Debug().Str("model", chatReq.Model).
		Int("message_count", len(chatReq.Messages)).
		Msg("GLM Stream request")

	resp, err := p.doStreamRequest(ctx, "/chat/completions", chatReq)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, p.handleErrorResponse(resp.StatusCode, body)
	}

	// Handle non-SSE responses (e.g., when the API returns JSON instead of SSE)
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		result, err := p.handleNonStreamResponse(resp)
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	return ProcessStream(resp.Body), nil
}

// handleNonStreamResponse handles the case where GLM returns a regular JSON
// response instead of an SSE stream. It converts the response into synthetic
// stream events.
func (p *GLMProvider) handleNonStreamResponse(resp *http.Response) (<-chan provider.ChatEvent, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read non-stream response: %w", err)
	}

	if len(body) == 0 {
		return nil, &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   "GLM returned empty response",
			Provider:  "glm",
			Retryable: true,
		}
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		logger.Error().Err(err).Str("body", string(body)).Msg("GLM returned non-SSE response that couldn't be parsed")
		return nil, fmt.Errorf("%w: unexpected non-streaming response", ErrInvalidResponse)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("glm API error: [%s] %s", chatResp.Error.Type, chatResp.Error.Message)
	}

	// Convert to synthetic stream events
	events := make(chan provider.ChatEvent, 4)
	go func() {
		defer close(events)

		content := ""
		finishReason := "stop"
		if len(chatResp.Choices) > 0 {
			if chatResp.Choices[0].Message.Content != nil {
				content = *chatResp.Choices[0].Message.Content
			}
			if chatResp.Choices[0].FinishReason != "" {
				finishReason = chatResp.Choices[0].FinishReason
			}

			// Emit tool calls from non-streaming response
			for _, tc := range chatResp.Choices[0].Message.ToolCalls {
				events <- provider.ChatEvent{
					Type: provider.EventTypeToolCall,
					ToolCall: &provider.ToolCall{
						ID:        tc.ID,
						Type:      "function",
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}

		if content != "" {
			events <- provider.ChatEvent{
				Type:  provider.EventTypeContent,
				Delta: content,
			}
		}

		doneEvent := provider.ChatEvent{
			Type:         provider.EventTypeDone,
			FinishReason: finishReason,
		}
		if chatResp.Usage != nil {
			doneEvent.Usage = &provider.Usage{
				PromptTokens:     chatResp.Usage.PromptTokens,
				CompletionTokens: chatResp.Usage.CompletionTokens,
				TotalTokens:      chatResp.Usage.TotalTokens,
			}
		}
		events <- doneEvent
	}()

	logger.Warn().Str("content_type", resp.Header.Get("Content-Type")).
		Msg("GLM returned non-streaming response for stream request, converted to synthetic stream")

	return events, nil
}

// buildRequest converts a provider.ChatRequest to a GLM OpenAI-compatible request.
func (p *GLMProvider) buildRequest(req provider.ChatRequest, stream bool) *chatRequest {
	model := req.Model
	if model == "" {
		model = p.model
	}

	// Strip "glm:" prefix if present
	if strings.HasPrefix(model, "glm:") {
		model = strings.TrimPrefix(model, "glm:")
	}

	hasTools := len(req.Tools) > 0

	chatReq := &chatRequest{
		Model:    model,
		Messages: make([]chatMessage, 0, len(req.Messages)),
		Stream:   stream,
	}

	// Enable thinking mode for models that support it (streaming only)
	if stream {
		if meta, ok := ModelMetadata[model]; ok && meta.SupportsReasoning {
			chatReq.Thinking = &thinking{
				Type: "enabled",
			}
		}
		// Enable tool_stream for models that support it
		if meta, ok := ModelMetadata[model]; ok && meta.SupportsToolStream && hasTools {
			chatReq.ToolStream = true
		}
	}

	// Set max_tokens
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = p.maxTokens
	}
	if maxTokens > 0 {
		chatReq.MaxTokens = maxTokens
	}

	// GLM temperature range is [0.0, 1.0], default 0.95
	if req.Temperature > 0 {
		temp := req.Temperature
		if temp > 1.0 {
			temp = 1.0
		}
		chatReq.Temperature = &temp
	}

	// Convert messages
	for _, msg := range req.Messages {
		// Skip tool messages if no tools requested
		if !hasTools {
			if msg.Role == "tool" {
				continue
			}
			if msg.Role == "assistant" && len(msg.ToolCalls) > 0 && msg.Content == "" {
				continue
			}
		}

		chatMsg := chatMessage{
			Role: msg.Role,
		}

		// GLM requires content to be null (not empty string "") for
		// assistant messages that only carry tool_calls.
		if msg.Content != "" {
			chatMsg.Content = strPtr(msg.Content)
		} else if msg.Role != "assistant" || len(msg.ToolCalls) == 0 {
			chatMsg.Content = strPtr(msg.Content)
		}
		// else: assistant with tool_calls and empty content → Content stays nil → JSON null

		// Set tool_call_id for tool responses
		if msg.Role == "tool" && msg.ToolCallID != "" {
			chatMsg.ToolCallID = msg.ToolCallID
		}

		// Convert tool calls
		if hasTools {
			for _, tc := range msg.ToolCalls {
				openaiTC := chatToolCall{
					ID:   tc.ID,
					Type: "function",
				}
				if tc.Function != nil {
					openaiTC.Function.Name = tc.Function.Name
					openaiTC.Function.Arguments = tc.Function.Arguments
				} else {
					openaiTC.Function.Name = tc.Name
					openaiTC.Function.Arguments = tc.Arguments
				}
				chatMsg.ToolCalls = append(chatMsg.ToolCalls, openaiTC)
			}
		}

		chatReq.Messages = append(chatReq.Messages, chatMsg)
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

	// Process attachments (add to last user message)
	if len(req.Attachments) > 0 && len(chatReq.Messages) > 0 {
		// Check if there are image attachments
		hasImages := false
		for _, att := range req.Attachments {
			if att.Type == "image_url" && att.ImageURL != nil {
				hasImages = true
				break
			}
		}

		for i := len(chatReq.Messages) - 1; i >= 0; i-- {
			if chatReq.Messages[i].Role == "user" {
				if hasImages {
					// Vision mode: convert to multipart content array
					var parts []contentPart

					// Add text part from existing content
					if chatReq.Messages[i].Content != nil && *chatReq.Messages[i].Content != "" {
						parts = append(parts, contentPart{
							Type: "text",
							Text: *chatReq.Messages[i].Content,
						})
					}

					// Add all attachments as content parts
					for _, att := range req.Attachments {
						switch att.Type {
						case "image_url":
							if att.ImageURL != nil {
								parts = append(parts, contentPart{
									Type:     "image_url",
									ImageURL: &visionImageURL{URL: att.ImageURL.URL},
								})
								logger.Info().
									Str("filename", att.Filename).
									Str("mimeType", att.MimeType).
									Msg("GLM: adding image attachment to vision content")
							}
						case "text":
							contentText := fmt.Sprintf("\n\n--- File: %s ---\n```%s\n%s\n```",
								att.Filename,
								att.Metadata["language"],
								att.Text)
							parts = append(parts, contentPart{
								Type: "text",
								Text: contentText,
							})
						}
					}

					// Switch to multipart content
					chatReq.Messages[i].Content = nil
					chatReq.Messages[i].ContentParts = parts

					logger.Info().
						Int("parts", len(parts)).
						Msg("GLM: using multipart vision content")
				} else {
					// Text-only attachments: append to Content string as before
					for _, att := range req.Attachments {
						if att.Type == "text" {
							contentText := fmt.Sprintf("\n\n--- File: %s ---\n```%s\n%s\n```",
								att.Filename,
								att.Metadata["language"],
								att.Text)
							if chatReq.Messages[i].Content != nil {
								*chatReq.Messages[i].Content += contentText
							} else {
								chatReq.Messages[i].Content = strPtr(contentText)
							}
						}
					}
				}
				break
			}
		}
	}

	return chatReq
}

// doRequest sends an HTTP request to the GLM API.
func (p *GLMProvider) doRequest(ctx context.Context, path string, body interface{}) (*http.Response, error) {
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
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrRequestTimeout
		}
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}

	return resp, nil
}

// doStreamRequest sends an HTTP request using the stream client (no body read timeout).
func (p *GLMProvider) doStreamRequest(ctx context.Context, path string, body interface{}) (*http.Response, error) {
	url := p.endpoint + path

	var reqBody io.Reader
	var reqSize int
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		reqSize = len(data)
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	logger.Debug().Int("request_bytes", reqSize).Str("url", url).Msg("GLM doStreamRequest sending")

	resp, err := p.streamClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrRequestTimeout
		}
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}

	logger.Debug().
		Int("status", resp.StatusCode).
		Str("content_type", resp.Header.Get("Content-Type")).
		Int("request_bytes", reqSize).
		Msg("GLM doStreamRequest response")

	return resp, nil
}

// handleErrorResponse converts an error response to an appropriate error.
func (p *GLMProvider) handleErrorResponse(statusCode int, body []byte) error {
	var errResp chatResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil {
		msg := errResp.Error.Message
		// Check for context window exceeded before switch
		lowerMsg := strings.ToLower(msg)
		if strings.Contains(lowerMsg, "context window") ||
			strings.Contains(lowerMsg, "context length") ||
			strings.Contains(lowerMsg, "too many tokens") ||
			strings.Contains(lowerMsg, "maximum context length") {
			return &provider.ProviderError{
				Code:      provider.ErrCodeContextWindowExceeded,
				Message:   msg,
				Provider:  "glm",
				Retryable: true,
			}
		}

		switch statusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("%w: %s", ErrAuthFailed, msg)
		case http.StatusNotFound:
			return fmt.Errorf("%w: %s", ErrModelNotFound, msg)
		case http.StatusTooManyRequests:
			// 区分并发限制（可重试）和余额不足（不可重试）。
			// 并发 429 通常包含 "concurrency" / "并发" 关键词。
			retryable := strings.Contains(lowerMsg, "concurren") ||
				strings.Contains(lowerMsg, "并发") ||
				strings.Contains(lowerMsg, "rate") ||
				strings.Contains(lowerMsg, "too many")
			hint := msg
			if !retryable {
				hint = msg + " (提示：请确认端点为 CodingPlan 地址 https://open.bigmodel.cn/api/coding/paas/v4 而非按量付费地址)"
			}
			return &provider.ProviderError{
				Code:      provider.ErrCodeRateLimited,
				Message:   hint,
				Provider:  "glm",
				Retryable: retryable,
			}
		case http.StatusBadRequest:
			return &provider.ProviderError{
				Code:      provider.ErrCodeInvalidRequest,
				Message:   fmt.Sprintf("[%s] %s", errResp.Error.Type, msg),
				Provider:  "glm",
				Retryable: false,
			}
		default:
			return fmt.Errorf("glm error (%d): [%s] %s", statusCode, errResp.Error.Type, msg)
		}
	}

	switch statusCode {
	case http.StatusUnauthorized:
		return ErrAuthFailed
	case http.StatusNotFound:
		return ErrModelNotFound
	case http.StatusServiceUnavailable:
		return ErrConnectionFailed
	default:
		return fmt.Errorf("glm returned status %d: %s", statusCode, string(body))
	}
}

// convertResponse converts a GLM response to a provider response.
func (p *GLMProvider) convertResponse(resp *chatResponse) *provider.ChatResponse {
	result := &provider.ChatResponse{
		FinishReason: provider.FinishReasonStop,
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		if choice.Message.Content != nil {
			result.Content = *choice.Message.Content
		}

		if choice.FinishReason == "tool_calls" {
			result.FinishReason = provider.FinishReasonToolCalls
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
	}

	if resp.Usage != nil {
		result.Usage = &provider.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	return result
}

// Ping checks if the GLM API is reachable and the API key is valid.
// Implements provider.HealthCheckable interface.
func (p *GLMProvider) Ping(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Build a minimal request with empty messages — this will be rejected
	// as a bad request (400) but proves connectivity and auth without
	// consuming any token quota.
	pingReq := chatRequest{
		Model:    DefaultModel,
		Messages: []chatMessage{},
	}

	body, err := json.Marshal(pingReq)
	if err != nil {
		return &provider.ProviderError{
			Code:      provider.ErrCodeNetworkError,
			Message:   fmt.Sprintf("创建请求失败: %v", err),
			Provider:  "glm",
			Retryable: true,
		}
	}

	url := p.endpoint + "/chat/completions"
	req, err := http.NewRequestWithContext(checkCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return &provider.ProviderError{
			Code:      provider.ErrCodeNetworkError,
			Message:   fmt.Sprintf("创建请求失败: %v", err),
			Provider:  "glm",
			Retryable: true,
		}
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   "GLM API 无法连接",
			Provider:  "glm",
			Retryable: true,
		}
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &provider.ProviderError{
			Code:      provider.ErrCodeAuthFailed,
			Message:   "GLM API Key 无效",
			Provider:  "glm",
			Retryable: false,
		}
	case http.StatusOK, http.StatusBadRequest:
		// 200 = unexpected but fine; 400 = expected (empty messages rejected)
		// Both prove the API is reachable and the key is valid.
		return nil
	case http.StatusTooManyRequests:
		// Rate limited but API is reachable and key is valid
		return nil
	default:
		respBody, _ := io.ReadAll(resp.Body)
		return &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   fmt.Sprintf("GLM API 返回异常状态码: %d, body: %s", resp.StatusCode, string(respBody)),
			Provider:  "glm",
			Retryable: true,
		}
	}
}

// GetState returns the current state of the GLM provider.
// Implements provider.HealthCheckable interface.
func (p *GLMProvider) GetState() provider.ProviderState {
	state := provider.ProviderState{
		Name:      "glm",
		LastCheck: time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
