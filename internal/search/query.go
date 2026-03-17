// Package search handles vector similarity search for repositories.
package search

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	"github.com/hackastak/repog/internal/gemini"
)

// SearchFilters contains optional filters for search queries.
type SearchFilters struct {
	Language *string
	Starred  *bool
	Owned    *bool
	Owner    *string
	Limit    int // default 3
}

// SearchResult represents a single search result.
type SearchResult struct {
	RepoFullName string
	Owner        string
	RepoName     string
	Description  string
	Language     string
	Stars        int
	IsStarred    bool
	IsOwned      bool
	HTMLURL      string
	ChunkType    string
	Content      string
	Similarity   float64
}

// SearchQueryResult contains the search results and timing metrics.
type SearchQueryResult struct {
	Results          []SearchResult
	TotalConsidered  int
	QueryEmbeddingMs int64
	SearchMs         int64
}

// SearchRepos embeds the query and performs a cosine similarity search
// against chunk_embeddings. Results are deduplicated by repo (highest
// similarity chunk per repo), then capped at Limit.
func SearchRepos(ctx context.Context, db *sql.DB, apiKey, query string, filters SearchFilters) (SearchQueryResult, error) {
	result := SearchQueryResult{
		Results: make([]SearchResult, 0),
	}

	// Set default limit
	limit := filters.Limit
	if limit <= 0 {
		limit = 3
	}

	// Embed the query
	embedStart := time.Now()
	embedding := gemini.EmbedQuery(ctx, apiKey, query)
	result.QueryEmbeddingMs = time.Since(embedStart).Milliseconds()

	if embedding == nil {
		return result, nil
	}

	// Convert embedding to bytes for sqlite-vec
	embeddingBytes := gemini.Float32SliceToBytes(embedding)

	// Build dynamic WHERE clause
	var whereClauses []string
	var args []interface{}
	args = append(args, embeddingBytes)

	if filters.Language != nil {
		whereClauses = append(whereClauses, "LOWER(r.language) = LOWER(?)")
		args = append(args, *filters.Language)
	}

	if filters.Starred != nil && *filters.Starred {
		whereClauses = append(whereClauses, "r.is_starred = 1")
	}

	if filters.Owned != nil && *filters.Owned {
		whereClauses = append(whereClauses, "r.is_owned = 1")
	}

	if filters.Owner != nil {
		whereClauses = append(whereClauses, "LOWER(r.owner) = LOWER(?)")
		args = append(args, *filters.Owner)
	}

	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = "AND " + strings.Join(whereClauses, " AND ")
	}

	// Fetch extra rows for deduplication
	fetchLimit := limit * 5
	args = append(args, fetchLimit)

	sqlQuery := `
		SELECT
			r.full_name,
			r.owner,
			r.name,
			COALESCE(r.description, ''),
			COALESCE(r.language, ''),
			r.stars,
			r.is_starred,
			r.is_owned,
			r.html_url,
			c.chunk_type,
			c.content,
			vec_distance_cosine(ce.embedding, ?) as distance
		FROM chunk_embeddings ce
		JOIN chunks c ON c.id = ce.chunk_id
		JOIN repos r ON r.id = c.repo_id
		WHERE 1=1 ` + whereClause + `
		ORDER BY distance ASC
		LIMIT ?
	`

	// Execute search
	searchStart := time.Now()
	rows, err := db.QueryContext(ctx, sqlQuery, args...)
	result.SearchMs = time.Since(searchStart).Milliseconds()

	if err != nil {
		return result, err
	}
	defer func() { _ = rows.Close() }()

	// Collect results and deduplicate
	seenRepos := make(map[string]SearchResult)

	for rows.Next() {
		var r SearchResult
		var isStarred, isOwned int
		var distance float64

		err := rows.Scan(
			&r.RepoFullName, &r.Owner, &r.RepoName, &r.Description,
			&r.Language, &r.Stars, &isStarred, &isOwned,
			&r.HTMLURL, &r.ChunkType, &r.Content, &distance,
		)
		if err != nil {
			continue
		}

		r.IsStarred = isStarred == 1
		r.IsOwned = isOwned == 1
		r.Similarity = 1 - distance

		result.TotalConsidered++

		// Keep highest similarity per repo
		if _, exists := seenRepos[r.RepoFullName]; !exists {
			seenRepos[r.RepoFullName] = r
		}
	}

	// Convert to slice, sort by similarity (descending), and cap at limit
	for _, r := range seenRepos {
		result.Results = append(result.Results, r)
	}
	sort.Slice(result.Results, func(i, j int) bool {
		return result.Results[i].Similarity > result.Results[j].Similarity
	})
	if len(result.Results) > limit {
		result.Results = result.Results[:limit]
	}

	return result, nil
}
