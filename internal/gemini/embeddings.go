// Package gemini provides HTTP client functionality for the Gemini API.
package gemini

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

const (
	// EmbeddingDimensions is the expected embedding vector size.
	EmbeddingDimensions = 768
	// EmbeddingModel is the Gemini embedding model to use.
	EmbeddingModel = "gemini-embedding-2-preview"
	// EmbeddingBatchSize is the maximum chunks per batch request.
	EmbeddingBatchSize = 20

	defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
)

// baseURL is the API base URL, can be overridden for testing.
var baseURL = defaultBaseURL

// SetBaseURL sets the API base URL (for testing).
func SetBaseURL(url string) {
	baseURL = url
}

// ResetBaseURL resets the API base URL to the default.
func ResetBaseURL() {
	baseURL = defaultBaseURL
}

// EmbedRequest represents a single chunk to embed.
type EmbedRequest struct {
	ID      int64
	Content string
}

// EmbedResult represents a successful embedding result.
type EmbedResult struct {
	ID        int64
	Embedding []float32 // always 768 dimensions
}

// EmbedError represents a failed embedding attempt.
type EmbedError struct {
	ID    int64
	Error string
}

// BatchEmbedResult contains results from a batch embedding operation.
type BatchEmbedResult struct {
	Results []EmbedResult
	Errors  []EmbedError
}

// batchEmbedRequest is the request body for batchEmbedContents.
type batchEmbedRequest struct {
	Requests []embedContentRequest `json:"requests"`
}

type embedContentRequest struct {
	Model               string       `json:"model"`
	Content             contentParts `json:"content"`
	TaskType            string       `json:"taskType"`
	OutputDimensionality int          `json:"outputDimensionality,omitempty"`
}

type contentParts struct {
	Parts []textPart `json:"parts"`
}

type textPart struct {
	Text string `json:"text"`
}

// batchEmbedResponse is the response from batchEmbedContents.
type batchEmbedResponse struct {
	Embeddings []embeddingData `json:"embeddings"`
}

type embeddingData struct {
	Values []float32 `json:"values"`
}

// EmbedChunks embeds up to 20 chunks in a single batchEmbedContents request.
// Uses taskType RETRIEVAL_DOCUMENT.
// Returns all chunks as errors if the API call fails entirely — never panics.
func EmbedChunks(ctx context.Context, apiKey string, chunks []EmbedRequest) BatchEmbedResult {
	result := BatchEmbedResult{
		Results: make([]EmbedResult, 0),
		Errors:  make([]EmbedError, 0),
	}

	if len(chunks) == 0 {
		return result
	}

	// Build batch request
	requests := make([]embedContentRequest, len(chunks))
	for i, chunk := range chunks {
		requests[i] = embedContentRequest{
			Model: "models/" + EmbeddingModel,
			Content: contentParts{
				Parts: []textPart{{Text: chunk.Content}},
			},
			TaskType:            "RETRIEVAL_DOCUMENT",
			OutputDimensionality: EmbeddingDimensions,
		}
	}

	reqBody := batchEmbedRequest{Requests: requests}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		for _, chunk := range chunks {
			result.Errors = append(result.Errors, EmbedError{ID: chunk.ID, Error: err.Error()})
		}
		return result
	}

	// Make HTTP request
	url := fmt.Sprintf("%s/models/%s:batchEmbedContents?key=%s", baseURL, EmbeddingModel, apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		for _, chunk := range chunks {
			result.Errors = append(result.Errors, EmbedError{ID: chunk.ID, Error: err.Error()})
		}
		return result
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		for _, chunk := range chunks {
			result.Errors = append(result.Errors, EmbedError{ID: chunk.ID, Error: err.Error()})
		}
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		for _, chunk := range chunks {
			result.Errors = append(result.Errors, EmbedError{ID: chunk.ID, Error: err.Error()})
		}
		return result
	}

	if resp.StatusCode != 200 {
		errMsg := fmt.Sprintf("API error: %s - %s", resp.Status, string(respBody))
		for _, chunk := range chunks {
			result.Errors = append(result.Errors, EmbedError{ID: chunk.ID, Error: errMsg})
		}
		return result
	}

	var embedResp batchEmbedResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		for _, chunk := range chunks {
			result.Errors = append(result.Errors, EmbedError{ID: chunk.ID, Error: err.Error()})
		}
		return result
	}

	// Process results
	for i, chunk := range chunks {
		if i >= len(embedResp.Embeddings) {
			result.Errors = append(result.Errors, EmbedError{
				ID:    chunk.ID,
				Error: "No embedding returned from API",
			})
			continue
		}

		embedding := embedResp.Embeddings[i].Values
		if len(embedding) != EmbeddingDimensions {
			result.Errors = append(result.Errors, EmbedError{
				ID:    chunk.ID,
				Error: fmt.Sprintf("Invalid dimensions: expected %d, got %d", EmbeddingDimensions, len(embedding)),
			})
			continue
		}

		result.Results = append(result.Results, EmbedResult{
			ID:        chunk.ID,
			Embedding: embedding,
		})
	}

	return result
}

// embedQueryRequest is the request body for single embedContent.
type embedQueryRequest struct {
	Model               string       `json:"model"`
	Content             contentParts `json:"content"`
	TaskType            string       `json:"taskType"`
	OutputDimensionality int          `json:"outputDimensionality,omitempty"`
}

// embedQueryResponse is the response from embedContent.
type embedQueryResponse struct {
	Embedding embeddingData `json:"embedding"`
}

// EmbedQuery embeds a single query string with taskType RETRIEVAL_QUERY.
// Returns nil on any error — never panics.
func EmbedQuery(ctx context.Context, apiKey string, query string) []float32 {
	reqBody := embedQueryRequest{
		Model: "models/" + EmbeddingModel,
		Content: contentParts{
			Parts: []textPart{{Text: query}},
		},
		TaskType:            "RETRIEVAL_QUERY",
		OutputDimensionality: EmbeddingDimensions,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil
	}

	url := fmt.Sprintf("%s/models/%s:embedContent?key=%s", baseURL, EmbeddingModel, apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")

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

	var embedResp embedQueryResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		return nil
	}

	if len(embedResp.Embedding.Values) != EmbeddingDimensions {
		return nil
	}

	return embedResp.Embedding.Values
}

// Float32SliceToBytes converts a float32 slice to little-endian bytes
// for sqlite-vec storage.
func Float32SliceToBytes(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// BytesToFloat32Slice converts little-endian bytes back to float32 slice.
func BytesToFloat32Slice(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	result := make([]float32, len(b)/4)
	for i := range result {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		result[i] = math.Float32frombits(bits)
	}
	return result
}
