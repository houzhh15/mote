package memory

import (
	"context"
	"hash/fnv"
	"math"
)

// SimpleEmbedder is a deterministic embedder for development and testing.
// It generates pseudo-random vectors based on text hashing.
type SimpleEmbedder struct {
	dims int
}

// NewSimpleEmbedder creates a new SimpleEmbedder with the specified dimensions.
func NewSimpleEmbedder(dims int) *SimpleEmbedder {
	if dims <= 0 {
		dims = 384
	}
	return &SimpleEmbedder{dims: dims}
}

// Embed generates a deterministic embedding vector for text.
// The same text will always produce the same vector.
func (e *SimpleEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if text == "" {
		// Return zero vector for empty text
		return make([]float32, e.dims), nil
	}

	// Use FNV-1a hash for deterministic seed
	h := fnv.New64a()
	h.Write([]byte(text))
	seed := h.Sum64()

	// Generate pseudo-random vector from seed
	vec := make([]float32, e.dims)
	for i := 0; i < e.dims; i++ {
		// Simple LCG-like deterministic "random" number
		seed = seed*6364136223846793005 + 1442695040888963407
		// Convert to float32 in range [-1, 1]
		vec[i] = float32(int64(seed>>32)&0x7FFFFFFF)/float32(0x7FFFFFFF)*2 - 1
	}

	// Normalize to unit length
	e.normalize(vec)

	return vec, nil
}

// EmbedBatch generates embedding vectors for multiple texts.
func (e *SimpleEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := e.Embed(ctx, text)
		if err != nil {
			return nil, err
		}
		results[i] = vec
	}
	return results, nil
}

// Dimensions returns the embedding dimension.
func (e *SimpleEmbedder) Dimensions() int {
	return e.dims
}

// normalize normalizes a vector to unit length.
func (e *SimpleEmbedder) normalize(vec []float32) {
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return
	}
	norm := math.Sqrt(sum)
	for i := range vec {
		vec[i] = float32(float64(vec[i]) / norm)
	}
}
