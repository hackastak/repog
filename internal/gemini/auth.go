package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// APIKeyValidationResult contains the result of validating a Gemini API key.
type APIKeyValidationResult struct {
	Valid bool
	Error string
}

// ValidateAPIKey makes a lightweight embedContent call to verify the key.
func ValidateAPIKey(ctx context.Context, apiKey string) APIKeyValidationResult {
	// Build a minimal embed request
	reqBody := embedQueryRequest{
		Model: "models/" + EmbeddingModel,
		Content: contentParts{
			Parts: []textPart{{Text: "test"}},
		},
		TaskType: "RETRIEVAL_QUERY",
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return APIKeyValidationResult{
			Valid: false,
			Error: fmt.Sprintf("Failed to build request: %s", err.Error()),
		}
	}

	url := fmt.Sprintf("%s/models/%s:embedContent?key=%s", baseURL, EmbeddingModel, apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return APIKeyValidationResult{
			Valid: false,
			Error: fmt.Sprintf("Failed to create request: %s", err.Error()),
		}
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return APIKeyValidationResult{
			Valid: false,
			Error: fmt.Sprintf("Request failed: %s", err.Error()),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return APIKeyValidationResult{
			Valid: false,
			Error: fmt.Sprintf("API returned %d: %s", resp.StatusCode, string(body)),
		}
	}

	return APIKeyValidationResult{
		Valid: true,
	}
}
