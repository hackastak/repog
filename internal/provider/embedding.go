package provider

import "context"

// EmbeddingProvider defines the interface for embedding operations
type EmbeddingProvider interface {
	// Name returns the provider name (e.g., "gemini", "ollama")
	Name() string

	// Dimensions returns the embedding vector dimensions
	Dimensions() int

	// BatchSize returns the maximum batch size for embedding requests
	BatchSize() int

	// EmbedChunks embeds multiple chunks in a batch
	EmbedChunks(ctx context.Context, chunks []EmbedRequest) BatchEmbedResult

	// EmbedQuery embeds a single query for search
	EmbedQuery(ctx context.Context, query string) []float32

	// Validate tests the provider connection/credentials
	Validate(ctx context.Context) error
}
