package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// TokenProvider provides authentication tokens for the embedding API.
type TokenProvider interface {
	// GetToken returns a valid token for API authentication.
	GetToken(ctx context.Context) (string, error)
}

// CopilotEmbedder implements Embedder using OpenAI-compatible embedding API.
// It can work with Copilot tokens or direct OpenAI API keys.
type CopilotEmbedder struct {
	tokenProvider TokenProvider
	httpClient    *http.Client
	model         string
	dimensions    int
	batchSize     int
	timeout       time.Duration
	baseURL       string
	logger        zerolog.Logger
}

// CopilotEmbedderOptions holds configuration for CopilotEmbedder.
type CopilotEmbedderOptions struct {
	TokenProvider TokenProvider
	Model         string        // Default: "text-embedding-3-small"
	Dimensions    int           // Default: 1536, can be reduced to 384
	BatchSize     int           // Default: 100
	Timeout       time.Duration // Default: 30s
	BaseURL       string        // Default: "https://api.openai.com"
	Logger        zerolog.Logger
}

// NewCopilotEmbedder creates a new CopilotEmbedder with the given options.
func NewCopilotEmbedder(opts CopilotEmbedderOptions) (*CopilotEmbedder, error) {
	if opts.TokenProvider == nil {
		return nil, errors.New("token provider is required")
	}

	// Apply defaults
	if opts.Model == "" {
		opts.Model = "text-embedding-3-small"
	}
	if opts.Dimensions <= 0 {
		opts.Dimensions = 1536
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 100
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.BaseURL == "" {
		opts.BaseURL = "https://api.openai.com"
	}

	return &CopilotEmbedder{
		tokenProvider: opts.TokenProvider,
		httpClient: &http.Client{
			Timeout: opts.Timeout,
		},
		model:      opts.Model,
		dimensions: opts.Dimensions,
		batchSize:  opts.BatchSize,
		timeout:    opts.Timeout,
		baseURL:    opts.BaseURL,
		logger:     opts.Logger,
	}, nil
}

// embeddingRequest represents the request body for the embedding API.
type embeddingRequest struct {
	Model      string `json:"model"`
	Input      any    `json:"input"`
	Dimensions int    `json:"dimensions,omitempty"`
}

// embeddingResponse represents the response from the embedding API.
type embeddingResponse struct {
	Object string             `json:"object"`
	Data   []embeddingData    `json:"data"`
	Model  string             `json:"model"`
	Usage  embeddingUsage     `json:"usage"`
	Error  *embeddingAPIError `json:"error,omitempty"`
}

// embeddingData represents a single embedding result.
type embeddingData struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

// embeddingUsage represents token usage information.
type embeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// embeddingAPIError represents an API error response.
type embeddingAPIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// Embed generates an embedding vector for a single text.
func (e *CopilotEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := e.embedInternal(ctx, text)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("embedding response contains no data")
	}
	return results[0], nil
}

// EmbedBatch generates embedding vectors for multiple texts.
// It automatically splits large batches according to batchSize.
func (e *CopilotEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	var results [][]float32

	for i := 0; i < len(texts); i += e.batchSize {
		end := i + e.batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		embeddings, err := e.embedInternal(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("batch %d-%d: %w", i, end, err)
		}
		results = append(results, embeddings...)
	}

	return results, nil
}

// Dimensions returns the embedding dimension.
func (e *CopilotEmbedder) Dimensions() int {
	return e.dimensions
}

// embedInternal calls the embedding API with either a single string or a batch.
func (e *CopilotEmbedder) embedInternal(ctx context.Context, input any) ([][]float32, error) {
	// Get authentication token
	token, err := e.tokenProvider.GetToken(ctx)
	if err != nil {
		return nil, &MemoryError{
			Op:  "get_token",
			Err: fmt.Errorf("%w: %v", ErrTokenExpired, err),
		}
	}

	// Build request
	reqBody := embeddingRequest{
		Model: e.model,
		Input: input,
	}
	// Only set dimensions if using a model that supports it
	if e.model == "text-embedding-3-small" || e.model == "text-embedding-3-large" {
		reqBody.Dimensions = e.dimensions
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.baseURL+"/v1/embeddings",
		bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, &MemoryError{
			Op:  "http_request",
			Err: fmt.Errorf("%w: %v", ErrEmbeddingFailed, err),
		}
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Handle error status codes
	if resp.StatusCode != http.StatusOK {
		var apiResp embeddingResponse
		if json.Unmarshal(body, &apiResp) == nil && apiResp.Error != nil {
			// Handle specific error types
			switch {
			case resp.StatusCode == http.StatusTooManyRequests:
				return nil, &MemoryError{
					Op:  "embedding",
					Err: fmt.Errorf("%w: %s", ErrRateLimited, apiResp.Error.Message),
				}
			case resp.StatusCode == http.StatusUnauthorized:
				return nil, &MemoryError{
					Op:  "embedding",
					Err: fmt.Errorf("%w: %s", ErrTokenExpired, apiResp.Error.Message),
				}
			default:
				return nil, &MemoryError{
					Op:  "embedding",
					Err: fmt.Errorf("%w: [%d] %s", ErrEmbeddingFailed, resp.StatusCode, apiResp.Error.Message),
				}
			}
		}
		return nil, &MemoryError{
			Op:  "embedding",
			Err: fmt.Errorf("%w: status %d", ErrEmbeddingFailed, resp.StatusCode),
		}
	}

	// Parse successful response
	var apiResp embeddingResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// Extract embeddings in order
	embeddings := make([][]float32, len(apiResp.Data))
	for _, data := range apiResp.Data {
		if data.Index < 0 || data.Index >= len(embeddings) {
			return nil, fmt.Errorf("invalid embedding index: %d", data.Index)
		}
		embeddings[data.Index] = data.Embedding
	}

	e.logger.Debug().
		Int("count", len(embeddings)).
		Int("promptTokens", apiResp.Usage.PromptTokens).
		Msg("embedding completed")

	return embeddings, nil
}

// Compile-time interface check
var _ Embedder = (*CopilotEmbedder)(nil)
