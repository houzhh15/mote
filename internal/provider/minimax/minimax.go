// Package minimax implements the Provider interface for MiniMax AI.
// MiniMax provides an OpenAI-compatible chat completions API.
// API docs: https://platform.minimax.io/docs/api-reference/text-openai-api
package minimax

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
	_ provider.Provider        = (*MinimaxProvider)(nil)
	_ provider.HealthCheckable = (*MinimaxProvider)(nil)
)

// Error definitions.
var (
	ErrConnectionFailed = errors.New("failed to connect to MiniMax API")
	ErrModelNotFound    = errors.New("model not found")
	ErrInvalidResponse  = errors.New("invalid response from MiniMax")
	ErrRequestTimeout   = errors.New("request timeout")
	ErrAuthFailed       = errors.New("authentication failed")
)

// MinimaxProvider implements the Provider interface for MiniMax.
type MinimaxProvider struct {
	apiKey       string
	endpoint     string
	model        string
	maxTokens    int
	httpClient   *http.Client // For non-streaming requests (has overall timeout)
	streamClient *http.Client // For streaming requests (no body read timeout)
}

// NewMinimaxProvider creates a new MiniMax provider.
func NewMinimaxProvider(cfg Config) provider.Provider {
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

	return &MinimaxProvider{
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

// Factory creates a ProviderFactory for MiniMax.
func Factory(apiKey string, maxTokens int) provider.ProviderFactory {
	return func(model string) (provider.Provider, error) {
		cfg := Config{
			APIKey:    apiKey,
			Endpoint:  DefaultEndpoint,
			Model:     model,
			MaxTokens: maxTokens,
			Timeout:   DefaultTimeout,
		}
		return NewMinimaxProvider(cfg), nil
	}
}

// ListModels returns the list of available MiniMax models.
func ListModels() []string {
	return AvailableModels
}

// Name returns the provider name.
func (p *MinimaxProvider) Name() string {
	return "minimax"
}

// Models returns the list of available models.
func (p *MinimaxProvider) Models() []string {
	return AvailableModels
}

// Chat sends a chat completion request and returns the response.
func (p *MinimaxProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	chatReq := p.buildRequest(req, false)

	logger.Debug().Str("model", chatReq.Model).Msg("MiniMax Chat request")

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
		logger.Error().Int("status", resp.StatusCode).Str("body", string(body)).Msg("MiniMax error response")
		return nil, p.handleErrorResponse(resp.StatusCode, body)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		logger.Error().Err(err).Str("body", string(body)).Msg("Failed to parse MiniMax response")
		return nil, ErrInvalidResponse
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("minimax API error: [%s] %s", chatResp.Error.Type, chatResp.Error.Message)
	}

	return p.convertResponse(&chatResp), nil
}

// Stream sends a streaming chat completion request.
func (p *MinimaxProvider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.ChatEvent, error) {
	chatReq := p.buildRequest(req, true)

	logger.Debug().Str("model", chatReq.Model).
		Bool("reasoning_split", chatReq.ReasoningSplit).
		Int("message_count", len(chatReq.Messages)).
		Msg("MiniMax Stream request")

	resp, err := p.doStreamRequest(ctx, "/chat/completions", chatReq)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, p.handleErrorResponse(resp.StatusCode, body)
	}

	// Some models/plans return a non-streaming JSON response even when stream=true
	// (e.g. MiniMax-M2.5-highspeed on Coding Plan). Detect by Content-Type and
	// convert to a synthetic stream so the caller gets a proper response.
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		return p.handleNonStreamResponse(resp)
	}

	return ProcessStream(resp.Body), nil
}

// handleNonStreamResponse handles the case where MiniMax returns a regular JSON
// response instead of an SSE stream (e.g. model not available on current plan).
// It converts the response into synthetic stream events.
func (p *MinimaxProvider) handleNonStreamResponse(resp *http.Response) (<-chan provider.ChatEvent, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read non-stream response: %w", err)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		logger.Error().Err(err).Str("body", string(body)).Msg("MiniMax returned non-SSE response that couldn't be parsed")
		return nil, fmt.Errorf("%w: unexpected non-streaming response", ErrInvalidResponse)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("minimax API error: [%s] %s", chatResp.Error.Type, chatResp.Error.Message)
	}

	// Convert to synthetic stream events
	events := make(chan provider.ChatEvent, 4)
	go func() {
		defer close(events)

		content := ""
		finishReason := "stop"
		if len(chatResp.Choices) > 0 {
			content = chatResp.Choices[0].Message.Content
			if chatResp.Choices[0].FinishReason != "" {
				finishReason = chatResp.Choices[0].FinishReason
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
		Msg("MiniMax returned non-streaming response for stream request, converted to synthetic stream")

	return events, nil
}

// buildRequest converts a provider.ChatRequest to a MiniMax OpenAI-compatible request.
func (p *MinimaxProvider) buildRequest(req provider.ChatRequest, stream bool) *chatRequest {
	model := req.Model
	if model == "" {
		model = p.model
	}

	// Strip "minimax:" prefix if present
	if strings.HasPrefix(model, "minimax:") {
		model = strings.TrimPrefix(model, "minimax:")
	}

	hasTools := len(req.Tools) > 0

	// Only enable reasoning_split for streaming AND models that support it.
	// Highspeed models silently return empty responses when reasoning_split=true.
	reasoningSplit := false
	if stream {
		if meta, ok := ModelMetadata[model]; ok {
			reasoningSplit = meta.SupportsReasoning
		}
	}

	chatReq := &chatRequest{
		Model:          model,
		Messages:       make([]chatMessage, 0, len(req.Messages)),
		Stream:         stream,
		ReasoningSplit: reasoningSplit,
	}

	// Set max_tokens
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = p.maxTokens
	}
	if maxTokens > 0 {
		chatReq.MaxTokens = maxTokens
	}

	// MiniMax temperature range is (0.0, 1.0], default recommended 1.0
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
			Role:    msg.Role,
			Content: msg.Content,
		}

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

	// Process attachments (add text to last user message)
	if len(req.Attachments) > 0 && len(chatReq.Messages) > 0 {
		for i := len(chatReq.Messages) - 1; i >= 0; i-- {
			if chatReq.Messages[i].Role == "user" {
				for _, att := range req.Attachments {
					if att.Type == "text" {
						contentText := fmt.Sprintf("\n\n--- File: %s ---\n```%s\n%s\n```",
							att.Filename,
							att.Metadata["language"],
							att.Text)
						chatReq.Messages[i].Content += contentText
					}
				}
				break
			}
		}
	}

	return chatReq
}

// doRequest sends an HTTP request to the MiniMax API.
func (p *MinimaxProvider) doRequest(ctx context.Context, path string, body interface{}) (*http.Response, error) {
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
// This is used for SSE streaming where the body may be read over a long period.
func (p *MinimaxProvider) doStreamRequest(ctx context.Context, path string, body interface{}) (*http.Response, error) {
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

	resp, err := p.streamClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrRequestTimeout
		}
		return nil, fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}

	return resp, nil
}

// handleErrorResponse converts an error response to an appropriate error.
func (p *MinimaxProvider) handleErrorResponse(statusCode int, body []byte) error {
	var errResp chatResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil {
		msg := errResp.Error.Message
		switch statusCode {
		case http.StatusUnauthorized:
			return fmt.Errorf("%w: %s", ErrAuthFailed, msg)
		case http.StatusNotFound:
			return fmt.Errorf("%w: %s", ErrModelNotFound, msg)
		case http.StatusTooManyRequests:
			return &provider.ProviderError{
				Code:      provider.ErrCodeRateLimited,
				Message:   msg,
				Provider:  "minimax",
				Retryable: true,
			}
		default:
			return fmt.Errorf("minimax error (%d): [%s] %s", statusCode, errResp.Error.Type, msg)
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
		return fmt.Errorf("minimax returned status %d: %s", statusCode, string(body))
	}
}

// convertResponse converts a MiniMax response to a provider response.
func (p *MinimaxProvider) convertResponse(resp *chatResponse) *provider.ChatResponse {
	result := &provider.ChatResponse{
		FinishReason: provider.FinishReasonStop,
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		result.Content = choice.Message.Content

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

// Ping checks if the MiniMax API is reachable and the API key is valid.
// Implements provider.HealthCheckable interface.
// We send a request with empty messages to /chat/completions, which will
// return 400 (bad params) but NOT consume any token quota. This verifies:
// 1) Network connectivity  2) API key validity (401/403 = invalid key)
func (p *MinimaxProvider) Ping(ctx context.Context) error {
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
			Provider:  "minimax",
			Retryable: true,
		}
	}

	url := p.endpoint + "/chat/completions"
	req, err := http.NewRequestWithContext(checkCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return &provider.ProviderError{
			Code:      provider.ErrCodeNetworkError,
			Message:   fmt.Sprintf("创建请求失败: %v", err),
			Provider:  "minimax",
			Retryable: true,
		}
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   "MiniMax API 无法连接",
			Provider:  "minimax",
			Retryable: true,
		}
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &provider.ProviderError{
			Code:      provider.ErrCodeAuthFailed,
			Message:   "MiniMax API Key 无效",
			Provider:  "minimax",
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
			Message:   fmt.Sprintf("MiniMax API 返回异常状态码: %d, body: %s", resp.StatusCode, string(respBody)),
			Provider:  "minimax",
			Retryable: true,
		}
	}
}

// GetState returns the current state of the MiniMax provider.
// Implements provider.HealthCheckable interface.
func (p *MinimaxProvider) GetState() provider.ProviderState {
	state := provider.ProviderState{
		Name:      "minimax",
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
