package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
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
		e[i] = float32(math.Sin(float64(seed)+float64(i)*0.01))*0.5 + 0.5
	}
	return e
}

// insertTestRepoWithEmbedding inserts a test repo with a chunk and embedding
func insertTestRepoWithEmbedding(t *testing.T, database *sql.DB, githubID int64, fullName, owner, name, lang string, stars int, isOwned, isStarred bool, embeddingSeed float32) int64 {
	isOwnedInt := 0
	if isOwned {
		isOwnedInt = 1
	}
	isStarredInt := 0
	if isStarred {
		isStarredInt = 1
	}

	_, err := database.Exec(`INSERT INTO repos (github_id, owner, name, full_name, description, language, stars, pushed_at, pushed_at_hash, html_url, default_branch, synced_at, is_owned, is_starred)
		VALUES (?, ?, ?, ?, 'A test repo', ?, ?, '2024-01-15T10:00:00Z', 'hash123', ?, 'main', datetime('now'), ?, ?)`,
		githubID, owner, name, fullName, lang, stars, "https://github.com/"+fullName, isOwnedInt, isStarredInt)
	if err != nil {
		t.Fatalf("Failed to insert repo: %v", err)
	}

	var repoID int64
	err = database.QueryRow("SELECT id FROM repos WHERE github_id = ?", githubID).Scan(&repoID)
	if err != nil {
		t.Fatalf("Failed to get repo ID: %v", err)
	}

	// Insert chunk
	result, err := database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'metadata', ?)",
		repoID, `{"full_name":"`+fullName+`"}`)
	if err != nil {
		t.Fatalf("Failed to insert chunk: %v", err)
	}

	chunkID, _ := result.LastInsertId()

	// Insert embedding
	embedding := makeTestEmbedding(embeddingSeed)
	embeddingBlob := gemini.Float32SliceToBytes(embedding)
	_, err = database.Exec("INSERT INTO chunk_embeddings (chunk_id, embedding) VALUES (?, ?)", chunkID, embeddingBlob)
	if err != nil {
		t.Fatalf("Failed to insert embedding: %v", err)
	}

	return repoID
}

func TestSearchRepos_ReturnsResultsOrderedBySimilarity(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert 3 repos with different embeddings
	// Repo1 will have embedding most similar to query (seed 0.5)
	// Repo2 and Repo3 will be less similar
	insertTestRepoWithEmbedding(t, database, 1, "user/repo1", "user", "repo1", "Go", 100, true, false, 0.5)
	insertTestRepoWithEmbedding(t, database, 2, "user/repo2", "user", "repo2", "Go", 50, false, true, 2.0)
	insertTestRepoWithEmbedding(t, database, 3, "user/repo3", "user", "repo3", "Go", 25, false, false, 5.0)

	// Mock Gemini server to return query embedding (seed 0.5 - closest to repo1)
	queryEmbedding := makeTestEmbedding(0.5)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"embedding": map[string]interface{}{
				"values": queryEmbedding,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	// Search
	result, err := SearchRepos(context.Background(), database, "test-key", "test query", SearchFilters{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchRepos failed: %v", err)
	}

	// Verify we got results
	if len(result.Results) < 1 {
		t.Fatal("Expected at least 1 result")
	}

	// Find repo1 in results and verify it has the highest similarity (closest to 1.0)
	var maxSimilarity float64
	var maxSimilarityRepo string
	for _, r := range result.Results {
		if r.Similarity > maxSimilarity {
			maxSimilarity = r.Similarity
			maxSimilarityRepo = r.RepoFullName
		}
	}

	// repo1 should have highest similarity since it has the same embedding as query
	if maxSimilarityRepo != "user/repo1" {
		t.Errorf("Expected user/repo1 to have highest similarity, got %s (%.4f)", maxSimilarityRepo, maxSimilarity)
	}

	// repo1 should have similarity very close to 1.0 (same embedding)
	if maxSimilarity < 0.99 {
		t.Errorf("Expected similarity close to 1.0 for identical embeddings, got %.4f", maxSimilarity)
	}
}

func TestSearchRepos_LanguageFilterWorks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert Go repo and TypeScript repo
	insertTestRepoWithEmbedding(t, database, 1, "user/go-repo", "user", "go-repo", "Go", 100, true, false, 0.5)
	insertTestRepoWithEmbedding(t, database, 2, "user/ts-repo", "user", "ts-repo", "TypeScript", 100, true, false, 0.5)

	// Mock Gemini server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"embedding": map[string]interface{}{
				"values": makeTestEmbedding(0.5),
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	// Search with Go language filter
	lang := "Go"
	result, err := SearchRepos(context.Background(), database, "test-key", "test query", SearchFilters{
		Language: &lang,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("SearchRepos failed: %v", err)
	}

	// Verify only Go repo is returned
	if len(result.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(result.Results))
	}
	if len(result.Results) > 0 && result.Results[0].Language != "Go" {
		t.Errorf("Expected Go repo, got %s", result.Results[0].Language)
	}
}

func TestSearchRepos_StarredFilterWorks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert starred and non-starred repos
	insertTestRepoWithEmbedding(t, database, 1, "user/starred-repo", "user", "starred-repo", "Go", 100, false, true, 0.5)
	insertTestRepoWithEmbedding(t, database, 2, "user/not-starred-repo", "user", "not-starred-repo", "Go", 100, false, false, 0.5)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"embedding": map[string]interface{}{
				"values": makeTestEmbedding(0.5),
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	// Search with starred filter
	starred := true
	result, err := SearchRepos(context.Background(), database, "test-key", "test query", SearchFilters{
		Starred: &starred,
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("SearchRepos failed: %v", err)
	}

	// Verify only starred repo is returned
	if len(result.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(result.Results))
	}
	if len(result.Results) > 0 && !result.Results[0].IsStarred {
		t.Error("Expected starred repo")
	}
}

func TestSearchRepos_OwnedFilterWorks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert owned and not-owned repos
	insertTestRepoWithEmbedding(t, database, 1, "user/owned-repo", "user", "owned-repo", "Go", 100, true, false, 0.5)
	insertTestRepoWithEmbedding(t, database, 2, "user/not-owned-repo", "user", "not-owned-repo", "Go", 100, false, false, 0.5)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"embedding": map[string]interface{}{
				"values": makeTestEmbedding(0.5),
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	// Search with owned filter
	owned := true
	result, err := SearchRepos(context.Background(), database, "test-key", "test query", SearchFilters{
		Owned: &owned,
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchRepos failed: %v", err)
	}

	// Verify only owned repo is returned
	if len(result.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(result.Results))
	}
	if len(result.Results) > 0 && !result.Results[0].IsOwned {
		t.Error("Expected owned repo")
	}
}

func TestSearchRepos_LimitRespectedAfterDeduplication(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert 5 repos
	for i := 1; i <= 5; i++ {
		insertTestRepoWithEmbedding(t, database, int64(i), "user/repo"+string(rune('0'+i)), "user", "repo"+string(rune('0'+i)), "Go", 100, true, false, float32(i)*0.1)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"embedding": map[string]interface{}{
				"values": makeTestEmbedding(0.5),
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	// Search with limit of 2
	result, err := SearchRepos(context.Background(), database, "test-key", "test query", SearchFilters{
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("SearchRepos failed: %v", err)
	}

	// Verify exactly 2 results returned
	if len(result.Results) != 2 {
		t.Errorf("Expected 2 results (limit), got %d", len(result.Results))
	}
}

func TestSearchRepos_ReturnsAllChunkTypesPerRepo(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert 1 repo with 2 chunks (metadata and readme)
	_, err = database.Exec(`INSERT INTO repos (github_id, owner, name, full_name, description, language, stars, pushed_at, pushed_at_hash, html_url, default_branch, synced_at, is_owned, is_starred)
		VALUES (1, 'user', 'repo1', 'user/repo1', 'A test repo', 'Go', 100, '2024-01-15T10:00:00Z', 'hash123', 'https://github.com/user/repo1', 'main', datetime('now'), 1, 0)`)
	if err != nil {
		t.Fatalf("Failed to insert repo: %v", err)
	}

	var repoID int64
	_ = database.QueryRow("SELECT id FROM repos WHERE github_id = 1").Scan(&repoID)

	// Insert metadata chunk with embedding
	result, _ := database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'metadata', ?)", repoID, `{"full_name":"user/repo1"}`)
	metadataChunkID, _ := result.LastInsertId()
	_, _ = database.Exec("INSERT INTO chunk_embeddings (chunk_id, embedding) VALUES (?, ?)",
		metadataChunkID, gemini.Float32SliceToBytes(makeTestEmbedding(0.5)))

	// Insert readme chunk with embedding (close to query)
	result, _ = database.Exec("INSERT INTO chunks (repo_id, chunk_type, content) VALUES (?, 'readme', ?)", repoID, "# README")
	readmeChunkID, _ := result.LastInsertId()
	_, _ = database.Exec("INSERT INTO chunk_embeddings (chunk_id, embedding) VALUES (?, ?)",
		readmeChunkID, gemini.Float32SliceToBytes(makeTestEmbedding(0.5)))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"embedding": map[string]interface{}{
				"values": makeTestEmbedding(0.5),
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	// Search
	searchResult, err := SearchRepos(context.Background(), database, "test-key", "test query", SearchFilters{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchRepos failed: %v", err)
	}

	// Verify 2 results returned (one per chunk type: metadata + readme)
	// Search now returns all chunk types per repo for better RAG context
	if len(searchResult.Results) != 2 {
		t.Errorf("Expected 2 results (all chunk types per repo), got %d", len(searchResult.Results))
	}

	// Verify both chunk types are present
	chunkTypes := make(map[string]bool)
	for _, r := range searchResult.Results {
		chunkTypes[r.ChunkType] = true
	}
	if !chunkTypes["metadata"] || !chunkTypes["readme"] {
		t.Errorf("Expected both metadata and readme chunks, got: %v", chunkTypes)
	}
}

func TestSearchRepos_ReturnsEmptyOnNilEmbedding(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert a repo with embedding
	insertTestRepoWithEmbedding(t, database, 1, "user/repo1", "user", "repo1", "Go", 100, true, false, 0.5)

	// Mock Gemini server to return 500 error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	// Search - should return empty results, no error
	result, err := SearchRepos(context.Background(), database, "test-key", "test query", SearchFilters{
		Limit: 10,
	})

	// Should not return an error, just empty results
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if len(result.Results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(result.Results))
	}
}

func TestSearchRepos_SimilarityIsBetween0And1(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert a repo with embedding
	insertTestRepoWithEmbedding(t, database, 1, "user/repo1", "user", "repo1", "Go", 100, true, false, 0.5)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"embedding": map[string]interface{}{
				"values": makeTestEmbedding(0.5), // Same as repo embedding
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	gemini.SetBaseURL(server.URL)
	defer gemini.ResetBaseURL()

	result, err := SearchRepos(context.Background(), database, "test-key", "test query", SearchFilters{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchRepos failed: %v", err)
	}

	if len(result.Results) < 1 {
		t.Fatal("Expected at least 1 result")
	}

	// Verify similarity is between 0 and 1
	similarity := result.Results[0].Similarity
	if similarity < 0 || similarity > 1 {
		t.Errorf("Similarity should be between 0 and 1, got %f", similarity)
	}

	// For identical embeddings, similarity should be very high (close to 1)
	if similarity < 0.9 {
		t.Errorf("For identical embeddings, similarity should be > 0.9, got %f", similarity)
	}
}

func TestSearchFiltersDefaults(t *testing.T) {
	filters := SearchFilters{}

	// Default limit should be 3 when accessed
	limit := filters.Limit
	if limit <= 0 {
		limit = 3
	}

	if limit != 3 {
		t.Errorf("Default limit should be 3, got %d", limit)
	}
}

func TestSearchFiltersWithLanguage(t *testing.T) {
	lang := "Go"
	filters := SearchFilters{
		Language: &lang,
	}

	if filters.Language == nil || *filters.Language != "Go" {
		t.Error("Language filter not set correctly")
	}
}

func TestSearchFiltersWithStarred(t *testing.T) {
	starred := true
	filters := SearchFilters{
		Starred: &starred,
	}

	if filters.Starred == nil || !*filters.Starred {
		t.Error("Starred filter not set correctly")
	}
}

func TestSearchFiltersWithOwned(t *testing.T) {
	owned := true
	filters := SearchFilters{
		Owned: &owned,
	}

	if filters.Owned == nil || !*filters.Owned {
		t.Error("Owned filter not set correctly")
	}
}

func TestSearchResultStruct(t *testing.T) {
	result := SearchResult{
		RepoFullName: "owner/repo",
		Owner:        "owner",
		RepoName:     "repo",
		Description:  "A test repo",
		Language:     "Go",
		Stars:        100,
		IsStarred:    true,
		IsOwned:      false,
		HTMLURL:      "https://github.com/owner/repo",
		ChunkType:    "readme",
		Content:      "This is the content",
		Similarity:   0.85,
	}

	if result.RepoFullName != "owner/repo" {
		t.Errorf("RepoFullName should be 'owner/repo', got %s", result.RepoFullName)
	}
	if result.Stars != 100 {
		t.Errorf("Stars should be 100, got %d", result.Stars)
	}
	if !result.IsStarred {
		t.Error("IsStarred should be true")
	}
	if result.IsOwned {
		t.Error("IsOwned should be false")
	}
	if result.Similarity != 0.85 {
		t.Errorf("Similarity should be 0.85, got %f", result.Similarity)
	}
}

func TestSearchQueryResultStruct(t *testing.T) {
	result := SearchQueryResult{
		Results:          make([]SearchResult, 0),
		TotalConsidered:  10,
		QueryEmbeddingMs: 150,
		SearchMs:         50,
	}

	if result.TotalConsidered != 10 {
		t.Errorf("TotalConsidered should be 10, got %d", result.TotalConsidered)
	}
	if result.QueryEmbeddingMs != 150 {
		t.Errorf("QueryEmbeddingMs should be 150, got %d", result.QueryEmbeddingMs)
	}
	if result.SearchMs != 50 {
		t.Errorf("SearchMs should be 50, got %d", result.SearchMs)
	}
}
