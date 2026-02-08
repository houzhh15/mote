package memory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
)

// mockTokenProvider implements TokenProvider for testing.
type mockTokenProvider struct {
	token string
	err   error
}

func (m *mockTokenProvider) GetToken(ctx context.Context) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.token, nil
}

func TestCopilotEmbedder_NewCopilotEmbedder(t *testing.T) {
	t.Run("requires token provider", func(t *testing.T) {
		_, err := NewCopilotEmbedder(CopilotEmbedderOptions{})
		if err == nil {
			t.Error("expected error when token provider is nil")
		}
	})

	t.Run("applies defaults", func(t *testing.T) {
		tp := &mockTokenProvider{token: "test-token"}
		e, err := NewCopilotEmbedder(CopilotEmbedderOptions{
			TokenProvider: tp,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if e.model != "text-embedding-3-small" {
			t.Errorf("expected model 'text-embedding-3-small', got '%s'", e.model)
		}
		if e.dimensions != 1536 {
			t.Errorf("expected dimensions 1536, got %d", e.dimensions)
		}
		if e.batchSize != 100 {
			t.Errorf("expected batchSize 100, got %d", e.batchSize)
		}
	})

	t.Run("respects custom options", func(t *testing.T) {
		tp := &mockTokenProvider{token: "test-token"}
		e, err := NewCopilotEmbedder(CopilotEmbedderOptions{
			TokenProvider: tp,
			Model:         "text-embedding-ada-002",
			Dimensions:    384,
			BatchSize:     50,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if e.model != "text-embedding-ada-002" {
			t.Errorf("expected model 'text-embedding-ada-002', got '%s'", e.model)
		}
		if e.dimensions != 384 {
			t.Errorf("expected dimensions 384, got %d", e.dimensions)
		}
		if e.batchSize != 50 {
			t.Errorf("expected batchSize 50, got %d", e.batchSize)
		}
	})
}

func TestCopilotEmbedder_Embed(t *testing.T) {
	t.Run("successful single embedding", func(t *testing.T) {
		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/v1/embeddings" {
				t.Errorf("expected /v1/embeddings, got %s", r.URL.Path)
			}
			if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
				t.Errorf("expected 'Bearer test-token', got '%s'", auth)
			}

			// Return mock response
			resp := embeddingResponse{
				Object: "list",
				Data: []embeddingData{
					{
						Object:    "embedding",
						Index:     0,
						Embedding: []float32{0.1, 0.2, 0.3},
					},
				},
				Usage: embeddingUsage{
					PromptTokens: 5,
					TotalTokens:  5,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		tp := &mockTokenProvider{token: "test-token"}
		e, err := NewCopilotEmbedder(CopilotEmbedderOptions{
			TokenProvider: tp,
			BaseURL:       server.URL,
			Logger:        zerolog.Nop(),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		embedding, err := e.Embed(context.Background(), "hello world")
		if err != nil {
			t.Fatalf("Embed failed: %v", err)
		}

		if len(embedding) != 3 {
			t.Errorf("expected 3 dimensions, got %d", len(embedding))
		}
		if embedding[0] != 0.1 || embedding[1] != 0.2 || embedding[2] != 0.3 {
			t.Errorf("unexpected embedding values: %v", embedding)
		}
	})

	t.Run("handles rate limit error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			resp := embeddingResponse{
				Error: &embeddingAPIError{
					Message: "Rate limit exceeded",
					Type:    "rate_limit_error",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		tp := &mockTokenProvider{token: "test-token"}
		e, _ := NewCopilotEmbedder(CopilotEmbedderOptions{
			TokenProvider: tp,
			BaseURL:       server.URL,
			Logger:        zerolog.Nop(),
		})

		_, err := e.Embed(context.Background(), "test")
		if err == nil {
			t.Error("expected error for rate limit")
		}

		var memErr *MemoryError
		if !errors.As(err, &memErr) {
			t.Errorf("expected MemoryError, got %T", err)
		}
	})

	t.Run("handles unauthorized error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			resp := embeddingResponse{
				Error: &embeddingAPIError{
					Message: "Invalid API key",
					Type:    "invalid_request_error",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		tp := &mockTokenProvider{token: "invalid-token"}
		e, _ := NewCopilotEmbedder(CopilotEmbedderOptions{
			TokenProvider: tp,
			BaseURL:       server.URL,
			Logger:        zerolog.Nop(),
		})

		_, err := e.Embed(context.Background(), "test")
		if err == nil {
			t.Error("expected error for unauthorized")
		}
	})
}

func TestCopilotEmbedder_EmbedBatch(t *testing.T) {
	t.Run("successful batch embedding", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req embeddingRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			// Handle batch input
			inputs, ok := req.Input.([]any)
			if !ok {
				t.Error("expected array input for batch")
				return
			}

			// Return embeddings for each input
			data := make([]embeddingData, len(inputs))
			for i := range inputs {
				data[i] = embeddingData{
					Object:    "embedding",
					Index:     i,
					Embedding: []float32{float32(i) * 0.1, float32(i) * 0.2},
				}
			}

			resp := embeddingResponse{
				Object: "list",
				Data:   data,
				Usage:  embeddingUsage{PromptTokens: len(inputs) * 5, TotalTokens: len(inputs) * 5},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		tp := &mockTokenProvider{token: "test-token"}
		e, _ := NewCopilotEmbedder(CopilotEmbedderOptions{
			TokenProvider: tp,
			BaseURL:       server.URL,
			BatchSize:     10,
			Logger:        zerolog.Nop(),
		})

		texts := []string{"hello", "world", "test"}
		embeddings, err := e.EmbedBatch(context.Background(), texts)
		if err != nil {
			t.Fatalf("EmbedBatch failed: %v", err)
		}

		if len(embeddings) != 3 {
			t.Errorf("expected 3 embeddings, got %d", len(embeddings))
		}
	})

	t.Run("splits large batches", func(t *testing.T) {
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			var req embeddingRequest
			_ = json.NewDecoder(r.Body).Decode(&req)

			inputs, _ := req.Input.([]any)
			data := make([]embeddingData, len(inputs))
			for i := range inputs {
				data[i] = embeddingData{
					Object:    "embedding",
					Index:     i,
					Embedding: []float32{0.1},
				}
			}

			resp := embeddingResponse{Object: "list", Data: data}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		tp := &mockTokenProvider{token: "test-token"}
		e, _ := NewCopilotEmbedder(CopilotEmbedderOptions{
			TokenProvider: tp,
			BaseURL:       server.URL,
			BatchSize:     2, // Small batch size for testing
			Logger:        zerolog.Nop(),
		})

		// 5 texts with batch size 2 should make 3 API calls
		texts := []string{"a", "b", "c", "d", "e"}
		embeddings, err := e.EmbedBatch(context.Background(), texts)
		if err != nil {
			t.Fatalf("EmbedBatch failed: %v", err)
		}

		if len(embeddings) != 5 {
			t.Errorf("expected 5 embeddings, got %d", len(embeddings))
		}
		if callCount != 3 {
			t.Errorf("expected 3 API calls, got %d", callCount)
		}
	})

	t.Run("empty batch returns nil", func(t *testing.T) {
		tp := &mockTokenProvider{token: "test-token"}
		e, _ := NewCopilotEmbedder(CopilotEmbedderOptions{
			TokenProvider: tp,
			Logger:        zerolog.Nop(),
		})

		embeddings, err := e.EmbedBatch(context.Background(), []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if embeddings != nil {
			t.Errorf("expected nil for empty batch, got %v", embeddings)
		}
	})
}

func TestCopilotEmbedder_Dimensions(t *testing.T) {
	tp := &mockTokenProvider{token: "test-token"}
	e, _ := NewCopilotEmbedder(CopilotEmbedderOptions{
		TokenProvider: tp,
		Dimensions:    768,
	})

	if e.Dimensions() != 768 {
		t.Errorf("expected Dimensions() = 768, got %d", e.Dimensions())
	}
}
