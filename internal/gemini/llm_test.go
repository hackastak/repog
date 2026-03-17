package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCallLLM_ReturnsLLMResultOnSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"text": "hello world"},
						},
					},
				},
			},
			"usageMetadata": map[string]interface{}{
				"promptTokenCount":     10,
				"candidatesTokenCount": 5,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	defer ResetBaseURL()

	result, llmErr := CallLLM(context.Background(), "test-key", LLMRequest{
		Prompt: "test prompt",
	})

	if llmErr != nil {
		t.Fatalf("Expected no error, got %v", llmErr)
	}
	if result.Text != "hello world" {
		t.Errorf("Expected text 'hello world', got '%s'", result.Text)
	}
	if result.InputTokens != 10 {
		t.Errorf("Expected input tokens 10, got %d", result.InputTokens)
	}
	if result.OutputTokens != 5 {
		t.Errorf("Expected output tokens 5, got %d", result.OutputTokens)
	}
	if result.DurationMs < 0 {
		t.Errorf("Expected non-negative duration, got %d", result.DurationMs)
	}
}

func TestCallLLM_ReturnsLLMErrorOnAPIFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	defer ResetBaseURL()

	_, llmErr := CallLLM(context.Background(), "test-key", LLMRequest{
		Prompt: "test prompt",
	})

	if llmErr == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestCallLLM_WithSystemPrompt(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)

		resp := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"text": "response"},
						},
					},
				},
			},
			"usageMetadata": map[string]interface{}{
				"promptTokenCount":     10,
				"candidatesTokenCount": 5,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	defer ResetBaseURL()

	_, _ = CallLLM(context.Background(), "test-key", LLMRequest{
		Prompt:       "test prompt",
		SystemPrompt: "You are a helpful assistant",
	})

	// Verify system_instruction was included in request
	if receivedBody["system_instruction"] == nil {
		t.Error("Expected system_instruction in request body")
	}
}

func TestStreamLLM_CallsOnChunkForEachSSELine(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		chunks := []string{"Hello", " world", "!"}
		for _, chunk := range chunks {
			payload := fmt.Sprintf(`{"candidates":[{"content":{"parts":[{"text":%q}]}}]}`, chunk)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	defer ResetBaseURL()

	var receivedChunks []string
	result, llmErr := StreamLLM(context.Background(), "test-key", LLMRequest{
		Prompt: "test prompt",
	}, func(chunk string) {
		receivedChunks = append(receivedChunks, chunk)
	})

	if llmErr != nil {
		t.Fatalf("Expected no error, got %v", llmErr)
	}

	// Verify onChunk was called for each chunk
	if len(receivedChunks) != 3 {
		t.Errorf("Expected 3 chunks, got %d", len(receivedChunks))
	}

	// Verify assembled text
	expectedText := "Hello world!"
	if result.Text != expectedText {
		t.Errorf("Expected text '%s', got '%s'", expectedText, result.Text)
	}
}

func TestStreamLLM_ReturnsLLMErrorOnAPIFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	defer ResetBaseURL()

	_, llmErr := StreamLLM(context.Background(), "test-key", LLMRequest{
		Prompt: "test prompt",
	}, nil)

	if llmErr == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestStreamLLM_NilOnChunkCallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		payload := `{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`
		_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	defer ResetBaseURL()

	// Pass nil callback - should not panic
	result, llmErr := StreamLLM(context.Background(), "test-key", LLMRequest{
		Prompt: "test prompt",
	}, nil)

	if llmErr != nil {
		t.Fatalf("Expected no error, got %v", llmErr)
	}
	if result.Text != "hello" {
		t.Errorf("Expected text 'hello', got '%s'", result.Text)
	}
}

func TestCallLLM_DefaultsApplied(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)

		resp := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"text": "response"},
						},
					},
				},
			},
			"usageMetadata": map[string]interface{}{},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	defer ResetBaseURL()

	_, _ = CallLLM(context.Background(), "test-key", LLMRequest{
		Prompt: "test",
		// MaxTokens and Temperature not set - should use defaults
	})

	// Verify defaults were applied
	config, ok := receivedBody["generationConfig"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected generationConfig in request")
	}

	if maxTokens, ok := config["maxOutputTokens"].(float64); !ok || maxTokens != 1024 {
		t.Errorf("Expected maxOutputTokens 1024, got %v", config["maxOutputTokens"])
	}
	if temp, ok := config["temperature"].(float64); !ok || temp != 0.3 {
		t.Errorf("Expected temperature 0.3, got %v", config["temperature"])
	}
}
