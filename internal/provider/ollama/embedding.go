package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hackastak/repog/internal/config"
	"github.com/hackastak/repog/internal/provider"
)

func init() {
	// Register Ollama providers
	provider.RegisterEmbeddingProvider("ollama", func(cfg config.ProviderConfig, apiKey string) (provider.EmbeddingProvider, error) {
		return NewOllamaEmbeddingProvider(cfg.Model, cfg.Dimensions, cfg.BaseURL)
	})
	provider.RegisterLLMProvider("ollama", func(cfg config.ProviderConfig, apiKey string) (provider.LLMProvider, error) {
		return NewOllamaLLMProvider(cfg.Model, cfg.Fallback, cfg.BaseURL)
	})
}

const defaultBaseURL = "http://localhost:11434"

// OllamaEmbeddingProvider implements the EmbeddingProvider interface for Ollama
type OllamaEmbeddingProvider struct {
	model      string
	dimensions int
	batchSize  int
	baseURL    string
}

// NewOllamaEmbeddingProvider creates a new Ollama embedding provider
func NewOllamaEmbeddingProvider(model string, dimensions int, baseURL string) (*OllamaEmbeddingProvider, error) {
	if model == "" {
		model = "nomic-embed-text" // Default embedding model
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if dimensions == 0 {
		// Default dimensions for common models
		switch model {
		case "nomic-embed-text":
			dimensions = 768
		case "mxbai-embed-large":
			dimensions = 1024
		case "all-minilm":
			dimensions = 384
		default:
			dimensions = 768 // Safe default
		}
	}

	return &OllamaEmbeddingProvider{
		model:      model,
		dimensions: dimensions,
		batchSize:  10, // Conservative batch size for local processing
		baseURL:    baseURL,
	}, nil
}

// Name returns the provider name
func (o *OllamaEmbeddingProvider) Name() string {
	return "ollama"
}

// Dimensions returns the embedding vector dimensions
func (o *OllamaEmbeddingProvider) Dimensions() int {
	return o.dimensions
}

// BatchSize returns the maximum batch size
func (o *OllamaEmbeddingProvider) BatchSize() int {
	return o.batchSize
}

// Validate tests the provider connection
func (o *OllamaEmbeddingProvider) Validate(ctx context.Context) error {
	// Make a test embed call with minimal content
	result := o.EmbedQuery(ctx, "test")
	if result == nil {
		return fmt.Errorf("validation failed: could not embed test query (is Ollama running?)")
	}
	return nil
}

// EmbedChunks embeds multiple chunks in a batch
func (o *OllamaEmbeddingProvider) EmbedChunks(ctx context.Context, chunks []provider.EmbedRequest) provider.BatchEmbedResult {
	result := provider.BatchEmbedResult{
		Results: make([]provider.EmbedResult, 0),
		Errors:  0,
	}

	if len(chunks) == 0 {
		return result
	}

	// Ollama doesn't support batch embeddings, so we process one at a time
	for _, chunk := range chunks {
		embedding := o.EmbedQuery(ctx, chunk.Content)
		if embedding == nil {
			result.Errors++
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     fmt.Errorf("failed to embed chunk"),
			})
		} else {
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: embedding,
				Error:     nil,
			})
		}
	}

	return result
}

// EmbedQuery embeds a single query for search
func (o *OllamaEmbeddingProvider) EmbedQuery(ctx context.Context, query string) []float32 {
	reqBody := embeddingRequest{
		Model:  o.model,
		Prompt: query,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil
	}

	url := fmt.Sprintf("%s/api/embeddings", o.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second} // Longer timeout for local processing
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var embedResp embeddingResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		return nil
	}

	if len(embedResp.Embedding) == 0 {
		return nil
	}

	// Ollama returns float64, convert to float32
	embedding := make([]float32, len(embedResp.Embedding))
	for i, v := range embedResp.Embedding {
		embedding[i] = float32(v)
	}

	// Verify dimensions match
	if len(embedding) != o.dimensions {
		// Update dimensions if this is the first call
		o.dimensions = len(embedding)
	}

	return embedding
}

// Request/response types for Ollama API

type embeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embeddingResponse struct {
	Embedding []float64 `json:"embedding"`
}
