package provider

import (
	"context"
)

// MockEmbeddingProvider is a test double for EmbeddingProvider
type MockEmbeddingProvider struct {
	NameVal       string
	DimensionsVal int
	BatchSizeVal  int
	MaxTokensVal  int
	EmbedFunc     func(ctx context.Context, chunks []EmbedRequest) BatchEmbedResult
	QueryFunc     func(ctx context.Context, query string) []float32
}

func (m *MockEmbeddingProvider) Name() string                     { return m.NameVal }
func (m *MockEmbeddingProvider) Dimensions() int                  { return m.DimensionsVal }
func (m *MockEmbeddingProvider) BatchSize() int                   { return m.BatchSizeVal }
func (m *MockEmbeddingProvider) MaxTokens() int                   { return m.MaxTokensVal }
func (m *MockEmbeddingProvider) Validate(_ context.Context) error { return nil }

func (m *MockEmbeddingProvider) EmbedChunks(ctx context.Context, chunks []EmbedRequest) BatchEmbedResult {
	if m.EmbedFunc != nil {
		return m.EmbedFunc(ctx, chunks)
	}
	// Default: return 768-dim embeddings for each chunk
	results := make([]EmbedResult, len(chunks))
	for i, c := range chunks {
		embedding := make([]float32, m.DimensionsVal)
		for j := range embedding {
			embedding[j] = 0.5
		}
		results[i] = EmbedResult{ID: c.ID, Embedding: embedding}
	}
	return BatchEmbedResult{Results: results}
}

func (m *MockEmbeddingProvider) EmbedQuery(ctx context.Context, query string) []float32 {
	if m.QueryFunc != nil {
		return m.QueryFunc(ctx, query)
	}
	// Default: return 768-dim embedding
	embedding := make([]float32, m.DimensionsVal)
	for i := range embedding {
		embedding[i] = 0.5
	}
	return embedding
}

// NewMockEmbeddingProvider creates a mock embedding provider with sensible defaults
func NewMockEmbeddingProvider() *MockEmbeddingProvider {
	return &MockEmbeddingProvider{
		NameVal:       "mock",
		DimensionsVal: 768,
		BatchSizeVal:  20,
		MaxTokensVal:  8192,
	}
}

// MockLLMProvider is a test double for LLMProvider
type MockLLMProvider struct {
	NameVal    string
	CallFunc   func(ctx context.Context, req LLMRequest) (*LLMResult, *LLMError)
	StreamFunc func(ctx context.Context, req LLMRequest, onChunk func(string)) (*LLMResult, *LLMError)
}

func (m *MockLLMProvider) Name() string                     { return m.NameVal }
func (m *MockLLMProvider) Validate(_ context.Context) error { return nil }

func (m *MockLLMProvider) Call(ctx context.Context, req LLMRequest) (*LLMResult, *LLMError) {
	if m.CallFunc != nil {
		return m.CallFunc(ctx, req)
	}
	return &LLMResult{Text: "Mock response", InputTokens: 100, OutputTokens: 50}, nil
}

func (m *MockLLMProvider) Stream(ctx context.Context, req LLMRequest, onChunk func(string)) (*LLMResult, *LLMError) {
	if m.StreamFunc != nil {
		return m.StreamFunc(ctx, req, onChunk)
	}
	// Default: stream a simple response
	if onChunk != nil {
		onChunk("Mock ")
		onChunk("response")
	}
	return &LLMResult{Text: "Mock response", InputTokens: 100, OutputTokens: 50}, nil
}

// NewMockLLMProvider creates a mock LLM provider with sensible defaults
func NewMockLLMProvider() *MockLLMProvider {
	return &MockLLMProvider{
		NameVal: "mock",
	}
}
