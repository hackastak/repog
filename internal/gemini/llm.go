package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// LLMModel is the default Gemini LLM model to use.
	// gemini-2.5-flash is the recommended stable model as of March 2026.
	LLMModel = "gemini-2.5-flash"
	// LLMModelFallback is used when the primary model returns 404.
	LLMModelFallback = "gemini-3.0-flash"
)

// LLMRequest contains parameters for an LLM call.
type LLMRequest struct {
	Prompt       string
	SystemPrompt string
	MaxTokens    int     // default 1024
	Temperature  float32 // default 0.3
}

// LLMResult contains the result of a successful LLM call.
type LLMResult struct {
	Text         string
	InputTokens  int
	OutputTokens int
	DurationMs   int64
}

// LLMError contains details of a failed LLM call.
type LLMError struct {
	Error      string
	DurationMs int64
}

// generateContentRequest is the request body for generateContent.
type generateContentRequest struct {
	SystemInstruction *systemInstruction `json:"system_instruction,omitempty"`
	Contents          []content          `json:"contents"`
	GenerationConfig  generationConfig   `json:"generationConfig"`
}

type systemInstruction struct {
	Parts []textPart `json:"parts"`
}

type content struct {
	Role  string     `json:"role"`
	Parts []textPart `json:"parts"`
}

type generationConfig struct {
	MaxOutputTokens int     `json:"maxOutputTokens"`
	Temperature     float32 `json:"temperature"`
}

// generateContentResponse is the response from generateContent.
type generateContentResponse struct {
	Candidates    []candidate   `json:"candidates"`
	UsageMetadata usageMetadata `json:"usageMetadata"`
}

type candidate struct {
	Content content `json:"content"`
}

type usageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
}

// CallLLM makes a non-streaming generateContent request.
// Returns LLMResult on success, LLMError on failure — never panics.
// Falls back to LLMModelFallback if the primary model returns 404.
func CallLLM(ctx context.Context, apiKey string, req LLMRequest) (LLMResult, *LLMError) {
	start := time.Now()

	// Set defaults
	if req.MaxTokens == 0 {
		req.MaxTokens = 1024
	}
	if req.Temperature == 0 {
		req.Temperature = 0.3
	}

	// Build request body
	body := generateContentRequest{
		Contents: []content{
			{
				Role:  "user",
				Parts: []textPart{{Text: req.Prompt}},
			},
		},
		GenerationConfig: generationConfig{
			MaxOutputTokens: req.MaxTokens,
			Temperature:     req.Temperature,
		},
	}

	if req.SystemPrompt != "" {
		body.SystemInstruction = &systemInstruction{
			Parts: []textPart{{Text: req.SystemPrompt}},
		}
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return LLMResult{}, &LLMError{
			Error:      err.Error(),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// Try primary model, fall back if 404
	models := []string{LLMModel, LLMModelFallback}
	client := &http.Client{Timeout: 120 * time.Second}

	var lastError *LLMError
	for _, model := range models {
		url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", baseURL, model, apiKey)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			return LLMResult{}, &LLMError{
				Error:      err.Error(),
				DurationMs: time.Since(start).Milliseconds(),
			}
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			return LLMResult{}, &LLMError{
				Error:      err.Error(),
				DurationMs: time.Since(start).Milliseconds(),
			}
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return LLMResult{}, &LLMError{
				Error:      err.Error(),
				DurationMs: time.Since(start).Milliseconds(),
			}
		}

		// If 404, try next model
		if resp.StatusCode == 404 {
			lastError = &LLMError{
				Error:      fmt.Sprintf("API error: %s - %s", resp.Status, string(respBody)),
				DurationMs: time.Since(start).Milliseconds(),
			}
			continue
		}

		if resp.StatusCode != 200 {
			return LLMResult{}, &LLMError{
				Error:      fmt.Sprintf("API error: %s - %s", resp.Status, string(respBody)),
				DurationMs: time.Since(start).Milliseconds(),
			}
		}

		var genResp generateContentResponse
		if err := json.Unmarshal(respBody, &genResp); err != nil {
			return LLMResult{}, &LLMError{
				Error:      err.Error(),
				DurationMs: time.Since(start).Milliseconds(),
			}
		}

		// Extract text from response
		var text string
		if len(genResp.Candidates) > 0 && len(genResp.Candidates[0].Content.Parts) > 0 {
			text = genResp.Candidates[0].Content.Parts[0].Text
		}

		return LLMResult{
			Text:         text,
			InputTokens:  genResp.UsageMetadata.PromptTokenCount,
			OutputTokens: genResp.UsageMetadata.CandidatesTokenCount,
			DurationMs:   time.Since(start).Milliseconds(),
		}, nil
	}

	// All models returned 404
	return LLMResult{}, lastError
}

// streamChunk represents a chunk from the SSE stream.
type streamChunk struct {
	Candidates    []candidate   `json:"candidates"`
	UsageMetadata usageMetadata `json:"usageMetadata"`
}

// StreamLLM makes a streaming generateContent request.
// Calls onChunk for each text chunk as it arrives via SSE.
// Returns LLMResult with assembled text on completion.
// Returns LLMError on failure — never panics.
// Falls back to LLMModelFallback if the primary model returns 404.
func StreamLLM(ctx context.Context, apiKey string, req LLMRequest, onChunk func(string)) (LLMResult, *LLMError) {
	start := time.Now()

	// Set defaults
	if req.MaxTokens == 0 {
		req.MaxTokens = 1024
	}
	if req.Temperature == 0 {
		req.Temperature = 0.3
	}

	// Build request body
	body := generateContentRequest{
		Contents: []content{
			{
				Role:  "user",
				Parts: []textPart{{Text: req.Prompt}},
			},
		},
		GenerationConfig: generationConfig{
			MaxOutputTokens: req.MaxTokens,
			Temperature:     req.Temperature,
		},
	}

	if req.SystemPrompt != "" {
		body.SystemInstruction = &systemInstruction{
			Parts: []textPart{{Text: req.SystemPrompt}},
		}
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return LLMResult{}, &LLMError{
			Error:      err.Error(),
			DurationMs: time.Since(start).Milliseconds(),
		}
	}

	// Try primary model, fall back if 404
	models := []string{LLMModel, LLMModelFallback}
	client := &http.Client{Timeout: 300 * time.Second}

	var lastError *LLMError
	for _, model := range models {
		url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", baseURL, model, apiKey)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			return LLMResult{}, &LLMError{
				Error:      err.Error(),
				DurationMs: time.Since(start).Milliseconds(),
			}
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(httpReq)
		if err != nil {
			return LLMResult{}, &LLMError{
				Error:      err.Error(),
				DurationMs: time.Since(start).Milliseconds(),
			}
		}

		// If 404, try next model
		if resp.StatusCode == 404 {
			respBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			lastError = &LLMError{
				Error:      fmt.Sprintf("API error: %s - %s", resp.Status, string(respBody)),
				DurationMs: time.Since(start).Milliseconds(),
			}
			continue
		}

		if resp.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return LLMResult{}, &LLMError{
				Error:      fmt.Sprintf("API error: %s - %s", resp.Status, string(respBody)),
				DurationMs: time.Since(start).Milliseconds(),
			}
		}

		// Parse SSE stream
		var fullText strings.Builder
		var inputTokens, outputTokens int

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				jsonData := strings.TrimPrefix(line, "data: ")
				if jsonData == "" {
					continue
				}

				var chunk streamChunk
				if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
					continue
				}

				// Extract text from chunk
				if len(chunk.Candidates) > 0 && len(chunk.Candidates[0].Content.Parts) > 0 {
					text := chunk.Candidates[0].Content.Parts[0].Text
					if text != "" {
						fullText.WriteString(text)
						if onChunk != nil {
							onChunk(text)
						}
					}
				}

				// Update token counts (they accumulate in the stream)
				if chunk.UsageMetadata.PromptTokenCount > 0 {
					inputTokens = chunk.UsageMetadata.PromptTokenCount
				}
				if chunk.UsageMetadata.CandidatesTokenCount > 0 {
					outputTokens = chunk.UsageMetadata.CandidatesTokenCount
				}
			}
		}

		_ = resp.Body.Close()

		if err := scanner.Err(); err != nil {
			return LLMResult{}, &LLMError{
				Error:      err.Error(),
				DurationMs: time.Since(start).Milliseconds(),
			}
		}

		return LLMResult{
			Text:         fullText.String(),
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			DurationMs:   time.Since(start).Milliseconds(),
		}, nil
	}

	// All models returned 404
	return LLMResult{}, lastError
}
