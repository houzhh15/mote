package memory

import "context"

// Embedder defines the interface for generating text embeddings.
type Embedder interface {
	// Embed generates an embedding vector for a single text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch generates embedding vectors for multiple texts.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the dimension of the embedding vectors.
	Dimensions() int
}
