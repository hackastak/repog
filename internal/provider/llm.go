package provider

import "context"

// LLMProvider defines the interface for LLM operations
type LLMProvider interface {
	// Name returns the provider name
	Name() string

	// Call makes a non-streaming LLM request
	Call(ctx context.Context, req LLMRequest) (*LLMResult, *LLMError)

	// Stream makes a streaming LLM request, calling onChunk for each token
	Stream(ctx context.Context, req LLMRequest, onChunk func(string)) (*LLMResult, *LLMError)

	// Validate tests the provider connection/credentials
	Validate(ctx context.Context) error
}
