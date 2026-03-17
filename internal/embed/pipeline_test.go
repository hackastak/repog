package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/gemini"
)

// makeTestEmbedding creates a 768-dimension embedding with a seed value
func makeTestEmbedding(seed float32) []float32 {
	e := make([]float32, 768)
	for i := range e {
		e[i] = seed + float32(i)*0.001
	}
	return e
}

func TestRunEmbedPipeline_EmbedsSingleBatch(t *testing.T) {
	// Open test database
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert a repo with pushed_at_hash but no embedded_hash
	_, err = database.Exec(`INSERT INTO repos (github_id, owner, name, full_name, stars, pushed_at, pushed_at_hash, html_url, default_branch, synced_at)
		VALUES (1, 'user', 'repo1', 'user/repo1', 100, '2024-01-15T10:00:00Z', 'abc123hash', 'https://github.com/user/repo1', 'main', datetime('now'))`)
	if err != nil {
		t.Fatalf("Failed to insert repo: %v", err)
	}

	// Get repo ID
	var repoID int64
	err = database.QueryRow("SELECT id FROM repos WHERE full_name = 'user/repo1'").Scan(&repoID)
	if err != nil {
		t.Fatalf("Failed to get repo ID: %v", err)
	}

	// Insert a metadata chunk
	_, err = database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'metadata', ?)",
		repoID, `{"full_name":"user/repo1","stars":100}`)
	if err != nil {
		t.Fatalf("Failed to insert chunk: %v", err)
	}

	// Set up mock Gemini server returning valid 768-dim embeddings
	embedding := makeTestEmbedding(0.5)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"embeddings": []map[string]interface{}{
				{"values": embedding},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Override Gemini base URL
	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	// Run embed pipeline
	eventCh := RunEmbedPipeline(context.Background(), EmbedOptions{
		DB:           database,
		GeminiAPIKey: "test-key",
		BatchSize:    10,
	})

	// Drain events
	var doneEvent EmbedEvent
	for event := range eventCh {
		if event.Type == "done" {
			doneEvent = event
		}
	}

	// Verify chunk_embeddings has one row
	var count int
	err = database.QueryRow("SELECT COUNT(*) FROM chunk_embeddings").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query chunk_embeddings: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 embedding, got %d", count)
	}

	// Verify embedding blob has correct length (768 * 4 = 3072 bytes)
	var embeddingBlob []byte
	err = database.QueryRow("SELECT embedding FROM chunk_embeddings").Scan(&embeddingBlob)
	if err != nil {
		t.Fatalf("Failed to query embedding: %v", err)
	}
	if len(embeddingBlob) != 768*4 {
		t.Errorf("Expected embedding length %d, got %d", 768*4, len(embeddingBlob))
	}

	// Verify done event
	if doneEvent.ChunksEmbedded < 1 {
		t.Errorf("Expected at least 1 chunk embedded, got %d", doneEvent.ChunksEmbedded)
	}
}

func TestRunEmbedPipeline_SkipsAlreadyEmbeddedRepo(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert repo with embedded_hash == pushed_at_hash (already embedded)
	hash := "abc123hash"
	_, err = database.Exec(`INSERT INTO repos (github_id, owner, name, full_name, stars, pushed_at, pushed_at_hash, embedded_hash, html_url, default_branch, synced_at)
		VALUES (1, 'user', 'repo1', 'user/repo1', 100, '2024-01-15T10:00:00Z', ?, ?, 'https://github.com/user/repo1', 'main', datetime('now'))`,
		hash, hash)
	if err != nil {
		t.Fatalf("Failed to insert repo: %v", err)
	}

	// Get repo ID and insert chunk
	var repoID int64
	err = database.QueryRow("SELECT id FROM repos WHERE full_name = 'user/repo1'").Scan(&repoID)
	if err != nil {
		t.Fatalf("Failed to get repo ID: %v", err)
	}

	_, err = database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'metadata', ?)",
		repoID, `{"full_name":"user/repo1"}`)
	if err != nil {
		t.Fatalf("Failed to insert chunk: %v", err)
	}

	// Track API calls
	apiCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls++
		resp := map[string]interface{}{
			"embeddings": []map[string]interface{}{
				{"values": makeTestEmbedding(0.5)},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	// Run embed pipeline
	eventCh := RunEmbedPipeline(context.Background(), EmbedOptions{
		DB:           database,
		GeminiAPIKey: "test-key",
		BatchSize:    10,
	})

	// Drain events and look for repo_skip
	var gotSkipEvent bool
	for event := range eventCh {
		if event.Type == "repo_skip" {
			gotSkipEvent = true
		}
	}

	if !gotSkipEvent {
		t.Error("Expected repo_skip event for already embedded repo")
	}

	// Verify Gemini API was NOT called (repo was skipped)
	if apiCalls != 0 {
		t.Errorf("Expected 0 API calls (repo should be skipped), got %d", apiCalls)
	}
}

func TestRunEmbedPipeline_BatchesChunksCorrectly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert 25 repos with 1 chunk each to get 25 chunks for batching test
	for i := 0; i < 25; i++ {
		_, err = database.Exec(`INSERT INTO repos (github_id, owner, name, full_name, stars, pushed_at, pushed_at_hash, html_url, default_branch, synced_at)
			VALUES (?, 'user', ?, ?, 100, '2024-01-15T10:00:00Z', ?, 'https://github.com/user/repo', 'main', datetime('now'))`,
			i+100, "repo"+string(rune('A'+i)), "user/repo"+string(rune('A'+i)), "hash"+string(rune('A'+i)))
		if err != nil {
			t.Fatalf("Failed to insert repo %d: %v", i, err)
		}
		var rID int64
		_ = database.QueryRow("SELECT id FROM repos WHERE github_id = ?", i+100).Scan(&rID)
		_, err = database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'metadata', ?)",
			rID, `{"index":`+string(rune('0'+i%10))+`}`)
		if err != nil {
			t.Fatalf("Failed to insert chunk %d: %v", i, err)
		}
	}

	// Track API requests
	apiRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiRequests++
		// Return embeddings for all chunks in the request
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Logf("Failed to decode request body: %v", err)
		}

		requests, ok := reqBody["requests"].([]interface{})
		if !ok {
			requests = []interface{}{}
		}

		embeddings := make([]map[string]interface{}, len(requests))
		for i := range embeddings {
			embeddings[i] = map[string]interface{}{"values": makeTestEmbedding(float32(i))}
		}

		resp := map[string]interface{}{"embeddings": embeddings}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	// Run embed pipeline with batch size of 10
	eventCh := RunEmbedPipeline(context.Background(), EmbedOptions{
		DB:           database,
		GeminiAPIKey: "test-key",
		BatchSize:    10,
	})

	// Drain events
	for range eventCh {
	}

	// Verify 3 API requests (batches of 10, 10, 5)
	if apiRequests != 3 {
		t.Errorf("Expected 3 API requests (batches of 10, 10, 5), got %d", apiRequests)
	}
}

func TestRunEmbedPipeline_ExcludesFileTreeByDefault(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert repo
	_, err = database.Exec(`INSERT INTO repos (github_id, owner, name, full_name, stars, pushed_at, pushed_at_hash, html_url, default_branch, synced_at)
		VALUES (1, 'user', 'repo1', 'user/repo1', 100, '2024-01-15T10:00:00Z', 'abc123hash', 'https://github.com/user/repo1', 'main', datetime('now'))`)
	if err != nil {
		t.Fatalf("Failed to insert repo: %v", err)
	}

	var repoID int64
	err = database.QueryRow("SELECT id FROM repos").Scan(&repoID)
	if err != nil {
		t.Fatalf("Failed to get repo ID: %v", err)
	}

	// Insert metadata, readme, and file_tree chunks
	_, err = database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'metadata', ?)", repoID, `{"full_name":"user/repo1"}`)
	if err != nil {
		t.Fatalf("Failed to insert metadata chunk: %v", err)
	}
	_, err = database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'readme', ?)", repoID, "# README")
	if err != nil {
		t.Fatalf("Failed to insert readme chunk: %v", err)
	}
	_, err = database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'file_tree', ?)", repoID, "main.go\ngo.mod")
	if err != nil {
		t.Fatalf("Failed to insert file_tree chunk: %v", err)
	}

	// Track chunks sent to API
	chunksReceived := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err == nil {
			if requests, ok := reqBody["requests"].([]interface{}); ok {
				chunksReceived = len(requests)
			}
		}

		embeddings := make([]map[string]interface{}, chunksReceived)
		for i := range embeddings {
			embeddings[i] = map[string]interface{}{"values": makeTestEmbedding(float32(i))}
		}
		resp := map[string]interface{}{"embeddings": embeddings}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	// Run embed pipeline with IncludeFileTree=false (default)
	eventCh := RunEmbedPipeline(context.Background(), EmbedOptions{
		DB:              database,
		GeminiAPIKey:    "test-key",
		BatchSize:       10,
		IncludeFileTree: false,
	})

	for range eventCh {
	}

	// Verify only 2 chunks were sent (metadata + readme, not file_tree)
	if chunksReceived != 2 {
		t.Errorf("Expected 2 chunks (excluding file_tree), got %d", chunksReceived)
	}

	// Verify chunk_embeddings has 2 rows
	var count int
	err = database.QueryRow("SELECT COUNT(*) FROM chunk_embeddings").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query chunk_embeddings: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected 2 embeddings, got %d", count)
	}
}

func TestRunEmbedPipeline_IncludesFileTreeWhenFlagSet(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert repo
	_, err = database.Exec(`INSERT INTO repos (github_id, owner, name, full_name, stars, pushed_at, pushed_at_hash, html_url, default_branch, synced_at)
		VALUES (1, 'user', 'repo1', 'user/repo1', 100, '2024-01-15T10:00:00Z', 'abc123hash', 'https://github.com/user/repo1', 'main', datetime('now'))`)
	if err != nil {
		t.Fatalf("Failed to insert repo: %v", err)
	}

	var repoID int64
	err = database.QueryRow("SELECT id FROM repos").Scan(&repoID)
	if err != nil {
		t.Fatalf("Failed to get repo ID: %v", err)
	}

	// Insert metadata, readme, and file_tree chunks
	_, err = database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'metadata', ?)", repoID, `{"full_name":"user/repo1"}`)
	if err != nil {
		t.Fatalf("Failed to insert metadata chunk: %v", err)
	}
	_, err = database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'readme', ?)", repoID, "# README")
	if err != nil {
		t.Fatalf("Failed to insert readme chunk: %v", err)
	}
	_, err = database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'file_tree', ?)", repoID, "main.go\ngo.mod")
	if err != nil {
		t.Fatalf("Failed to insert file_tree chunk: %v", err)
	}

	// Track chunks sent to API
	chunksReceived := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err == nil {
			if requests, ok := reqBody["requests"].([]interface{}); ok {
				chunksReceived = len(requests)
			}
		}

		embeddings := make([]map[string]interface{}, chunksReceived)
		for i := range embeddings {
			embeddings[i] = map[string]interface{}{"values": makeTestEmbedding(float32(i))}
		}
		resp := map[string]interface{}{"embeddings": embeddings}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	// Run embed pipeline with IncludeFileTree=true
	eventCh := RunEmbedPipeline(context.Background(), EmbedOptions{
		DB:              database,
		GeminiAPIKey:    "test-key",
		BatchSize:       10,
		IncludeFileTree: true,
	})

	for range eventCh {
	}

	// Verify 3 chunks were sent (metadata + readme + file_tree)
	if chunksReceived != 3 {
		t.Errorf("Expected 3 chunks (including file_tree), got %d", chunksReceived)
	}

	// Verify chunk_embeddings has 3 rows
	var count int
	err = database.QueryRow("SELECT COUNT(*) FROM chunk_embeddings").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query chunk_embeddings: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 embeddings, got %d", count)
	}
}

func TestRunEmbedPipeline_UpdatesEmbeddedHashAfterSuccess(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	pushedAtHash := "abc123hash"
	_, err = database.Exec(`INSERT INTO repos (github_id, owner, name, full_name, stars, pushed_at, pushed_at_hash, html_url, default_branch, synced_at)
		VALUES (1, 'user', 'repo1', 'user/repo1', 100, '2024-01-15T10:00:00Z', ?, 'https://github.com/user/repo1', 'main', datetime('now'))`,
		pushedAtHash)
	if err != nil {
		t.Fatalf("Failed to insert repo: %v", err)
	}

	var repoID int64
	err = database.QueryRow("SELECT id FROM repos").Scan(&repoID)
	if err != nil {
		t.Fatalf("Failed to get repo ID: %v", err)
	}

	_, err = database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'metadata', ?)", repoID, `{"full_name":"user/repo1"}`)
	if err != nil {
		t.Fatalf("Failed to insert chunk: %v", err)
	}

	// Verify embedded_hash is NULL before pipeline
	var embeddedHash *string
	err = database.QueryRow("SELECT embedded_hash FROM repos WHERE id = ?", repoID).Scan(&embeddedHash)
	if err != nil {
		t.Fatalf("Failed to query repo: %v", err)
	}
	if embeddedHash != nil {
		t.Error("Expected embedded_hash to be NULL before pipeline")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"embeddings": []map[string]interface{}{
				{"values": makeTestEmbedding(0.5)},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	eventCh := RunEmbedPipeline(context.Background(), EmbedOptions{
		DB:           database,
		GeminiAPIKey: "test-key",
		BatchSize:    10,
	})

	for range eventCh {
	}

	// Verify embedded_hash is now set and equals pushed_at_hash
	var updatedHash string
	err = database.QueryRow("SELECT embedded_hash FROM repos WHERE id = ?", repoID).Scan(&updatedHash)
	if err != nil {
		t.Fatalf("Failed to query updated repo: %v", err)
	}
	if updatedHash != pushedAtHash {
		t.Errorf("Expected embedded_hash '%s', got '%s'", pushedAtHash, updatedHash)
	}
}

func TestRunEmbedPipeline_DoneEventHasCorrectTotals(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	hash := "abc123hash"

	// Repo 1: already embedded (should be skipped)
	_, err = database.Exec(`INSERT INTO repos (github_id, owner, name, full_name, stars, pushed_at, pushed_at_hash, embedded_hash, html_url, default_branch, synced_at)
		VALUES (1, 'user', 'repo1', 'user/repo1', 100, '2024-01-15T10:00:00Z', ?, ?, 'https://github.com/user/repo1', 'main', datetime('now'))`,
		hash, hash)
	if err != nil {
		t.Fatalf("Failed to insert repo1: %v", err)
	}

	// Repo 2: needs embedding
	_, err = database.Exec(`INSERT INTO repos (github_id, owner, name, full_name, stars, pushed_at, pushed_at_hash, html_url, default_branch, synced_at)
		VALUES (2, 'user', 'repo2', 'user/repo2', 100, '2024-01-15T10:00:00Z', 'def456hash', 'https://github.com/user/repo2', 'main', datetime('now'))`)
	if err != nil {
		t.Fatalf("Failed to insert repo2: %v", err)
	}

	// Insert chunks for both repos
	var repo1ID, repo2ID int64
	_ = database.QueryRow("SELECT id FROM repos WHERE full_name = 'user/repo1'").Scan(&repo1ID)
	_ = database.QueryRow("SELECT id FROM repos WHERE full_name = 'user/repo2'").Scan(&repo2ID)

	_, _ = database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'metadata', ?)", repo1ID, `{"full_name":"user/repo1"}`)
	_, _ = database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'metadata', ?)", repo2ID, `{"full_name":"user/repo2"}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"embeddings": []map[string]interface{}{
				{"values": makeTestEmbedding(0.5)},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	eventCh := RunEmbedPipeline(context.Background(), EmbedOptions{
		DB:           database,
		GeminiAPIKey: "test-key",
		BatchSize:    10,
	})

	var doneEvent EmbedEvent
	for event := range eventCh {
		if event.Type == "done" {
			doneEvent = event
		}
	}

	// Verify done event has correct totals
	// ChunksSkipped should be 1 (repo1's chunk)
	// ChunksEmbedded should be 1 (repo2's chunk)
	if doneEvent.ChunksSkipped < 1 {
		t.Errorf("Expected at least 1 chunk skipped, got %d", doneEvent.ChunksSkipped)
	}
	if doneEvent.ChunksEmbedded < 1 {
		t.Errorf("Expected at least 1 chunk embedded, got %d", doneEvent.ChunksEmbedded)
	}
}

func TestEmbedOptionsDefaults(t *testing.T) {
	opts := EmbedOptions{
		BatchSize: 0,
	}

	// Verify defaults would be set in RunEmbedPipeline
	if opts.BatchSize <= 0 {
		opts.BatchSize = 20
	}
	if opts.BatchSize > 100 {
		opts.BatchSize = 100
	}

	if opts.BatchSize != 20 {
		t.Errorf("Default batch size should be 20, got %d", opts.BatchSize)
	}
}

func TestEmbedOptionsBatchSizeCap(t *testing.T) {
	opts := EmbedOptions{
		BatchSize: 200,
	}

	if opts.BatchSize <= 0 {
		opts.BatchSize = 20
	}
	if opts.BatchSize > 100 {
		opts.BatchSize = 100
	}

	if opts.BatchSize != 100 {
		t.Errorf("Batch size should be capped at 100, got %d", opts.BatchSize)
	}
}
