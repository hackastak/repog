package openai

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
	// Register OpenAI providers
	provider.RegisterEmbeddingProvider("openai", func(cfg config.ProviderConfig, apiKey string) (provider.EmbeddingProvider, error) {
		return NewOpenAIEmbeddingProvider(apiKey, cfg.Model, cfg.Dimensions)
	})
	provider.RegisterLLMProvider("openai", func(cfg config.ProviderConfig, apiKey string) (provider.LLMProvider, error) {
		return NewOpenAILLMProvider(apiKey, cfg.Model, cfg.Fallback)
	})
}

const defaultBaseURL = "https://api.openai.com/v1"

// OpenAIEmbeddingProvider implements the EmbeddingProvider interface for OpenAI
type OpenAIEmbeddingProvider struct {
	apiKey     string
	model      string
	dimensions int
	batchSize  int
	baseURL    string
}

// NewOpenAIEmbeddingProvider creates a new OpenAI embedding provider
func NewOpenAIEmbeddingProvider(apiKey, model string, dimensions int) (*OpenAIEmbeddingProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if model == "" {
		model = "text-embedding-3-small" // Default model
	}
	if dimensions == 0 {
		// Default dimensions for common models
		switch model {
		case "text-embedding-3-small":
			dimensions = 1536
		case "text-embedding-3-large":
			dimensions = 3072
		case "text-embedding-ada-002":
			dimensions = 1536
		default:
			dimensions = 1536 // Safe default
		}
	}

	return &OpenAIEmbeddingProvider{
		apiKey:     apiKey,
		model:      model,
		dimensions: dimensions,
		batchSize:  100, // OpenAI supports large batches
		baseURL:    defaultBaseURL,
	}, nil
}

// Name returns the provider name
func (o *OpenAIEmbeddingProvider) Name() string {
	return "openai"
}

// Dimensions returns the embedding vector dimensions
func (o *OpenAIEmbeddingProvider) Dimensions() int {
	return o.dimensions
}

// BatchSize returns the maximum batch size
func (o *OpenAIEmbeddingProvider) BatchSize() int {
	return o.batchSize
}

// Validate tests the provider connection
func (o *OpenAIEmbeddingProvider) Validate(ctx context.Context) error {
	// Make a test embed call with minimal content
	result := o.EmbedQuery(ctx, "test")
	if result == nil {
		return fmt.Errorf("validation failed: could not embed test query")
	}
	return nil
}

// EmbedChunks embeds multiple chunks in a batch
func (o *OpenAIEmbeddingProvider) EmbedChunks(ctx context.Context, chunks []provider.EmbedRequest) provider.BatchEmbedResult {
	result := provider.BatchEmbedResult{
		Results: make([]provider.EmbedResult, 0),
		Errors:  0,
	}

	if len(chunks) == 0 {
		return result
	}

	// Build request with array of strings
	inputs := make([]string, len(chunks))
	for i, chunk := range chunks {
		inputs[i] = chunk.Content
	}

	reqBody := embeddingRequest{
		Input:      inputs,
		Model:      o.model,
		Dimensions: o.dimensions,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		result.Errors = len(chunks)
		for _, chunk := range chunks {
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     err,
			})
		}
		return result
	}

	// Make HTTP request
	url := fmt.Sprintf("%s/embeddings", o.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		result.Errors = len(chunks)
		for _, chunk := range chunks {
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     err,
			})
		}
		return result
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		result.Errors = len(chunks)
		for _, chunk := range chunks {
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     err,
			})
		}
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Errors = len(chunks)
		for _, chunk := range chunks {
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     err,
			})
		}
		return result
	}

	if resp.StatusCode != 200 {
		err := fmt.Errorf("API error: %s - %s", resp.Status, string(respBody))
		result.Errors = len(chunks)
		for _, chunk := range chunks {
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     err,
			})
		}
		return result
	}

	var embedResp embeddingResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		result.Errors = len(chunks)
		for _, chunk := range chunks {
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     err,
			})
		}
		return result
	}

	// Process results - OpenAI returns embeddings in order
	for i, chunk := range chunks {
		if i >= len(embedResp.Data) {
			result.Errors++
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     fmt.Errorf("no embedding returned from API"),
			})
			continue
		}

		embedding := embedResp.Data[i].Embedding
		if len(embedding) != o.dimensions {
			result.Errors++
			result.Results = append(result.Results, provider.EmbedResult{
				ID:        chunk.ID,
				Embedding: nil,
				Error:     fmt.Errorf("invalid dimensions: expected %d, got %d", o.dimensions, len(embedding)),
			})
			continue
		}

		result.Results = append(result.Results, provider.EmbedResult{
			ID:        chunk.ID,
			Embedding: embedding,
			Error:     nil,
		})
	}

	return result
}

// EmbedQuery embeds a single query for search
func (o *OpenAIEmbeddingProvider) EmbedQuery(ctx context.Context, query string) []float32 {
	reqBody := embeddingRequest{
		Input:      []string{query},
		Model:      o.model,
		Dimensions: o.dimensions,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil
	}

	url := fmt.Sprintf("%s/embeddings", o.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
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

	if len(embedResp.Data) == 0 || len(embedResp.Data[0].Embedding) != o.dimensions {
		return nil
	}

	return embedResp.Data[0].Embedding
}

// Request/response types for OpenAI API

type embeddingRequest struct {
	Input      []string `json:"input"`
	Model      string   `json:"model"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embeddingResponse struct {
	Data  []embeddingData `json:"data"`
	Usage usageInfo       `json:"usage"`
}

type embeddingData struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

type usageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
