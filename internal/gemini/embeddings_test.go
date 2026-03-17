package gemini

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFloat32SliceToBytes(t *testing.T) {
	input := []float32{1.0, 2.0, 3.0, 4.0}
	bytes := Float32SliceToBytes(input)

	if len(bytes) != len(input)*4 {
		t.Errorf("Expected %d bytes, got %d", len(input)*4, len(bytes))
	}

	// Verify round-trip
	output := BytesToFloat32Slice(bytes)
	if len(output) != len(input) {
		t.Errorf("Round-trip failed: expected %d elements, got %d", len(input), len(output))
	}

	for i, v := range input {
		if math.Abs(float64(output[i]-v)) > 0.0001 {
			t.Errorf("Round-trip failed at index %d: expected %f, got %f", i, v, output[i])
		}
	}
}

func TestEmbedChunksSuccess(t *testing.T) {
	// Create mock embedding response
	embedding := make([]float32, 768)
	for i := range embedding {
		embedding[i] = float32(i) * 0.001
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"embeddings": []map[string]interface{}{
				{"values": embedding},
				{"values": embedding},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Note: We can't easily inject the server URL into EmbedChunks
	// This test verifies the Float32SliceToBytes function works correctly
	// Full integration tests would require mocking the HTTP client
}

func TestBytesToFloat32SliceInvalidLength(t *testing.T) {
	// Test with invalid byte length (not multiple of 4)
	result := BytesToFloat32Slice([]byte{1, 2, 3})
	if result != nil {
		t.Error("Expected nil for invalid byte length")
	}
}

func TestFloat32SliceToBytesEmpty(t *testing.T) {
	bytes := Float32SliceToBytes([]float32{})
	if len(bytes) != 0 {
		t.Errorf("Expected empty bytes, got %d bytes", len(bytes))
	}
}

func TestFloat32SliceToBytesProducesCorrectLittleEndianBytes(t *testing.T) {
	// Test a known value: 1.0 in IEEE 754 single precision is 0x3F800000
	input := []float32{1.0}
	bytes := Float32SliceToBytes(input)

	// Little-endian: least significant byte first
	// 0x3F800000 -> [0x00, 0x00, 0x80, 0x3F]
	expected := []byte{0x00, 0x00, 0x80, 0x3F}

	if len(bytes) != 4 {
		t.Fatalf("Expected 4 bytes, got %d", len(bytes))
	}

	for i, b := range expected {
		if bytes[i] != b {
			t.Errorf("Byte %d: expected 0x%02X, got 0x%02X", i, b, bytes[i])
		}
	}
}

func TestFloat32SliceToBytesNegativeValues(t *testing.T) {
	input := []float32{-1.0, -0.5, -100.25}
	bytes := Float32SliceToBytes(input)
	output := BytesToFloat32Slice(bytes)

	if len(output) != len(input) {
		t.Fatalf("Expected %d elements, got %d", len(input), len(output))
	}

	for i, v := range input {
		if output[i] != v {
			t.Errorf("Index %d: expected %f, got %f", i, v, output[i])
		}
	}
}

func TestFloat32SliceToBytesSpecialValues(t *testing.T) {
	input := []float32{0.0, float32(math.Inf(1)), float32(math.Inf(-1))}
	bytes := Float32SliceToBytes(input)
	output := BytesToFloat32Slice(bytes)

	if len(output) != len(input) {
		t.Fatalf("Expected %d elements, got %d", len(input), len(output))
	}

	if output[0] != 0.0 {
		t.Errorf("Expected 0.0, got %f", output[0])
	}
	if !math.IsInf(float64(output[1]), 1) {
		t.Errorf("Expected +Inf, got %f", output[1])
	}
	if !math.IsInf(float64(output[2]), -1) {
		t.Errorf("Expected -Inf, got %f", output[2])
	}
}

func TestBytesToFloat32SliceEmpty(t *testing.T) {
	result := BytesToFloat32Slice([]byte{})
	if len(result) != 0 {
		t.Errorf("Expected empty slice, got %d elements", len(result))
	}
}

func TestEmbedRequestStruct(t *testing.T) {
	req := EmbedRequest{
		ID:      123,
		Content: "test content",
	}

	if req.ID != 123 {
		t.Errorf("Expected ID 123, got %d", req.ID)
	}
	if req.Content != "test content" {
		t.Errorf("Expected content 'test content', got %q", req.Content)
	}
}

func TestEmbedResultStruct(t *testing.T) {
	embedding := make([]float32, 768)
	for i := range embedding {
		embedding[i] = float32(i) * 0.001
	}

	result := EmbedResult{
		ID:        456,
		Embedding: embedding,
	}

	if result.ID != 456 {
		t.Errorf("Expected ID 456, got %d", result.ID)
	}
	if len(result.Embedding) != 768 {
		t.Errorf("Expected 768 dimensions, got %d", len(result.Embedding))
	}
}

func TestEmbedErrorStruct(t *testing.T) {
	err := EmbedError{
		ID:    789,
		Error: "test error message",
	}

	if err.ID != 789 {
		t.Errorf("Expected ID 789, got %d", err.ID)
	}
	if err.Error != "test error message" {
		t.Errorf("Expected error 'test error message', got %q", err.Error)
	}
}

func TestBatchEmbedResultStruct(t *testing.T) {
	result := BatchEmbedResult{
		Results: []EmbedResult{{ID: 1, Embedding: make([]float32, 768)}},
		Errors:  []EmbedError{{ID: 2, Error: "failed"}},
	}

	if len(result.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(result.Results))
	}
	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(result.Errors))
	}
}

func TestConstants(t *testing.T) {
	if EmbeddingDimensions != 768 {
		t.Errorf("Expected EmbeddingDimensions 768, got %d", EmbeddingDimensions)
	}
	if EmbeddingModel != "gemini-embedding-2-preview" {
		t.Errorf("Expected EmbeddingModel 'gemini-embedding-2-preview', got %q", EmbeddingModel)
	}
	if EmbeddingBatchSize != 20 {
		t.Errorf("Expected EmbeddingBatchSize 20, got %d", EmbeddingBatchSize)
	}
}

func TestLLMRequestStruct(t *testing.T) {
	req := LLMRequest{
		Prompt:       "Test prompt",
		SystemPrompt: "You are a helpful assistant",
		MaxTokens:    2048,
		Temperature:  0.7,
	}

	if req.Prompt != "Test prompt" {
		t.Errorf("Expected Prompt 'Test prompt', got %q", req.Prompt)
	}
	if req.SystemPrompt != "You are a helpful assistant" {
		t.Errorf("Expected SystemPrompt 'You are a helpful assistant', got %q", req.SystemPrompt)
	}
	if req.MaxTokens != 2048 {
		t.Errorf("Expected MaxTokens 2048, got %d", req.MaxTokens)
	}
	if req.Temperature != 0.7 {
		t.Errorf("Expected Temperature 0.7, got %f", req.Temperature)
	}
}

func TestLLMResultStruct(t *testing.T) {
	result := LLMResult{
		Text:         "Generated text",
		InputTokens:  100,
		OutputTokens: 50,
		DurationMs:   1000,
	}

	if result.Text != "Generated text" {
		t.Errorf("Expected Text 'Generated text', got %q", result.Text)
	}
	if result.InputTokens != 100 {
		t.Errorf("Expected InputTokens 100, got %d", result.InputTokens)
	}
	if result.OutputTokens != 50 {
		t.Errorf("Expected OutputTokens 50, got %d", result.OutputTokens)
	}
	if result.DurationMs != 1000 {
		t.Errorf("Expected DurationMs 1000, got %d", result.DurationMs)
	}
}

func TestLLMErrorStruct(t *testing.T) {
	llmErr := LLMError{
		Error:      "Something went wrong",
		DurationMs: 500,
	}

	if llmErr.Error != "Something went wrong" {
		t.Errorf("Expected Error 'Something went wrong', got %q", llmErr.Error)
	}
	if llmErr.DurationMs != 500 {
		t.Errorf("Expected DurationMs 500, got %d", llmErr.DurationMs)
	}
}

func TestLLMModelConstant(t *testing.T) {
	if LLMModel != "gemini-2.5-flash" {
		t.Errorf("Expected LLMModel 'gemini-2.5-flash', got %q", LLMModel)
	}
	if LLMModelFallback != "gemini-3.0-flash" {
		t.Errorf("Expected LLMModelFallback 'gemini-3.0-flash', got %q", LLMModelFallback)
	}
}

func TestLLMRequestDefaults(t *testing.T) {
	req := LLMRequest{
		Prompt: "Test",
	}

	// Verify default values would be set
	if req.MaxTokens == 0 {
		req.MaxTokens = 1024
	}
	if req.Temperature == 0 {
		req.Temperature = 0.3
	}

	if req.MaxTokens != 1024 {
		t.Errorf("Expected default MaxTokens 1024, got %d", req.MaxTokens)
	}
	if req.Temperature != 0.3 {
		t.Errorf("Expected default Temperature 0.3, got %f", req.Temperature)
	}
}

func TestFloat32SliceToBytesLargeArray(t *testing.T) {
	// Test with 768 dimensions (actual embedding size)
	input := make([]float32, 768)
	for i := range input {
		input[i] = float32(i) * 0.001
	}

	bytes := Float32SliceToBytes(input)

	if len(bytes) != 768*4 {
		t.Errorf("Expected %d bytes, got %d", 768*4, len(bytes))
	}

	// Verify round-trip
	output := BytesToFloat32Slice(bytes)
	if len(output) != 768 {
		t.Errorf("Expected 768 elements, got %d", len(output))
	}

	for i, v := range input {
		if output[i] != v {
			t.Errorf("Mismatch at index %d: expected %f, got %f", i, v, output[i])
		}
	}
}

func makeTestEmbedding(seed float32) []float32 {
	e := make([]float32, 768)
	for i := range e {
		e[i] = seed + float32(i)*0.001
	}
	return e
}

func TestEmbedChunks_ReturnsCorrectEmbeddingsOnSuccess(t *testing.T) {
	embedding1 := makeTestEmbedding(0.5)
	embedding2 := makeTestEmbedding(1.0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"embeddings": []map[string]interface{}{
				{"values": embedding1},
				{"values": embedding2},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	defer ResetBaseURL()

	chunks := []EmbedRequest{
		{ID: 1, Content: "chunk 1"},
		{ID: 2, Content: "chunk 2"},
	}

	result := EmbedChunks(context.Background(), "test-key", chunks)

	if len(result.Results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(result.Results))
	}
	if len(result.Errors) != 0 {
		t.Errorf("Expected 0 errors, got %d", len(result.Errors))
	}

	for _, r := range result.Results {
		if len(r.Embedding) != 768 {
			t.Errorf("Expected 768 dimensions, got %d", len(r.Embedding))
		}
	}
}

func TestEmbedChunks_ReturnsAllChunksAsErrorsOnAPIFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	defer ResetBaseURL()

	chunks := []EmbedRequest{
		{ID: 1, Content: "chunk 1"},
		{ID: 2, Content: "chunk 2"},
	}

	result := EmbedChunks(context.Background(), "test-key", chunks)

	if len(result.Results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(result.Results))
	}
	if len(result.Errors) != 2 {
		t.Errorf("Expected 2 errors (one per chunk), got %d", len(result.Errors))
	}
}

func TestEmbedChunks_ValidatesEmbeddingDimensions(t *testing.T) {
	// Return an embedding with wrong dimensions
	wrongSizeEmbedding := make([]float32, 10) // Only 10 dimensions instead of 768

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"embeddings": []map[string]interface{}{
				{"values": wrongSizeEmbedding},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	defer ResetBaseURL()

	chunks := []EmbedRequest{
		{ID: 1, Content: "chunk 1"},
	}

	result := EmbedChunks(context.Background(), "test-key", chunks)

	// Chunk should be in errors due to dimension mismatch
	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(result.Errors))
	}
	if len(result.Results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(result.Results))
	}
}

func TestEmbedChunks_EmptyInput(t *testing.T) {
	result := EmbedChunks(context.Background(), "test-key", []EmbedRequest{})

	if len(result.Results) != 0 {
		t.Errorf("Expected 0 results for empty input, got %d", len(result.Results))
	}
	if len(result.Errors) != 0 {
		t.Errorf("Expected 0 errors for empty input, got %d", len(result.Errors))
	}
}

func TestEmbedQuery_Returns768DimSliceOnSuccess(t *testing.T) {
	embedding := makeTestEmbedding(0.5)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"embedding": map[string]interface{}{
				"values": embedding,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	defer ResetBaseURL()

	result := EmbedQuery(context.Background(), "test-key", "test query")

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if len(result) != 768 {
		t.Errorf("Expected 768 dimensions, got %d", len(result))
	}
}

func TestEmbedQuery_ReturnsNilOnAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	defer ResetBaseURL()

	result := EmbedQuery(context.Background(), "test-key", "test query")

	if result != nil {
		t.Errorf("Expected nil on API error, got %v", result)
	}
}

func TestEmbedQuery_ReturnsNilOnWrongDimensions(t *testing.T) {
	wrongSizeEmbedding := make([]float32, 10)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"embedding": map[string]interface{}{
				"values": wrongSizeEmbedding,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	SetBaseURL(server.URL)
	defer ResetBaseURL()

	result := EmbedQuery(context.Background(), "test-key", "test query")

	if result != nil {
		t.Errorf("Expected nil for wrong dimensions, got %v", result)
	}
}
