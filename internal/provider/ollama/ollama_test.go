package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mote/internal/provider"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaProvider_Name(t *testing.T) {
	p := NewOllamaProvider(DefaultConfig())
	assert.Equal(t, "ollama", p.Name())
}

func TestOllamaProvider_Chat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/chat", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req ollamaRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "test-model", req.Model)
		assert.False(t, req.Stream)
		assert.Len(t, req.Messages, 1)
		assert.Equal(t, "user", req.Messages[0].Role)
		assert.Equal(t, "Hello", req.Messages[0].Content)

		resp := ollamaResponse{
			Model:           "test-model",
			CreatedAt:       time.Now().Format(time.RFC3339),
			Message:         ollamaMessage{Role: "assistant", Content: "Hello! How can I help you?"},
			Done:            true,
			PromptEvalCount: 5,
			EvalCount:       10,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOllamaProvider(Config{
		Endpoint: server.URL,
		Model:    "test-model",
		Timeout:  10 * time.Second,
	})

	resp, err := p.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{{Role: "user", Content: "Hello"}},
	})

	require.NoError(t, err)
	assert.Equal(t, "Hello! How can I help you?", resp.Content)
	assert.Equal(t, provider.FinishReasonStop, resp.FinishReason)
	require.NotNil(t, resp.Usage)
	assert.Equal(t, 5, resp.Usage.PromptTokens)
	assert.Equal(t, 10, resp.Usage.CompletionTokens)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
}

func TestOllamaProvider_ChatWithTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollamaRequest
		json.NewDecoder(r.Body).Decode(&req)

		assert.Len(t, req.Tools, 1)
		assert.Equal(t, "function", req.Tools[0].Type)
		assert.Equal(t, "get_weather", req.Tools[0].Function.Name)

		resp := ollamaResponse{
			Model: "test-model",
			Message: ollamaMessage{
				Role:    "assistant",
				Content: "",
				ToolCalls: []ollamaToolCall{{
					ID:   "call_123",
					Type: "function",
					Function: struct {
						Name      string                 `json:"name"`
						Arguments map[string]interface{} `json:"arguments"`
					}{Name: "get_weather", Arguments: map[string]interface{}{"location": "San Francisco"}},
				}},
			},
			Done: true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOllamaProvider(Config{Endpoint: server.URL, Model: "test-model"})

	resp, err := p.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{{Role: "user", Content: "What's the weather?"}},
		Tools: []provider.Tool{{
			Type: "function",
			Function: provider.ToolFunction{
				Name:        "get_weather",
				Description: "Get the weather",
				Parameters:  json.RawMessage(`{"type":"object"}`),
			},
		}},
	})

	require.NoError(t, err)
	assert.Equal(t, provider.FinishReasonToolCalls, resp.FinishReason)
	require.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, "call_123", resp.ToolCalls[0].ID)
	assert.Equal(t, "get_weather", resp.ToolCalls[0].Name)
}

func TestOllamaProvider_ChatError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		response   string
		wantErr    error
	}{
		{
			name:       "model not found",
			statusCode: http.StatusNotFound,
			response:   `{"error": "model 'unknown' not found"}`,
			wantErr:    ErrModelNotFound,
		},
		{
			name:       "server error",
			statusCode: http.StatusInternalServerError,
			response:   `{"error": "internal error"}`,
			wantErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			p := NewOllamaProvider(Config{Endpoint: server.URL})

			_, err := p.Chat(context.Background(), provider.ChatRequest{
				Messages: []provider.Message{{Role: "user", Content: "test"}},
			})

			require.Error(t, err)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}

func TestOllamaProvider_Models(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			resp := ollamaModelsResponse{
				Models: []ollamaModelInfo{
					{Name: "llama3.2:latest", Size: 1000000},
					{Name: "mistral:latest", Size: 2000000},
					{Name: "codellama:7b", Size: 3000000},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := NewOllamaProvider(Config{Endpoint: server.URL})
	models := p.Models()

	assert.Len(t, models, 3)
	assert.Contains(t, models, "llama3.2:latest")
	assert.Contains(t, models, "mistral:latest")
	assert.Contains(t, models, "codellama:7b")
}

func TestOllamaProvider_ConnectionFailed(t *testing.T) {
	p := NewOllamaProvider(Config{
		Endpoint: "http://localhost:99999",
		Timeout:  1 * time.Second,
	})

	_, err := p.Chat(context.Background(), provider.ChatRequest{
		Messages: []provider.Message{{Role: "user", Content: "test"}},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrConnectionFailed)
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, DefaultEndpoint, cfg.Endpoint)
	assert.Equal(t, DefaultModel, cfg.Model)
	assert.Equal(t, DefaultTimeout, cfg.Timeout)
	assert.Equal(t, DefaultKeepAlive, cfg.KeepAlive)
}

func TestBuildRequest(t *testing.T) {
	p := &OllamaProvider{model: "default-model", keepAlive: "5m"}

	req := provider.ChatRequest{
		Model: "custom-model",
		Messages: []provider.Message{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Hello"},
		},
		Temperature: 0.7,
		MaxTokens:   100,
	}

	ollamaReq := p.buildRequest(req, true)

	assert.Equal(t, "custom-model", ollamaReq.Model)
	assert.True(t, ollamaReq.Stream)
	assert.Equal(t, "5m", ollamaReq.KeepAlive)
	assert.Len(t, ollamaReq.Messages, 2)
	require.NotNil(t, ollamaReq.Options)
	assert.Equal(t, 0.7, ollamaReq.Options.Temperature)
	assert.Equal(t, 100, ollamaReq.Options.NumPredict)
}

func TestBuildRequest_DefaultModel(t *testing.T) {
	p := &OllamaProvider{model: "default-model", keepAlive: "5m"}

	req := provider.ChatRequest{
		Model:    "",
		Messages: []provider.Message{{Role: "user", Content: "Hello"}},
	}

	ollamaReq := p.buildRequest(req, false)
	assert.Equal(t, "default-model", ollamaReq.Model)
}
