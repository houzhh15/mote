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
	"regexp"
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

// Compile-time check for ConnectionResettable.
var _ provider.ConnectionResettable = (*MinimaxProvider)(nil)

// ResetConnections closes all idle HTTP connections, forcing fresh TCP
// connections on the next request.  This prevents MiniMax's ALB from
// associating subsequent requests with a degraded server-side session
// (X-Session-Id) after sustained Chat API usage (e.g., compaction).
func (p *MinimaxProvider) ResetConnections() {
	p.httpClient.CloseIdleConnections()
	p.streamClient.CloseIdleConnections()
	logger.Info().Msg("MiniMax connections reset")
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

	// Empty body with HTTP 200 is a server-side anomaly (ALB session
	// degradation after sustained API usage, e.g. compaction).
	// Return a retryable error so the runner's transient retry can handle it.
	if len(body) == 0 {
		logger.Warn().
			Int("status", resp.StatusCode).
			Str("x_session_id", resp.Header.Get("X-Session-Id")).
			Msg("MiniMax Chat returned empty body")
		p.httpClient.CloseIdleConnections()
		return nil, &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   "MiniMax Chat returned empty response (HTTP 200) — likely ALB session degradation",
			Provider:  "minimax",
			Retryable: true,
		}
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

	// Diagnostic: dump message roles and tool_call structure for debugging
	// ordering issues (e.g. MiniMax 2013 error)
	for i, msg := range chatReq.Messages {
		role := msg.Role
		tcCount := len(msg.ToolCalls)
		tcID := ""
		if msg.ToolCallID != "" {
			tcID = msg.ToolCallID
		}
		hasContent := msg.Content != nil && *msg.Content != ""
		if tcCount > 0 || tcID != "" {
			logger.Debug().
				Int("idx", i).
				Str("role", role).
				Int("tool_calls", tcCount).
				Str("tool_call_id", tcID).
				Bool("has_content", hasContent).
				Msg("MiniMax msg detail")
		}
	}

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
		result, err := p.handleNonStreamResponse(resp)
		if err != nil {
			// If Stream endpoint returned empty body (ALB anomaly after
			// sustained Chat usage e.g. compaction), fall back to the
			// non-streaming Chat API which is not affected.
			var pe *provider.ProviderError
			if errors.As(err, &pe) && pe.Code == provider.ErrCodeServiceUnavailable {
				logger.Warn().Msg("MiniMax Stream returned empty response — falling back to Chat API")
				return p.streamViaChat(ctx, req)
			}
			return nil, err
		}
		return result, nil
	}

	return ProcessStream(resp.Body), nil
}

// streamViaChat falls back to the non-streaming Chat API when the Stream
// endpoint returns an empty response (ALB anomaly after compaction).  The
// Chat endpoint uses a different HTTP client and is unaffected by the
// streaming endpoint's ALB session issue.  The ChatResponse is converted
// to synthetic stream events so the caller sees no difference.
func (p *MinimaxProvider) streamViaChat(ctx context.Context, req provider.ChatRequest) (<-chan provider.ChatEvent, error) {
	chatResp, err := p.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Chat fallback also failed: %w", err)
	}

	logger.Info().
		Int("content_len", len(chatResp.Content)).
		Int("tool_calls", len(chatResp.ToolCalls)).
		Str("finish_reason", chatResp.FinishReason).
		Msg("MiniMax streamViaChat fallback succeeded")

	events := make(chan provider.ChatEvent, 2+len(chatResp.ToolCalls))
	go func() {
		defer close(events)

		if chatResp.Content != "" {
			events <- provider.ChatEvent{
				Type:  provider.EventTypeContent,
				Delta: chatResp.Content,
			}
		}

		for i := range chatResp.ToolCalls {
			tc := chatResp.ToolCalls[i]
			tc.Index = i
			events <- provider.ChatEvent{
				Type:     provider.EventTypeToolCall,
				ToolCall: &tc,
			}
		}

		doneEvent := provider.ChatEvent{
			Type:         provider.EventTypeDone,
			FinishReason: chatResp.FinishReason,
		}
		if chatResp.Usage != nil {
			doneEvent.Usage = chatResp.Usage
		}
		events <- doneEvent
	}()

	return events, nil
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

	// Empty body with non-SSE Content-Type is a server-side anomaly
	// (e.g., transient gateway error returning 200 with empty body, or
	// stale connection after sustained API usage during compaction).
	// Close idle connections to force new TCP connections on retry.
	if len(body) == 0 {
		// Dump ALL response headers for diagnosis
		allHeaders := make(map[string]string)
		for key, vals := range resp.Header {
			allHeaders[key] = strings.Join(vals, "; ")
		}
		headersJSON, _ := json.Marshal(allHeaders)

		logger.Warn().
			Int("status", resp.StatusCode).
			Str("proto", resp.Proto).
			Bool("close", resp.Close).
			Int64("content_length_raw", resp.ContentLength).
			Str("all_headers", string(headersJSON)).
			Msg("MiniMax returned empty body — full response details")

		// Reset connection pool — stale connections after long compaction
		// sessions may be silently broken by upstream CDN/gateways.
		p.streamClient.CloseIdleConnections()
		p.httpClient.CloseIdleConnections()

		return nil, &provider.ProviderError{
			Code:      provider.ErrCodeServiceUnavailable,
			Message:   fmt.Sprintf("MiniMax returned empty response (HTTP 200, Content-Length: 0, X-Session-Id: %s) — likely ALB session rejection after sustained API usage", resp.Header.Get("X-Session-Id")),
			Provider:  "minimax",
			Retryable: true,
		}
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
			if chatResp.Choices[0].Message.Content != nil {
				content = *chatResp.Choices[0].Message.Content
			}
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
			Role: msg.Role,
		}

		// MiniMax requires content to be null (not empty string "") for
		// assistant messages that only carry tool_calls.
		// Go's string type can't represent null, so we use *string.
		if msg.Content != "" {
			chatMsg.Content = strPtr(msg.Content)
		} else if msg.Role != "assistant" || len(msg.ToolCalls) == 0 {
			// Non-assistant messages or assistant without tool calls: keep empty string
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
						if chatReq.Messages[i].Content != nil {
							*chatReq.Messages[i].Content += contentText
						} else {
							chatReq.Messages[i].Content = strPtr(contentText)
						}
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

	logger.Debug().Int("request_bytes", reqSize).Str("url", url).Msg("MiniMax doStreamRequest sending")

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
		Str("content_length", resp.Header.Get("Content-Length")).
		Int("request_bytes", reqSize).
		Msg("MiniMax doStreamRequest response")

	return resp, nil
}

// handleErrorResponse converts an error response to an appropriate error.
func (p *MinimaxProvider) handleErrorResponse(statusCode int, body []byte) error {
	var errResp chatResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != nil {
		msg := errResp.Error.Message
		// Check for context window exceeded before switch
		lowerMsg := strings.ToLower(msg)
		if strings.Contains(lowerMsg, "context window") ||
			strings.Contains(lowerMsg, "context length") ||
			strings.Contains(lowerMsg, "too many tokens") {
			return &provider.ProviderError{
				Code:      provider.ErrCodeContextWindowExceeded,
				Message:   msg,
				Provider:  "minimax",
				Retryable: true,
			}
		}

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
				Retryable: false,
			}
		case http.StatusBadRequest:
			// 400 errors are client-side (invalid params), retrying won't help
			return &provider.ProviderError{
				Code:      provider.ErrCodeInvalidRequest,
				Message:   fmt.Sprintf("[%s] %s", errResp.Error.Type, msg),
				Provider:  "minimax",
				Retryable: false,
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

		// MiniMax-M2.5 sometimes outputs tool calls as raw XML in content
		// (e.g. <invoke name="read_file"><parameter name="path">...</parameter></invoke>)
		// instead of using structured tool_calls. Detect and extract them.
		if len(result.ToolCalls) == 0 && result.Content != "" {
			if xmlCalls, cleanContent := ExtractXMLToolCalls(result.Content); len(xmlCalls) > 0 {
				logger.Warn().
					Int("extracted_calls", len(xmlCalls)).
					Msg("MiniMax: extracted tool calls from XML in content")
				result.ToolCalls = xmlCalls
				result.Content = cleanContent
				result.FinishReason = provider.FinishReasonToolCalls
			}
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

// xmlInvokePattern matches MiniMax's XML tool call format.
// Handles both <invoke>...</invoke> and <invoke>...</invoke></minimax:tool_call>
var xmlInvokePattern = regexp.MustCompile(`(?s)<invoke\s+name="([^"]+)">\s*((?:<parameter\s+name="[^"]*">[^<]*</parameter>\s*)*)</invoke>(?:\s*</minimax:tool_call>)?`)

// xmlParamPattern matches individual parameter elements.
var xmlParamPattern = regexp.MustCompile(`<parameter\s+name="([^"]*)">(.*?)</parameter>`)

// ExtractXMLToolCalls parses raw XML tool calls from model content.
// Returns extracted tool calls and the cleaned content with XML removed.
func ExtractXMLToolCalls(content string) ([]provider.ToolCall, string) {
	matches := xmlInvokePattern.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return nil, content
	}

	var calls []provider.ToolCall
	for i, match := range matches {
		fullMatch := content[match[0]:match[1]]
		name := content[match[2]:match[3]]
		paramsBlock := content[match[4]:match[5]]

		// Parse parameters into JSON
		params := make(map[string]interface{})
		paramMatches := xmlParamPattern.FindAllStringSubmatch(paramsBlock, -1)
		for _, pm := range paramMatches {
			if len(pm) >= 3 {
				params[pm[1]] = pm[2]
			}
		}

		argsJSON, err := json.Marshal(params)
		if err != nil {
			logger.Warn().Err(err).Str("tool", name).Msg("failed to marshal XML tool call params")
			continue
		}

		calls = append(calls, provider.ToolCall{
			ID:        fmt.Sprintf("xmlcall_%d", i),
			Type:      "function",
			Name:      name,
			Arguments: string(argsJSON),
		})

		_ = fullMatch // used via match indices
	}

	if len(calls) == 0 {
		return nil, content
	}

	// Remove XML tool call blocks from content, also remove [tool] markers
	cleaned := xmlInvokePattern.ReplaceAllString(content, "")
	cleaned = strings.ReplaceAll(cleaned, "[tool]", "")
	// Also remove any <minimax:tool_call> opening tags
	cleaned = regexp.MustCompile(`<minimax:tool_call[^>]*>`).ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)

	return calls, cleaned
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
