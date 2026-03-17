package summarize

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/gemini"
)

func TestBuildSummarizePrompt(t *testing.T) {
	chunks := []chunkRecord{
		{ChunkType: "metadata", Content: `{"full_name": "user/repo"}`},
		{ChunkType: "readme", Content: "# My Repo\nThis is a test repo."},
	}

	prompt := buildSummarizePrompt("user/repo", chunks)

	if !containsString(prompt, "user/repo") {
		t.Error("Prompt should contain repo name")
	}

	if !containsString(prompt, "metadata") {
		t.Error("Prompt should contain chunk type")
	}

	if !containsString(prompt, "# My Repo") {
		t.Error("Prompt should contain chunk content")
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func insertTestRepoWithChunks(t *testing.T, database *sql.DB, fullName string, chunks []chunkRecord) int64 {
	t.Helper()

	result, err := database.Exec(`
		INSERT INTO repos (github_id, full_name, owner, name, description, stars, language, topics, synced_at)
		VALUES (?, ?, 'owner', 'name', 'desc', 100, 'Go', '[]', datetime('now'))
	`, 12345, fullName)
	if err != nil {
		t.Fatalf("Failed to insert repo: %v", err)
	}

	repoID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get repo ID: %v", err)
	}

	for _, chunk := range chunks {
		_, err := database.Exec(`
			INSERT INTO chunks (repo_id, chunk_type, content)
			VALUES (?, ?, ?)
		`, repoID, chunk.ChunkType, chunk.Content)
		if err != nil {
			t.Fatalf("Failed to insert chunk: %v", err)
		}
	}

	return repoID
}

func TestSummarizeRepo_ReturnsPopulatedResult(t *testing.T) {
	// Create mock LLM server with streaming response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		chunks := []string{
			"## Overview\nThis is a test repo.",
			"\n\n## Tech Stack\nGo language.",
			"\n\n## Use Cases\nTesting.",
		}
		for _, chunk := range chunks {
			payload := fmt.Sprintf(`{"candidates":[{"content":{"parts":[{"text":%q}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":50}}`, chunk)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	// Set up test database
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	// Insert test data
	insertTestRepoWithChunks(t, database, "user/testrepo", []chunkRecord{
		{ChunkType: "metadata", Content: `{"full_name": "user/testrepo"}`},
		{ChunkType: "readme", Content: "# Test Repo\nA test repository."},
	})

	// Track chunks received
	var receivedChunks []string
	onChunk := func(chunk string) {
		receivedChunks = append(receivedChunks, chunk)
	}

	result, err := SummarizeRepo(context.Background(), SummarizeOptions{
		Repo:   "user/testrepo",
		DB:     database,
		APIKey: "test-key",
	}, onChunk)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.Repo != "user/testrepo" {
		t.Errorf("Expected repo 'user/testrepo', got '%s'", result.Repo)
	}

	if result.ChunksUsed != 2 {
		t.Errorf("Expected 2 chunks used, got %d", result.ChunksUsed)
	}

	if !containsString(result.Summary, "## Overview") {
		t.Errorf("Expected summary to contain '## Overview', got '%s'", result.Summary)
	}

	if len(receivedChunks) == 0 {
		t.Error("Expected onChunk to be called")
	}

	if result.DurationMs < 0 {
		t.Errorf("Expected non-negative duration, got %d", result.DurationMs)
	}
}

func TestSummarizeRepo_CaseInsensitiveMatch(t *testing.T) {
	// Create mock LLM server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		payload := `{"candidates":[{"content":{"parts":[{"text":"Summary"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`
		_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	// Insert with lowercase
	insertTestRepoWithChunks(t, database, "user/myrepo", []chunkRecord{
		{ChunkType: "readme", Content: "# My Repo"},
	})

	// Query with different case
	result, err := SummarizeRepo(context.Background(), SummarizeOptions{
		Repo:   "USER/MYREPO",
		DB:     database,
		APIKey: "test-key",
	}, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.ChunksUsed != 1 {
		t.Errorf("Expected 1 chunk used (case-insensitive match), got %d", result.ChunksUsed)
	}
}

func TestSummarizeRepo_ReturnsNoDataMessageWhenNoChunks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	var receivedChunk string
	onChunk := func(chunk string) {
		receivedChunk = chunk
	}

	result, err := SummarizeRepo(context.Background(), SummarizeOptions{
		Repo:   "nonexistent/repo",
		DB:     database,
		APIKey: "test-key",
	}, onChunk)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	expectedMsg := "No data found for this repository. Try running `repog sync` and `repog embed` first."
	if result.Summary != expectedMsg {
		t.Errorf("Expected no data message, got '%s'", result.Summary)
	}

	if receivedChunk != expectedMsg {
		t.Errorf("Expected onChunk to receive no data message, got '%s'", receivedChunk)
	}

	if result.ChunksUsed != 0 {
		t.Errorf("Expected 0 chunks used, got %d", result.ChunksUsed)
	}
}

func TestSummarizeRepo_HandlesLLMError(t *testing.T) {
	// Create mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	insertTestRepoWithChunks(t, database, "user/repo", []chunkRecord{
		{ChunkType: "readme", Content: "# Repo"},
	})

	result, err := SummarizeRepo(context.Background(), SummarizeOptions{
		Repo:   "user/repo",
		DB:     database,
		APIKey: "test-key",
	}, nil)

	// Should not return Go error, but Summary should contain error message
	if err != nil {
		t.Fatalf("Expected no Go error, got %v", err)
	}

	if !containsString(result.Summary, "Error generating summary") {
		t.Errorf("Expected error message in summary, got '%s'", result.Summary)
	}
}

func TestSummarizeRepo_VerifiesRequestStructure(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)

		w.Header().Set("Content-Type", "text/event-stream")
		payload := `{"candidates":[{"content":{"parts":[{"text":"Summary"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`
		_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	insertTestRepoWithChunks(t, database, "user/repo", []chunkRecord{
		{ChunkType: "readme", Content: "# Repo"},
	})

	_, _ = SummarizeRepo(context.Background(), SummarizeOptions{
		Repo:   "user/repo",
		DB:     database,
		APIKey: "test-key",
	}, nil)

	// Verify system_instruction was included
	if receivedBody["system_instruction"] == nil {
		t.Error("Expected system_instruction in request body")
	}
}

func TestSummarizeRepo_NilOnChunkCallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		payload := `{"candidates":[{"content":{"parts":[{"text":"Summary"}]}}],"usageMetadata":{}}`
		_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	insertTestRepoWithChunks(t, database, "user/repo", []chunkRecord{
		{ChunkType: "readme", Content: "# Repo"},
	})

	// Should not panic with nil callback
	result, err := SummarizeRepo(context.Background(), SummarizeOptions{
		Repo:   "user/repo",
		DB:     database,
		APIKey: "test-key",
	}, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.Summary != "Summary" {
		t.Errorf("Expected 'Summary', got '%s'", result.Summary)
	}
}
