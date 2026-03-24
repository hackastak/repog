package provider

import (
	"fmt"

	"github.com/hackastak/repog/internal/config"
)

// EmbeddingProviderFactory is a function that creates an EmbeddingProvider
type EmbeddingProviderFactory func(apiKey, model string, dimensions int) (EmbeddingProvider, error)

// LLMProviderFactory is a function that creates an LLMProvider
type LLMProviderFactory func(apiKey, model, fallbackModel string) (LLMProvider, error)

var (
	embeddingProviders = make(map[string]EmbeddingProviderFactory)
	llmProviders       = make(map[string]LLMProviderFactory)
)

// RegisterEmbeddingProvider registers an embedding provider factory
func RegisterEmbeddingProvider(name string, factory EmbeddingProviderFactory) {
	embeddingProviders[name] = factory
}

// RegisterLLMProvider registers an LLM provider factory
func RegisterLLMProvider(name string, factory LLMProviderFactory) {
	llmProviders[name] = factory
}

// NewEmbeddingProvider creates an EmbeddingProvider based on config
func NewEmbeddingProvider(cfg config.ProviderConfig, apiKey string) (EmbeddingProvider, error) {
	factory, ok := embeddingProviders[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("unknown embedding provider: %s", cfg.Provider)
	}
	return factory(apiKey, cfg.Model, cfg.Dimensions)
}

// NewLLMProvider creates an LLMProvider based on config
func NewLLMProvider(cfg config.ProviderConfig, apiKey string) (LLMProvider, error) {
	factory, ok := llmProviders[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("unknown LLM provider: %s", cfg.Provider)
	}
	return factory(apiKey, cfg.Model, cfg.Fallback)
}
