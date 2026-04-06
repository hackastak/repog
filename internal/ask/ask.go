// Package ask provides RAG-based Q&A functionality for repositories.
package ask

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/hackastak/repog/internal/provider"
	"github.com/hackastak/repog/internal/search"
)

// AskOptions configures the Q&A query.
type AskOptions struct {
	Question          string
	Repo              string // optional full_name filter
	Limit             int    // default 10
	DB                *sql.DB
	EmbeddingProvider provider.EmbeddingProvider
	LLMProvider       provider.LLMProvider
}

// SourceAttribution represents a source used in the answer.
type SourceAttribution struct {
	RepoFullName string
	ChunkType    string
	Similarity   float64
}

// AskResult contains the Q&A result.
type AskResult struct {
	Answer       string
	Sources      []SourceAttribution
	Question     string
	InputTokens  int
	OutputTokens int
	DurationMs   int64
}

const systemPrompt = `You are a helpful assistant that answers questions about GitHub repositories.
Answer based only on the provided context. If the context does not contain enough
information to answer the question, say so clearly. Be concise and precise.`

// buildAskPrompt builds the user prompt for Q&A.
func buildAskPrompt(question string, chunks []search.SearchResult) string {
	formattedChunks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		formattedChunks = append(formattedChunks, fmt.Sprintf("--- %s (%s) ---\n%s", chunk.RepoFullName, chunk.ChunkType, chunk.Content))
	}

	context := strings.Join(formattedChunks, "\n\n")

	return fmt.Sprintf(`Question: %s

Context from repositories:
%s

Answer the question based on the context above. If you reference specific
repositories, mention them by name.`, question, context)
}

// buildSourceAttributions creates source attributions from search results.
func buildSourceAttributions(results []search.SearchResult) []SourceAttribution {
	seen := make(map[string]SourceAttribution)

	for _, result := range results {
		existing, exists := seen[result.RepoFullName]
		if !exists || result.Similarity > existing.Similarity {
			seen[result.RepoFullName] = SourceAttribution{
				RepoFullName: result.RepoFullName,
				ChunkType:    result.ChunkType,
				Similarity:   result.Similarity,
			}
		}
	}

	// Convert to slice and cap at 5
	sources := make([]SourceAttribution, 0, len(seen))
	for _, s := range seen {
		sources = append(sources, s)
		if len(sources) >= 5 {
			break
		}
	}

	return sources
}

// AskQuestion performs RAG-based Q&A on repositories.
func AskQuestion(ctx context.Context, opts AskOptions, onChunk func(string)) (AskResult, error) {
	start := time.Now()
	result := AskResult{
		Question: opts.Question,
		Sources:  make([]SourceAttribution, 0),
	}

	// Set default limit
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}

	// Search for relevant chunks
	searchResult, err := search.SearchRepos(ctx, opts.DB, opts.EmbeddingProvider, opts.Question, search.SearchFilters{
		Limit: limit,
	})
	if err != nil {
		result.DurationMs = time.Since(start).Milliseconds()
		return result, err
	}

	// Filter by repo if specified
	var results []search.SearchResult
	if opts.Repo != "" {
		for _, r := range searchResult.Results {
			if strings.EqualFold(r.RepoFullName, opts.Repo) {
				results = append(results, r)
			}
		}
	} else {
		results = searchResult.Results
	}

	// Handle empty results
	if len(results) == 0 {
		result.Answer = "I couldn't find any relevant information in your knowledge base to answer this question."
		if onChunk != nil {
			onChunk(result.Answer)
		}
		result.DurationMs = time.Since(start).Milliseconds()
		return result, nil
	}

	// Build prompt
	prompt := buildAskPrompt(opts.Question, results)

	// Stream LLM response
	llmResult, llmErr := opts.LLMProvider.Stream(ctx, provider.LLMRequest{
		Prompt:       prompt,
		SystemPrompt: systemPrompt,
	}, onChunk)

	result.DurationMs = time.Since(start).Milliseconds()

	if llmErr != nil {
		result.Answer = fmt.Sprintf("Error generating answer: %s", llmErr.Message)
		return result, nil
	}

	result.Answer = llmResult.Text
	result.InputTokens = llmResult.InputTokens
	result.OutputTokens = llmResult.OutputTokens
	result.Sources = buildSourceAttributions(results)

	return result, nil
}
