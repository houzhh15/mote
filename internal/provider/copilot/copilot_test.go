package copilot

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"mote/internal/provider"
)

func TestNewCopilotProvider(t *testing.T) {
	p := NewCopilotProvider("test-token", "", 0)

	if p == nil {
		t.Fatal("NewCopilotProvider returned nil")
	}

	if p.Name() != "copilot" {
		t.Errorf("Name() = %s, want copilot", p.Name())
	}
}

func TestCopilotProvider_Models(t *testing.T) {
	p := NewCopilotProvider("test-token", "", 0)
	models := p.Models()

	if len(models) == 0 {
		t.Error("Models() returned empty list")
	}

	// Check for expected API models (free models only)
	expectedModels := []string{"gpt-4.1", "gpt-4o", "gpt-5-mini", "grok-code-fast-1"}
	modelSet := make(map[string]bool)
	for _, m := range models {
		modelSet[m] = true
	}

	for _, expected := range expectedModels {
		if !modelSet[expected] {
			t.Errorf("Models() missing expected API model: %s", expected)
		}
	}

	// ACP models should NOT be in CopilotProvider.Models()
	acpModels := []string{"claude-sonnet-4.5", "claude-opus-4.6"}
	for _, acpModel := range acpModels {
		if modelSet[acpModel] {
			t.Errorf("Models() should not contain ACP model: %s", acpModel)
		}
	}
}

func TestCopilotProvider_Chat(t *testing.T) {
	tokenCallCount := 0
	chatCallCount := 0

	transport := &mockTransport{
		handler: func(req *http.Request) (*http.Response, error) {
			// Handle token request
			if req.URL.Host == "api.github.com" {
				tokenCallCount++
				resp := &tokenResponse{
					Token:     "copilot-token",
					ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
				}
				resp.Endpoints.API = "https://test.api.com"
				body, _ := json.Marshal(resp)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       &mockBody{data: body},
					Header:     make(http.Header),
				}, nil
			}

			// Handle chat request
			chatCallCount++
			resp := &chatCompletionResponse{
				ID:    "resp-123",
				Model: "test-model",
				Choices: []struct {
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
				}{
					{
						Index: 0,
						Message: struct {
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
						}{
							Role:    "assistant",
							Content: "Hello, how can I help?",
						},
						FinishReason: "stop",
					},
				},
			}
			body, _ := json.Marshal(resp)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       &mockBody{data: body},
				Header:     make(http.Header),
			}, nil
		},
	}

	httpClient := &http.Client{Transport: transport}

	tm := NewTokenManager("test-token")
	tm.SetHTTPClient(httpClient)

	p := &CopilotProvider{
		tokenManager: tm,
		httpClient:   httpClient,
		model:        DefaultModel,
		maxTokens:    DefaultMaxTokens,
	}

	ctx := context.Background()
	req := provider.ChatRequest{
		Messages: []provider.Message{
			{Role: "user", Content: "Hello"},
		},
	}

	resp, err := p.Chat(ctx, req)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if resp.Content != "Hello, how can I help?" {
		t.Errorf("Content = %s, want 'Hello, how can I help?'", resp.Content)
	}

	if tokenCallCount == 0 {
		t.Error("token API was not called")
	}
	if chatCallCount == 0 {
		t.Error("chat API was not called")
	}
}

func TestCopilotProvider_ChatUnauthorized(t *testing.T) {
	p := &CopilotProvider{
		tokenManager: NewTokenManager("invalid-token"),
		httpClient: &http.Client{
			Transport: &mockTransport{
				handler: func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusUnauthorized,
						Body:       &mockBody{data: []byte(`{"error":"unauthorized"}`)},
						Header:     make(http.Header),
					}, nil
				},
			},
		},
		model:     DefaultModel,
		maxTokens: DefaultMaxTokens,
	}

	ctx := context.Background()
	req := provider.ChatRequest{
		Messages: []provider.Message{
			{Role: "user", Content: "Hello"},
		},
	}

	_, err := p.Chat(ctx, req)
	if err == nil {
		t.Error("expected error for unauthorized request")
	}
}

func TestCopilotProvider_ConvertMessages(t *testing.T) {
	p := &CopilotProvider{}

	messages := []provider.Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there", ToolCalls: []provider.ToolCall{
			{ID: "call_1", Name: "test", Arguments: "{}"},
		}},
		{Role: "tool", Content: "result", ToolCallID: "call_1"},
	}

	result := p.convertMessages(messages)

	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result))
	}

	// Check system message
	if result[0]["role"] != "system" {
		t.Errorf("message 0 role = %v, want system", result[0]["role"])
	}

	// Check tool calls
	if _, ok := result[2]["tool_calls"]; !ok {
		t.Error("message 2 missing tool_calls")
	}

	// Check tool_call_id
	if result[3]["tool_call_id"] != "call_1" {
		t.Errorf("message 3 tool_call_id = %v, want call_1", result[3]["tool_call_id"])
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"unauthorized", ErrUnauthorized, true},
		{"rate_limited", ErrRateLimited, true},
		{"service_unavailable", ErrServiceUnavailable, true},
		{"token_expired", ErrTokenExpired, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableError(tt.err)
			if got != tt.want {
				t.Errorf("isRetryableError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}

	// Test nil separately
	if isRetryableError(nil) {
		t.Error("isRetryableError(nil) = true, want false")
	}
}
