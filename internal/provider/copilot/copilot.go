package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"mote/internal/provider"
	"mote/pkg/logger"
)

const (
	// DefaultMaxTokens is the default max tokens for responses.
	DefaultMaxTokens = 8192

	// MaxRetries is the maximum number of retries for transient errors.
	MaxRetries = 3

	// InitialBackoff is the initial backoff duration for retries.
	InitialBackoff = 1 * time.Second
)

// LegacySupportedModels lists model IDs for backward compatibility.
// Use SupportedModels from models.go for detailed model info.
var LegacySupportedModels = []string{
	"claude-sonnet-4-20250514",
	"gpt-4o",
	"gpt-4o-mini",
	"o1",
	"o1-mini",
	"o3-mini",
}

// CopilotProvider implements the Provider interface for GitHub Copilot.
type CopilotProvider struct {
	tokenManager *TokenManager
	httpClient   *http.Client
	model        string
	maxTokens    int

	// Enhanced features
	modeManager  *ModeManager
	usageTracker *UsageTracker
	authManager  *AuthManager
}

// NewCopilotProvider creates a new Copilot provider.
func NewCopilotProvider(githubToken, model string, maxTokens int) provider.Provider {
	if model == "" {
		model = DefaultModel
	}
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}

	return &CopilotProvider{
		tokenManager: NewTokenManager(githubToken),
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Long timeout for streaming
		},
		model:        model,
		maxTokens:    maxTokens,
		modeManager:  NewModeManager(),
		usageTracker: NewUsageTracker(),
		authManager:  NewAuthManager(),
	}
}

// Name returns the provider name.
func (p *CopilotProvider) Name() string {
	return "copilot"
}

// Models returns the list of supported models.
func (p *CopilotProvider) Models() []string {
	return ListModels()
}

// GetMode returns the current agent mode.
func (p *CopilotProvider) GetMode() Mode {
	return p.modeManager.GetMode()
}

// SetMode sets the current agent mode.
func (p *CopilotProvider) SetMode(mode Mode) error {
	return p.modeManager.SetMode(mode)
}

// GetModeInfo returns information about the current mode.
func (p *CopilotProvider) GetModeInfo() ModeInfo {
	return p.modeManager.GetModeInfo()
}

// GetUsageStatus returns the current usage quota status.
func (p *CopilotProvider) GetUsageStatus() QuotaStatus {
	return p.usageTracker.GetQuotaStatus()
}

// GetCurrentMonthUsage returns usage statistics for the current month.
func (p *CopilotProvider) GetCurrentMonthUsage() MonthlyUsage {
	return p.usageTracker.GetCurrentMonthUsage()
}

// Authenticate performs OAuth Device Flow authentication.
func (p *CopilotProvider) Authenticate(ctx context.Context, onPrompt func(userCode, verificationURI string)) (string, error) {
	return p.authManager.Authenticate(ctx, onPrompt)
}

// Chat sends a chat completion request and returns the response.
func (p *CopilotProvider) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	return p.chatWithRetry(ctx, &req, false)
}

// Stream sends a streaming chat completion request and returns a channel of events.
func (p *CopilotProvider) Stream(ctx context.Context, req provider.ChatRequest) (<-chan provider.ChatEvent, error) {
	events := make(chan provider.ChatEvent)

	go func() {
		defer close(events)

		resp, err := p.doStreamRequest(ctx, &req)
		if err != nil {
			events <- provider.ChatEvent{
				Type:  provider.EventTypeError,
				Error: err,
			}
			return
		}
		defer resp.Body.Close()

		// Process SSE stream
		for event := range ProcessSSE(resp.Body) {
			select {
			case <-ctx.Done():
				events <- provider.ChatEvent{
					Type:  provider.EventTypeError,
					Error: ctx.Err(),
				}
				return
			case events <- event:
			}
		}
	}()

	return events, nil
}

// chatWithRetry performs a chat request with retry logic.
func (p *CopilotProvider) chatWithRetry(ctx context.Context, req *provider.ChatRequest, stream bool) (*provider.ChatResponse, error) {
	var lastErr error
	backoff := InitialBackoff

	for attempt := 0; attempt <= MaxRetries; attempt++ {
		if attempt > 0 {
			logger.Debug().
				Int("attempt", attempt).
				Dur("backoff", backoff).
				Msg("Retrying Copilot request")

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2 // Exponential backoff
		}

		resp, err := p.doRequest(ctx, req, stream)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Only retry on transient errors
		if !isRetryableError(err) {
			return nil, err
		}

		// Invalidate token on auth errors (might be expired)
		if err == ErrUnauthorized {
			p.tokenManager.Invalidate()
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// doRequest performs the actual HTTP request.
func (p *CopilotProvider) doRequest(ctx context.Context, req *provider.ChatRequest, stream bool) (*provider.ChatResponse, error) {
	token, err := p.tokenManager.GetToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	// Build request body
	body := p.buildRequestBody(req, stream)
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	logger.Debug().
		Int("body_size", len(bodyBytes)).
		Int("messages_count", len(req.Messages)).
		Int("tools_count", len(req.Tools)).
		Msg("Sending request to Copilot API")

	// Create HTTP request
	url := p.tokenManager.GetBaseURL() + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Copilot-Integration-Id", "vscode-chat")
	httpReq.Header.Set("Accept", "application/json")

	// Send request
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle errors
	if err := p.handleHTTPError(resp); err != nil {
		return nil, err
	}

	// Parse response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var chatResp chatCompletionResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return p.convertResponse(&chatResp), nil
}

// doStreamRequest performs a streaming HTTP request.
func (p *CopilotProvider) doStreamRequest(ctx context.Context, req *provider.ChatRequest) (*http.Response, error) {
	var lastErr error
	backoff := InitialBackoff

	for attempt := 0; attempt <= MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		token, err := p.tokenManager.GetToken()
		if err != nil {
			lastErr = err
			if err == ErrUnauthorized {
				p.tokenManager.Invalidate()
			}
			continue
		}

		// Build request body
		body := p.buildRequestBody(req, true)
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}

		// Debug: print first message (system prompt)
		if len(req.Messages) > 0 {
			logger.Debug().
				Int("system_prompt_len", len(req.Messages[0].Content)).
				Msg("System prompt size")
		}

		logger.Info().
			Int("body_size", len(bodyBytes)).
			Int("messages_count", len(req.Messages)).
			Int("tools_count", len(req.Tools)).
			Msg("Sending streaming request to Copilot API")

		// Create HTTP request
		url := p.tokenManager.GetBaseURL() + "/chat/completions"
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers
		httpReq.Header.Set("Authorization", "Bearer "+token)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Copilot-Integration-Id", "vscode-chat")
		httpReq.Header.Set("Accept", "text/event-stream")

		// Send request
		resp, err := p.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}

		// Check for errors
		if resp.StatusCode != http.StatusOK {
			// Read body before closing for error message
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			logger.Error().
				Int("status", resp.StatusCode).
				Str("body", string(body)).
				Int("request_body_size", len(bodyBytes)).
				Int("messages_count", len(req.Messages)).
				Int("tools_count", len(req.Tools)).
				Msg("Copilot streaming API error")

			// Try to parse structured error
			var apiErr apiErrorResponse
			if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error.Code != "" {
				lastErr = p.parseAPIError(resp.StatusCode, apiErr)
				// Don't retry for client errors (4xx) with specific error codes
				if resp.StatusCode >= 400 && resp.StatusCode < 500 {
					return nil, lastErr
				}
			} else {
				// Fallback to status-based handling
				if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
					lastErr = &provider.ProviderError{
						Code:      provider.ErrCodeAuthFailed,
						Message:   "GitHub Token 无效或已过期",
						Provider:  "copilot",
						Retryable: false,
					}
					p.tokenManager.Invalidate()
					return nil, lastErr
				} else if resp.StatusCode == http.StatusTooManyRequests {
					lastErr = &provider.ProviderError{
						Code:       provider.ErrCodeRateLimited,
						Message:    "请求过于频繁，请稍后重试",
						Provider:   "copilot",
						Retryable:  true,
						RetryAfter: 60,
					}
					return nil, lastErr
				} else if resp.StatusCode >= 500 {
					// Retry on 5xx errors
					lastErr = &provider.ProviderError{
						Code:      provider.ErrCodeServiceUnavailable,
						Message:   fmt.Sprintf("服务错误 (%d): %s", resp.StatusCode, string(body)),
						Provider:  "copilot",
						Retryable: true,
					}
					continue
				}
				lastErr = &provider.ProviderError{
					Code:      provider.ErrCodeUnknown,
					Message:   fmt.Sprintf("API 错误 (%d): %s", resp.StatusCode, string(body)),
					Provider:  "copilot",
					Retryable: false,
				}
			}
			return nil, lastErr
		}

		return resp, nil
	}

	return nil, &provider.ProviderError{
		Code:      provider.ErrCodeServiceUnavailable,
		Message:   fmt.Sprintf("重试次数已达上限: %v", lastErr),
		Provider:  "copilot",
		Retryable: false,
	}
}

// buildRequestBody builds the request body for the API.
func (p *CopilotProvider) buildRequestBody(req *provider.ChatRequest, stream bool) map[string]interface{} {
	model := req.Model
	if model == "" {
		model = p.model
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = p.maxTokens
	}

	body := map[string]interface{}{
		"model":      model,
		"messages":   p.convertMessages(req.Messages),
		"stream":     stream,
		"max_tokens": maxTokens,
	}

	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}

	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
	}

	return body
}

// convertMessages converts provider messages to API format.
func (p *CopilotProvider) convertMessages(messages []provider.Message) []map[string]interface{} {
	result := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		m := map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if len(msg.ToolCalls) > 0 {
			// Convert tool calls to proper format
			toolCalls := make([]map[string]interface{}, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				toolCalls[j] = map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				}
			}
			m["tool_calls"] = toolCalls
		}
		if msg.ToolCallID != "" {
			m["tool_call_id"] = msg.ToolCallID
		}
		result[i] = m

		// Debug log for tool role messages
		if msg.Role == "tool" {
			logger.Info().
				Str("role", string(msg.Role)).
				Str("tool_call_id", msg.ToolCallID).
				Int("content_len", len(msg.Content)).
				Msg("Tool message in request")
		}
	}
	return result
}

// apiErrorResponse represents an error response from the Copilot API.
type apiErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Code    string `json:"code"`
		Type    string `json:"type"`
		Param   string `json:"param"`
	} `json:"error"`
}

// handleHTTPError converts HTTP error responses to ProviderError.
func (p *CopilotProvider) handleHTTPError(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}

	// Read response body
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	logger.Error().
		Int("status", resp.StatusCode).
		Str("body", bodyStr).
		Msg("Copilot API error")

	// Try to parse structured error response
	var apiErr apiErrorResponse
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error.Code != "" {
		return p.parseAPIError(resp.StatusCode, apiErr)
	}

	// Fallback to status-based classification
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &provider.ProviderError{
			Code:      provider.ErrCodeAuthFailed,
			Message:   "GitHub Token 无效或已过期",
			Provider:  "copilot",
			Retryable: false,
		}
	case http.StatusTooManyRequests:
		return &provider.ProviderError{
			Code:       provider.ErrCodeRateLimited,
			Message:    "请求过于频繁，请稍后重试",
			Provider:   "copilot",
			Retryable:  true,
			RetryAfter: 60,
		}
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   "Copilot 服务暂时不可用",
			Provider:  "copilot",
			Retryable: true,
		}
	default:
		return &provider.ProviderError{
			Code:      provider.ErrCodeUnknown,
			Message:   fmt.Sprintf("API 错误 (%d): %s", resp.StatusCode, bodyStr),
			Provider:  "copilot",
			Retryable: false,
		}
	}
}

// parseAPIError converts a structured API error to ProviderError.
func (p *CopilotProvider) parseAPIError(statusCode int, apiErr apiErrorResponse) *provider.ProviderError {
	errCode := apiErr.Error.Code
	errMsg := apiErr.Error.Message
	errType := apiErr.Error.Type

	// Map API error codes to provider error codes
	switch errCode {
	case "model_not_supported", "model_not_found":
		return &provider.ProviderError{
			Code:      provider.ErrCodeModelNotFound,
			Message:   fmt.Sprintf("模型不支持: %s", errMsg),
			Provider:  "copilot",
			Retryable: false,
		}
	case "context_length_exceeded":
		return &provider.ProviderError{
			Code:      provider.ErrCodeInvalidRequest,
			Message:   fmt.Sprintf("上下文长度超限: %s", errMsg),
			Provider:  "copilot",
			Retryable: false,
		}
	case "max_tokens_exceeded", "tokens_exceeded":
		return &provider.ProviderError{
			Code:      provider.ErrCodeInvalidRequest,
			Message:   fmt.Sprintf("Token 数量超限: %s", errMsg),
			Provider:  "copilot",
			Retryable: false,
		}
	case "rate_limit_exceeded":
		return &provider.ProviderError{
			Code:       provider.ErrCodeRateLimited,
			Message:    fmt.Sprintf("请求频率超限: %s", errMsg),
			Provider:   "copilot",
			Retryable:  true,
			RetryAfter: 60,
		}
	case "invalid_api_key", "authentication_error":
		return &provider.ProviderError{
			Code:      provider.ErrCodeAuthFailed,
			Message:   fmt.Sprintf("认证失败: %s", errMsg),
			Provider:  "copilot",
			Retryable: false,
		}
	case "insufficient_quota":
		return &provider.ProviderError{
			Code:      provider.ErrCodeQuotaExceeded,
			Message:   fmt.Sprintf("配额不足: %s", errMsg),
			Provider:  "copilot",
			Retryable: false,
		}
	default:
		// Use error type as fallback
		if errType == "invalid_request_error" {
			return &provider.ProviderError{
				Code:      provider.ErrCodeInvalidRequest,
				Message:   fmt.Sprintf("请求错误: %s", errMsg),
				Provider:  "copilot",
				Retryable: false,
			}
		}
		return &provider.ProviderError{
			Code:      provider.ErrCodeUnknown,
			Message:   fmt.Sprintf("API 错误 [%s]: %s", errCode, errMsg),
			Provider:  "copilot",
			Retryable: false,
		}
	}
}

// convertResponse converts an API response to a ChatResponse.
func (p *CopilotProvider) convertResponse(resp *chatCompletionResponse) *provider.ChatResponse {
	result := &provider.ChatResponse{}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		result.Content = choice.Message.Content
		result.FinishReason = choice.FinishReason

		// Convert tool calls
		for _, tc := range choice.Message.ToolCalls {
			result.ToolCalls = append(result.ToolCalls, provider.ToolCall{
				ID:        tc.ID,
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

// isRetryableError checks if an error is retryable.
func isRetryableError(err error) bool {
	return err == ErrRateLimited || err == ErrServiceUnavailable || err == ErrUnauthorized
}

// chatCompletionResponse represents the API response.
type chatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// init registers the Copilot provider.
// Note: Registration is not done here because it requires a GitHub token.
// Users should call NewCopilotProvider and register it explicitly.

// Factory creates a ProviderFactory function that produces CopilotProviders with the
// specified GitHub token and max tokens. The returned factory can be used with provider.Pool
// to create providers for different models while sharing the same authentication.
//
// Usage:
//
//	factory := copilot.Factory(githubToken, maxTokens)
//	pool := provider.NewPool(factory)
//	pool.SetDefault("chat", "grok-code-fast-1")
//	pool.SetDefault("cron", "gpt-4o-mini")
//	prov, err := pool.Get("gpt-4o")
func Factory(githubToken string, maxTokens int) provider.ProviderFactory {
	return func(model string) (provider.Provider, error) {
		return NewCopilotProvider(githubToken, model, maxTokens), nil
	}
}

// Ping checks if the Copilot provider is available by verifying the token.
// Implements provider.HealthCheckable interface.
func (p *CopilotProvider) Ping(ctx context.Context) error {
	// Create a context with timeout for the health check
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Try to get a valid token - this will refresh if needed
	_, err := p.tokenManager.GetToken()
	if err != nil {
		return p.classifyError(err)
	}

	// Check context cancellation
	select {
	case <-checkCtx.Done():
		return &provider.ProviderError{
			Code:      provider.ErrCodeTimeout,
			Message:   "health check timeout",
			Provider:  "copilot",
			Retryable: true,
		}
	default:
		return nil
	}
}

// GetState returns the current state of the Copilot provider.
// Implements provider.HealthCheckable interface.
func (p *CopilotProvider) GetState() provider.ProviderState {
	state := provider.ProviderState{
		Name:      "copilot",
		LastCheck: time.Now(),
		Models:    p.Models(),
	}

	// Check token status
	tokenStatus := p.tokenManager.GetStatus()
	if !tokenStatus.Valid {
		state.Status = provider.StatusAuthFailed
		state.LastError = tokenStatus.Message
		return state
	}

	if tokenStatus.Warning != "" {
		state.LastError = tokenStatus.Warning
	}

	if tokenStatus.ExpiresAt != nil {
		state.TokenExpiry = tokenStatus.ExpiresAt
	}

	state.Status = provider.StatusConnected
	return state
}

// classifyError converts a generic error to a ProviderError with appropriate code.
func (p *CopilotProvider) classifyError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case err == ErrUnauthorized:
		return &provider.ProviderError{
			Code:      provider.ErrCodeAuthFailed,
			Message:   "GitHub Token 无效或已过期",
			Provider:  "copilot",
			Retryable: false,
		}
	case err == ErrRateLimited:
		return &provider.ProviderError{
			Code:       provider.ErrCodeRateLimited,
			Message:    "请求过于频繁，请稍后重试",
			Provider:   "copilot",
			Retryable:  true,
			RetryAfter: 60, // Default 60 seconds
		}
	case err == ErrServiceUnavailable:
		return &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   "Copilot 服务暂时不可用",
			Provider:  "copilot",
			Retryable: true,
		}
	case err == ErrTokenExpired:
		return &provider.ProviderError{
			Code:      provider.ErrCodeTokenExpired,
			Message:   "Copilot Token 已过期，正在刷新",
			Provider:  "copilot",
			Retryable: true,
		}
	default:
		return &provider.ProviderError{
			Code:      provider.ErrCodeUnknown,
			Message:   err.Error(),
			Provider:  "copilot",
			Retryable: false,
		}
	}
}
