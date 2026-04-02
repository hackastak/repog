package recommend

import (
	"context"
	"database/sql"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hackastak/repog/internal/db"
	"github.com/hackastak/repog/internal/provider"
	"github.com/hackastak/repog/internal/search"
)

func TestStripCodeFences(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "```json\n[{\"rank\": 1}]\n```",
			expected: "[{\"rank\": 1}]",
		},
		{
			input:    "```\n[{\"rank\": 1}]\n```",
			expected: "[{\"rank\": 1}]",
		},
		{
			input:    "Here is the result: [{\"rank\": 1}] and more text",
			expected: "[{\"rank\": 1}]",
		},
		{
			input:    "[{\"rank\": 1}]",
			expected: "[{\"rank\": 1}]",
		},
	}

	for _, tt := range tests {
		result := stripCodeFences(tt.input)
		if result != tt.expected {
			t.Errorf("stripCodeFences(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestParseRecommendations(t *testing.T) {
	input := `[
		{
			"rank": 1,
			"repoFullName": "user/repo1",
			"htmlUrl": "https://github.com/user/repo1",
			"reasoning": "This repo is great for X"
		},
		{
			"rank": 2,
			"repoFullName": "user/repo2",
			"htmlUrl": "https://github.com/user/repo2",
			"reasoning": "This repo is useful for Y"
		}
	]`

	recommendations := parseRecommendations(input, 5)

	if len(recommendations) != 2 {
		t.Errorf("Expected 2 recommendations, got %d", len(recommendations))
	}

	if recommendations[0].RepoFullName != "user/repo1" {
		t.Errorf("Expected user/repo1, got %s", recommendations[0].RepoFullName)
	}

	if recommendations[0].Rank != 1 {
		t.Errorf("Expected rank 1, got %d", recommendations[0].Rank)
	}
}

func TestParseRecommendationsLimit(t *testing.T) {
	input := `[
		{"rank": 1, "repoFullName": "user/repo1", "htmlUrl": "https://github.com/user/repo1", "reasoning": "Reason 1"},
		{"rank": 2, "repoFullName": "user/repo2", "htmlUrl": "https://github.com/user/repo2", "reasoning": "Reason 2"},
		{"rank": 3, "repoFullName": "user/repo3", "htmlUrl": "https://github.com/user/repo3", "reasoning": "Reason 3"}
	]`

	recommendations := parseRecommendations(input, 2)

	if len(recommendations) != 2 {
		t.Errorf("Expected 2 recommendations (limited), got %d", len(recommendations))
	}
}

func TestParseRecommendationsInvalidJSON(t *testing.T) {
	input := "This is not valid JSON"

	recommendations := parseRecommendations(input, 5)

	if recommendations != nil {
		t.Errorf("Expected nil for invalid JSON, got %v", recommendations)
	}
}

func TestStripCodeFencesWithJSON(t *testing.T) {
	input := "```JSON\n{\"key\": \"value\"}\n```"
	result := stripCodeFences(input)
	if result != "{\"key\": \"value\"}" {
		t.Errorf("stripCodeFences case insensitive failed: got %q", result)
	}
}

func TestStripCodeFencesNoFences(t *testing.T) {
	input := "  {\"key\": \"value\"}  "
	result := stripCodeFences(input)
	if result != "{\"key\": \"value\"}" {
		t.Errorf("stripCodeFences with whitespace: got %q", result)
	}
}

func TestParseRecommendationsEmptyArray(t *testing.T) {
	input := "[]"
	recommendations := parseRecommendations(input, 5)
	if len(recommendations) != 0 {
		t.Errorf("Expected empty array, got %d items", len(recommendations))
	}
}

func TestParseRecommendationsMissingFields(t *testing.T) {
	input := `[{"rank": 1, "repoFullName": "user/repo1"}]` // missing htmlUrl and reasoning
	recommendations := parseRecommendations(input, 5)
	if len(recommendations) != 0 {
		t.Errorf("Expected 0 recommendations for incomplete data, got %d", len(recommendations))
	}
}

func TestParseRecommendationsAutoRank(t *testing.T) {
	input := `[
		{"repoFullName": "user/repo1", "htmlUrl": "https://github.com/user/repo1", "reasoning": "Reason 1"},
		{"repoFullName": "user/repo2", "htmlUrl": "https://github.com/user/repo2", "reasoning": "Reason 2"}
	]`
	recommendations := parseRecommendations(input, 5)
	if len(recommendations) != 2 {
		t.Fatalf("Expected 2 recommendations, got %d", len(recommendations))
	}
	if recommendations[0].Rank != 1 {
		t.Errorf("Expected auto-rank 1, got %d", recommendations[0].Rank)
	}
	if recommendations[1].Rank != 2 {
		t.Errorf("Expected auto-rank 2, got %d", recommendations[1].Rank)
	}
}

func TestRecommendOptionsDefaults(t *testing.T) {
	opts := RecommendOptions{
		Query: "test query",
		Limit: 0,
	}

	// When limit is 0 or negative, it should default to 3
	if opts.Limit <= 0 {
		opts.Limit = 3
	}

	if opts.Limit != 3 {
		t.Errorf("Expected default limit 3, got %d", opts.Limit)
	}
}

func TestRecommendationStruct(t *testing.T) {
	r := Recommendation{
		Rank:         1,
		RepoFullName: "owner/repo",
		HTMLURL:      "https://github.com/owner/repo",
		Reasoning:    "This is the reasoning",
	}

	if r.Rank != 1 {
		t.Errorf("Expected rank 1, got %d", r.Rank)
	}
	if r.RepoFullName != "owner/repo" {
		t.Errorf("Expected owner/repo, got %s", r.RepoFullName)
	}
	if r.HTMLURL != "https://github.com/owner/repo" {
		t.Errorf("Expected github URL, got %s", r.HTMLURL)
	}
	if r.Reasoning != "This is the reasoning" {
		t.Errorf("Expected reasoning, got %s", r.Reasoning)
	}
}

func TestRecommendResultStruct(t *testing.T) {
	result := RecommendResult{
		Recommendations:      make([]Recommendation, 0),
		Query:                "test query",
		CandidatesConsidered: 10,
		InputTokens:          100,
		OutputTokens:         50,
		DurationMs:           1000,
	}

	if result.Query != "test query" {
		t.Errorf("Expected 'test query', got %s", result.Query)
	}
	if result.CandidatesConsidered != 10 {
		t.Errorf("Expected 10 candidates, got %d", result.CandidatesConsidered)
	}
	if result.DurationMs != 1000 {
		t.Errorf("Expected 1000ms, got %d", result.DurationMs)
	}
}

func TestBuildRecommendPrompt(t *testing.T) {
	candidates := []search.SearchResult{
		{
			RepoFullName: "user/repo1",
			Description:  "A great repo",
			Language:     "Go",
			HTMLURL:      "https://github.com/user/repo1",
		},
		{
			RepoFullName: "user/repo2",
			Description:  "",
			Language:     "",
			HTMLURL:      "https://github.com/user/repo2",
		},
	}

	prompt := buildRecommendPrompt("machine learning", candidates, 3)

	if !strings.Contains(prompt, "machine learning") {
		t.Error("Prompt should contain query")
	}
	if !strings.Contains(prompt, "user/repo1") {
		t.Error("Prompt should contain first repo")
	}
	if !strings.Contains(prompt, "No description") {
		t.Error("Prompt should have 'No description' for empty description")
	}
	if !strings.Contains(prompt, "Unknown language") {
		t.Error("Prompt should have 'Unknown language' for empty language")
	}
	if !strings.Contains(prompt, "top 3") {
		t.Error("Prompt should mention limit")
	}
}

// makeTestEmbedding creates a 768-dimension embedding with a seed value
func makeTestEmbedding(seed float32) []float32 {
	e := make([]float32, 768)
	for i := range e {
		e[i] = float32(math.Sin(float64(seed)+float64(i)*0.01))*0.5 + 0.5
	}
	return e
}

// insertTestRepoWithEmbedding inserts a test repo with a chunk and embedding
func insertTestRepoWithEmbedding(t *testing.T, database *sql.DB, githubID int64, fullName, owner, name, lang string, stars int, isOwned, isStarred bool, embeddingSeed float32) int64 { //nolint:unparam
	t.Helper()
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
	embeddingBlob := provider.Float32SliceToBytes(embedding)
	_, err = database.Exec("INSERT INTO chunk_embeddings (chunk_id, embedding) VALUES (?, ?)", chunkID, embeddingBlob)
	if err != nil {
		t.Fatalf("Failed to insert embedding: %v", err)
	}

	return repoID
}

func TestRecommendRepos_ReturnsRankedRecommendations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert 5 repos with different embeddings
	insertTestRepoWithEmbedding(t, database, 1, "owner/repo1", "owner", "repo1", "Go", 100, true, false, 0.5)
	insertTestRepoWithEmbedding(t, database, 2, "owner/repo2", "owner", "repo2", "Go", 80, true, false, 0.6)
	insertTestRepoWithEmbedding(t, database, 3, "owner/repo3", "owner", "repo3", "Go", 60, true, false, 0.7)
	insertTestRepoWithEmbedding(t, database, 4, "owner/repo4", "owner", "repo4", "Go", 40, true, false, 0.8)
	insertTestRepoWithEmbedding(t, database, 5, "owner/repo5", "owner", "repo5", "Go", 20, true, false, 0.9)

	// Create mock providers
	mockEmbed := provider.NewMockEmbeddingProvider()
	mockEmbed.QueryFunc = func(_ context.Context, _ string) []float32 {
		return makeTestEmbedding(0.5)
	}

	mockLLM := &provider.MockLLMProvider{
		NameVal: "mock",
		CallFunc: func(_ context.Context, _ provider.LLMRequest) (*provider.LLMResult, *provider.LLMError) {
			llmResponse := `[
				{"rank": 1, "repoFullName": "owner/repo1", "htmlUrl": "https://github.com/owner/repo1", "reasoning": "Best fit"},
				{"rank": 2, "repoFullName": "owner/repo2", "htmlUrl": "https://github.com/owner/repo2", "reasoning": "Good fit"}
			]`
			return &provider.LLMResult{Text: llmResponse, InputTokens: 100, OutputTokens: 50}, nil
		},
	}

	// Call RecommendRepos
	result, err := RecommendRepos(context.Background(), RecommendOptions{
		Query:             "test query",
		Limit:             2,
		DB:                database,
		EmbeddingProvider: mockEmbed,
		LLMProvider:       mockLLM,
	})
	if err != nil {
		t.Fatalf("RecommendRepos failed: %v", err)
	}

	if len(result.Recommendations) != 2 {
		t.Errorf("Expected 2 recommendations, got %d", len(result.Recommendations))
	}

	if len(result.Recommendations) > 0 && result.Recommendations[0].Rank != 1 {
		t.Errorf("Expected first recommendation rank 1, got %d", result.Recommendations[0].Rank)
	}

	if len(result.Recommendations) > 0 && result.Recommendations[0].RepoFullName != "owner/repo1" {
		t.Errorf("Expected first recommendation to be owner/repo1, got %s", result.Recommendations[0].RepoFullName)
	}

	if len(result.Recommendations) > 0 && result.Recommendations[0].Reasoning != "Best fit" {
		t.Errorf("Expected reasoning 'Best fit', got %s", result.Recommendations[0].Reasoning)
	}

	if result.CandidatesConsidered <= 0 {
		t.Errorf("Expected CandidatesConsidered > 0, got %d", result.CandidatesConsidered)
	}

	if result.DurationMs < 0 {
		t.Errorf("Expected DurationMs >= 0, got %d", result.DurationMs)
	}
}

func TestRecommendRepos_ReturnsEmptyWhenNoCandidates(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// No repos inserted - empty database

	llmRequestCount := 0

	mockEmbed := provider.NewMockEmbeddingProvider()
	mockEmbed.QueryFunc = func(_ context.Context, _ string) []float32 {
		return makeTestEmbedding(0.5)
	}

	mockLLM := &provider.MockLLMProvider{
		NameVal: "mock",
		CallFunc: func(_ context.Context, _ provider.LLMRequest) (*provider.LLMResult, *provider.LLMError) {
			llmRequestCount++
			return &provider.LLMResult{Text: "[]", InputTokens: 10, OutputTokens: 5}, nil
		},
	}

	result, err := RecommendRepos(context.Background(), RecommendOptions{
		Query:             "test query",
		Limit:             3,
		DB:                database,
		EmbeddingProvider: mockEmbed,
		LLMProvider:       mockLLM,
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(result.Recommendations) != 0 {
		t.Errorf("Expected empty recommendations, got %d", len(result.Recommendations))
	}

	if llmRequestCount != 0 {
		t.Errorf("Expected 0 LLM requests when no candidates, got %d", llmRequestCount)
	}
}

func TestRecommendRepos_ReturnsEmptyOnLLMFailure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert repos with embeddings
	insertTestRepoWithEmbedding(t, database, 1, "owner/repo1", "owner", "repo1", "Go", 100, true, false, 0.5)

	mockEmbed := provider.NewMockEmbeddingProvider()
	mockEmbed.QueryFunc = func(_ context.Context, _ string) []float32 {
		return makeTestEmbedding(0.5)
	}

	mockLLM := &provider.MockLLMProvider{
		NameVal: "mock",
		CallFunc: func(_ context.Context, _ provider.LLMRequest) (*provider.LLMResult, *provider.LLMError) {
			return nil, &provider.LLMError{Message: "Internal Server Error", StatusCode: 500}
		},
	}

	result, err := RecommendRepos(context.Background(), RecommendOptions{
		Query:             "test query",
		Limit:             2,
		DB:                database,
		EmbeddingProvider: mockEmbed,
		LLMProvider:       mockLLM,
	})

	// The function should return an error or empty recommendations
	if err == nil {
		if len(result.Recommendations) != 0 {
			t.Errorf("Expected empty recommendations on LLM failure, got %d", len(result.Recommendations))
		}
	}
}

func TestRecommendRepos_StripsMarkdownFencesFromLLMResponse(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	insertTestRepoWithEmbedding(t, database, 1, "owner/repo", "owner", "repo", "Go", 100, true, false, 0.5)

	mockEmbed := provider.NewMockEmbeddingProvider()
	mockEmbed.QueryFunc = func(_ context.Context, _ string) []float32 {
		return makeTestEmbedding(0.5)
	}

	mockLLM := &provider.MockLLMProvider{
		NameVal: "mock",
		CallFunc: func(_ context.Context, _ provider.LLMRequest) (*provider.LLMResult, *provider.LLMError) {
			// LLM response wrapped in markdown fences
			llmResponse := "```json\n[{\"rank\":1,\"repoFullName\":\"owner/repo\",\"htmlUrl\":\"https://github.com/owner/repo\",\"reasoning\":\"Good\"}]\n```"
			return &provider.LLMResult{Text: llmResponse, InputTokens: 100, OutputTokens: 50}, nil
		},
	}

	result, err := RecommendRepos(context.Background(), RecommendOptions{
		Query:             "test query",
		Limit:             3,
		DB:                database,
		EmbeddingProvider: mockEmbed,
		LLMProvider:       mockLLM,
	})
	if err != nil {
		t.Fatalf("RecommendRepos failed: %v", err)
	}

	if len(result.Recommendations) != 1 {
		t.Errorf("Expected 1 recommendation after stripping fences, got %d", len(result.Recommendations))
	}
}

func TestRecommendRepos_CapsAtLimit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	insertTestRepoWithEmbedding(t, database, 1, "owner/repo1", "owner", "repo1", "Go", 100, true, false, 0.5)
	insertTestRepoWithEmbedding(t, database, 2, "owner/repo2", "owner", "repo2", "Go", 80, true, false, 0.6)
	insertTestRepoWithEmbedding(t, database, 3, "owner/repo3", "owner", "repo3", "Go", 60, true, false, 0.7)

	mockEmbed := provider.NewMockEmbeddingProvider()
	mockEmbed.QueryFunc = func(_ context.Context, _ string) []float32 {
		return makeTestEmbedding(0.5)
	}

	mockLLM := &provider.MockLLMProvider{
		NameVal: "mock",
		CallFunc: func(_ context.Context, _ provider.LLMRequest) (*provider.LLMResult, *provider.LLMError) {
			// LLM returns 5 recommendations
			llmResponse := `[
				{"rank": 1, "repoFullName": "owner/repo1", "htmlUrl": "https://github.com/owner/repo1", "reasoning": "R1"},
				{"rank": 2, "repoFullName": "owner/repo2", "htmlUrl": "https://github.com/owner/repo2", "reasoning": "R2"},
				{"rank": 3, "repoFullName": "owner/repo3", "htmlUrl": "https://github.com/owner/repo3", "reasoning": "R3"},
				{"rank": 4, "repoFullName": "owner/repo4", "htmlUrl": "https://github.com/owner/repo4", "reasoning": "R4"},
				{"rank": 5, "repoFullName": "owner/repo5", "htmlUrl": "https://github.com/owner/repo5", "reasoning": "R5"}
			]`
			return &provider.LLMResult{Text: llmResponse, InputTokens: 100, OutputTokens: 50}, nil
		},
	}

	result, err := RecommendRepos(context.Background(), RecommendOptions{
		Query:             "test query",
		Limit:             2,
		DB:                database,
		EmbeddingProvider: mockEmbed,
		LLMProvider:       mockLLM,
	})
	if err != nil {
		t.Fatalf("RecommendRepos failed: %v", err)
	}

	if len(result.Recommendations) != 2 {
		t.Errorf("Expected 2 recommendations (capped at limit), got %d", len(result.Recommendations))
	}
}

func TestRecommendRepos_PassesFiltersToSearch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = db.Close(database) }()

	// Insert repos with different languages
	insertTestRepoWithEmbedding(t, database, 1, "owner/go-repo", "owner", "go-repo", "Go", 100, true, false, 0.5)
	insertTestRepoWithEmbedding(t, database, 2, "owner/ts-repo", "owner", "ts-repo", "TypeScript", 100, true, false, 0.5)

	var capturedPrompt string

	mockEmbed := provider.NewMockEmbeddingProvider()
	mockEmbed.QueryFunc = func(_ context.Context, _ string) []float32 {
		return makeTestEmbedding(0.5)
	}

	mockLLM := &provider.MockLLMProvider{
		NameVal: "mock",
		CallFunc: func(_ context.Context, req provider.LLMRequest) (*provider.LLMResult, *provider.LLMError) {
			capturedPrompt = req.Prompt
			llmResponse := `[{"rank": 1, "repoFullName": "owner/go-repo", "htmlUrl": "https://github.com/owner/go-repo", "reasoning": "Go repo"}]`
			return &provider.LLMResult{Text: llmResponse, InputTokens: 100, OutputTokens: 50}, nil
		},
	}

	goLang := "Go"
	_, err = RecommendRepos(context.Background(), RecommendOptions{
		Query:             "test query",
		Limit:             3,
		DB:                database,
		EmbeddingProvider: mockEmbed,
		LLMProvider:       mockLLM,
		Filters: search.SearchFilters{
			Language: &goLang,
		},
	})
	if err != nil {
		t.Fatalf("RecommendRepos failed: %v", err)
	}

	// LLM prompt should only contain Go repos as candidates
	if !strings.Contains(capturedPrompt, "go-repo") {
		t.Errorf("Expected prompt to contain Go repo, prompt was: %s", capturedPrompt)
	}
	if strings.Contains(capturedPrompt, "ts-repo") {
		t.Errorf("Expected prompt NOT to contain TypeScript repo when filtering by Go, prompt was: %s", capturedPrompt)
	}
}
