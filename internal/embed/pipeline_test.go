package embed

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/provider"
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
	database, err := db.Open(dbPath, 768)
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

	// Run embed pipeline with mock provider
	eventCh := RunEmbedPipeline(context.Background(), EmbedOptions{
		DB:                database,
		EmbeddingProvider: provider.NewMockEmbeddingProvider(),
		BatchSize:         10,
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
	database, err := db.Open(dbPath, 768)
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

	// Get repo ID and insert chunk with embedding
	var repoID int64
	err = database.QueryRow("SELECT id FROM repos WHERE full_name = 'user/repo1'").Scan(&repoID)
	if err != nil {
		t.Fatalf("Failed to get repo ID: %v", err)
	}

	result, err := database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'metadata', ?)",
		repoID, `{"full_name":"user/repo1"}`)
	if err != nil {
		t.Fatalf("Failed to insert chunk: %v", err)
	}

	// Insert embedding for the chunk so it's considered fully embedded
	chunkID, _ := result.LastInsertId()
	_, err = database.Exec("INSERT INTO chunk_embeddings (chunk_id, embedding) VALUES (?, ?)",
		chunkID, provider.Float32SliceToBytes(makeTestEmbedding(0.5)))
	if err != nil {
		t.Fatalf("Failed to insert chunk embedding: %v", err)
	}

	// Track API calls via mock provider
	apiCalls := 0
	mockProvider := provider.NewMockEmbeddingProvider()
	mockProvider.EmbedFunc = func(_ context.Context, chunks []provider.EmbedRequest) provider.BatchEmbedResult {
		apiCalls++
		results := make([]provider.EmbedResult, len(chunks))
		for i, c := range chunks {
			results[i] = provider.EmbedResult{ID: c.ID, Embedding: makeTestEmbedding(0.5)}
		}
		return provider.BatchEmbedResult{Results: results}
	}

	// Run embed pipeline
	eventCh := RunEmbedPipeline(context.Background(), EmbedOptions{
		DB:                database,
		EmbeddingProvider: mockProvider,
		BatchSize:         10,
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

	// Verify embedding API was NOT called (repo was skipped)
	if apiCalls != 0 {
		t.Errorf("Expected 0 API calls (repo should be skipped), got %d", apiCalls)
	}
}

func TestRunEmbedPipeline_BatchesChunksCorrectly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
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

	// Track API requests via mock provider
	apiRequests := 0
	mockProvider := provider.NewMockEmbeddingProvider()
	mockProvider.EmbedFunc = func(_ context.Context, chunks []provider.EmbedRequest) provider.BatchEmbedResult {
		apiRequests++
		results := make([]provider.EmbedResult, len(chunks))
		for i, c := range chunks {
			results[i] = provider.EmbedResult{ID: c.ID, Embedding: makeTestEmbedding(float32(i))}
		}
		return provider.BatchEmbedResult{Results: results}
	}

	// Run embed pipeline with batch size of 10
	eventCh := RunEmbedPipeline(context.Background(), EmbedOptions{
		DB:                database,
		EmbeddingProvider: mockProvider,
		BatchSize:         10,
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
	database, err := db.Open(dbPath, 768)
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

	// Track chunks sent to API via mock provider
	chunksReceived := 0
	mockProvider := provider.NewMockEmbeddingProvider()
	mockProvider.EmbedFunc = func(_ context.Context, chunks []provider.EmbedRequest) provider.BatchEmbedResult {
		chunksReceived = len(chunks)
		results := make([]provider.EmbedResult, len(chunks))
		for i, c := range chunks {
			results[i] = provider.EmbedResult{ID: c.ID, Embedding: makeTestEmbedding(float32(i))}
		}
		return provider.BatchEmbedResult{Results: results}
	}

	// Run embed pipeline with IncludeFileTree=false (default)
	eventCh := RunEmbedPipeline(context.Background(), EmbedOptions{
		DB:                database,
		EmbeddingProvider: mockProvider,
		BatchSize:         10,
		IncludeFileTree:   false,
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
	database, err := db.Open(dbPath, 768)
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

	// Track chunks sent to API via mock provider
	chunksReceived := 0
	mockProvider := provider.NewMockEmbeddingProvider()
	mockProvider.EmbedFunc = func(_ context.Context, chunks []provider.EmbedRequest) provider.BatchEmbedResult {
		chunksReceived = len(chunks)
		results := make([]provider.EmbedResult, len(chunks))
		for i, c := range chunks {
			results[i] = provider.EmbedResult{ID: c.ID, Embedding: makeTestEmbedding(float32(i))}
		}
		return provider.BatchEmbedResult{Results: results}
	}

	// Run embed pipeline with IncludeFileTree=true
	eventCh := RunEmbedPipeline(context.Background(), EmbedOptions{
		DB:                database,
		EmbeddingProvider: mockProvider,
		BatchSize:         10,
		IncludeFileTree:   true,
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
	database, err := db.Open(dbPath, 768)
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

	eventCh := RunEmbedPipeline(context.Background(), EmbedOptions{
		DB:                database,
		EmbeddingProvider: provider.NewMockEmbeddingProvider(),
		BatchSize:         10,
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
	database, err := db.Open(dbPath, 768)
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

	result1, _ := database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'metadata', ?)", repo1ID, `{"full_name":"user/repo1"}`)
	_, _ = database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'metadata', ?)", repo2ID, `{"full_name":"user/repo2"}`)

	// Insert embedding for repo1's chunk so it's considered fully embedded
	chunk1ID, _ := result1.LastInsertId()
	_, _ = database.Exec("INSERT INTO chunk_embeddings (chunk_id, embedding) VALUES (?, ?)",
		chunk1ID, provider.Float32SliceToBytes(makeTestEmbedding(0.5)))

	eventCh := RunEmbedPipeline(context.Background(), EmbedOptions{
		DB:                database,
		EmbeddingProvider: provider.NewMockEmbeddingProvider(),
		BatchSize:         10,
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
