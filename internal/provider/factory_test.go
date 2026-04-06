package provider_test

import (
	"testing"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/provider"
	_ "github.com/hackastak/repog/internal/provider/gemini"
	_ "github.com/hackastak/repog/internal/provider/ollama"
	_ "github.com/hackastak/repog/internal/provider/openai"
	_ "github.com/hackastak/repog/internal/provider/openrouter"
)

func TestEmbeddingProviderRegistration(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		dims     int
	}{
		{"Gemini", "gemini", "gemini-embedding-2-preview", 768},
		{"OpenAI", "openai", "text-embedding-3-small", 1536},
		{"OpenRouter", "openrouter", "openai/text-embedding-3-small", 1536},
		{"Ollama", "ollama", "nomic-embed-text", 768},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.ProviderConfig{
				Provider:   tt.provider,
				Model:      tt.model,
				Dimensions: tt.dims,
			}

			p, err := provider.NewEmbeddingProvider(cfg, "fake-api-key")
			if err != nil {
				t.Fatalf("Failed to create %s provider: %v", tt.provider, err)
			}

			if p.Name() != tt.provider {
				t.Errorf("Expected provider name %s, got %s", tt.provider, p.Name())
			}

			if p.Dimensions() != tt.dims {
				t.Errorf("Expected dimensions %d, got %d", tt.dims, p.Dimensions())
			}

			if p.BatchSize() == 0 {
				t.Errorf("BatchSize should be > 0")
			}
		})
	}
}

func TestLLMProviderRegistration(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
		fallback string
	}{
		{"Gemini", "gemini", "gemini-2.5-flash", "gemini-3.0-flash"},
		{"OpenAI", "openai", "gpt-4o", "gpt-3.5-turbo"},
		{"OpenRouter", "openrouter", "openai/gpt-4o", "openai/gpt-3.5-turbo"},
		{"Ollama", "ollama", "llama3.2", "llama2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.ProviderConfig{
				Provider: tt.provider,
				Model:    tt.model,
				Fallback: tt.fallback,
			}

			p, err := provider.NewLLMProvider(cfg, "fake-api-key")
			if err != nil {
				t.Fatalf("Failed to create %s provider: %v", tt.provider, err)
			}

			if p.Name() != tt.provider {
				t.Errorf("Expected provider name %s, got %s", tt.provider, p.Name())
			}
		})
	}
}

func TestUnknownProvider(t *testing.T) {
	cfg := config.ProviderConfig{
		Provider:   "unknown",
		Model:      "test",
		Dimensions: 768,
	}

	_, err := provider.NewEmbeddingProvider(cfg, "fake-key")
	if err == nil {
		t.Error("Expected error for unknown provider, got nil")
	}

	_, err = provider.NewLLMProvider(cfg, "fake-key")
	if err == nil {
		t.Error("Expected error for unknown provider, got nil")
	}
}
