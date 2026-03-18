// Package embed handles embedding generation for repository chunks.
package embed

import (
	"context"
	"database/sql"
	"time"

	"github.com/hackastak/repog/internal/gemini"
)

// EmbedOptions configures the embedding pipeline.
type EmbedOptions struct {
	IncludeFileTree bool
	BatchSize       int // default 20, max 100
	DB              *sql.DB
	GeminiAPIKey    string
}

// EmbedEvent represents a progress event from the embedding pipeline.
type EmbedEvent struct {
	Type           string   // "batch", "repo_skip", "error", "done"
	RepoFullName   string   // for repo_skip
	BatchIndex     int
	BatchTotal     int
	ChunksEmbedded int
	ChunksSkipped  int
	ChunksErrored  int
	TotalChunks    int
	Errors         []string // error messages from the current batch
}

// chunkRecord represents a chunk from the database.
type chunkRecord struct {
	ID      int64
	RepoID  int64
	Content string
}

// repoRecord represents a repo from the database.
type repoRecord struct {
	ID           int64
	FullName     string
	PushedAtHash string
	EmbeddedHash sql.NullString
}

// RunEmbedPipeline embeds all un-embedded chunks.
// Skips repos where embedded_hash == pushed_at_hash.
// Events are sent to the returned channel.
func RunEmbedPipeline(ctx context.Context, opts EmbedOptions) <-chan EmbedEvent {
	eventCh := make(chan EmbedEvent, 100)

	// Set defaults
	if opts.BatchSize <= 0 {
		opts.BatchSize = 20
	}
	if opts.BatchSize > 100 {
		opts.BatchSize = 100
	}

	go func() {
		defer close(eventCh)

		var chunksEmbedded, chunksSkipped, chunksErrored int

		// Get repos that need embedding
		rows, err := opts.DB.Query(`
			SELECT id, full_name, pushed_at_hash, embedded_hash
			FROM repos
			WHERE pushed_at_hash IS NOT NULL
		`)
		if err != nil {
			eventCh <- EmbedEvent{Type: "error", RepoFullName: "database error: " + err.Error()}
			return
		}

		var repos []repoRecord
		for rows.Next() {
			var r repoRecord
			if err := rows.Scan(&r.ID, &r.FullName, &r.PushedAtHash, &r.EmbeddedHash); err != nil {
				continue
			}
			repos = append(repos, r)
		}
		_ = rows.Close()

		if len(repos) == 0 {
			eventCh <- EmbedEvent{Type: "done", TotalChunks: 0}
			return
		}

		// Identify repos to skip vs process
		var reposToProcess []repoRecord
		for _, repo := range repos {
			// Build chunk type filter
			chunkTypeFilter := ""
			if !opts.IncludeFileTree {
				chunkTypeFilter = " AND chunk_type != 'file_tree'"
			}

			// Check if there are any chunks without embeddings
			var unembeddedCount int
			_ = opts.DB.QueryRow(`
				SELECT COUNT(*) FROM chunks c
				WHERE c.repo_id = ? `+chunkTypeFilter+`
				AND NOT EXISTS (SELECT 1 FROM chunk_embeddings ce WHERE ce.chunk_id = c.id)
			`, repo.ID).Scan(&unembeddedCount)

			if unembeddedCount == 0 && repo.EmbeddedHash.Valid && repo.EmbeddedHash.String == repo.PushedAtHash {
				// Skip - all chunks already embedded
				var count int
				_ = opts.DB.QueryRow(
					"SELECT COUNT(*) FROM chunks WHERE repo_id = ?"+chunkTypeFilter,
					repo.ID,
				).Scan(&count)

				chunksSkipped += count
				eventCh <- EmbedEvent{
					Type:           "repo_skip",
					RepoFullName:   repo.FullName,
					ChunksSkipped:  chunksSkipped,
					ChunksEmbedded: chunksEmbedded,
				}
			} else {
				reposToProcess = append(reposToProcess, repo)
			}
		}

		if len(reposToProcess) == 0 {
			eventCh <- EmbedEvent{
				Type:          "done",
				ChunksSkipped: chunksSkipped,
				TotalChunks:   chunksSkipped,
			}
			return
		}

		// Collect un-embedded chunks from repos to process
		var chunks []chunkRecord
		for _, repo := range reposToProcess {
			chunkTypeFilter := ""
			if !opts.IncludeFileTree {
				chunkTypeFilter = " AND chunk_type != 'file_tree'"
			}

			// Only get chunks that don't have embeddings yet
			chunkRows, err := opts.DB.Query(`
				SELECT c.id, c.repo_id, c.content FROM chunks c
				WHERE c.repo_id = ? `+chunkTypeFilter+`
				AND NOT EXISTS (SELECT 1 FROM chunk_embeddings ce WHERE ce.chunk_id = c.id)
			`, repo.ID)
			if err != nil {
				continue
			}

			for chunkRows.Next() {
				var c chunkRecord
				if err := chunkRows.Scan(&c.ID, &c.RepoID, &c.Content); err != nil {
					continue
				}
				chunks = append(chunks, c)
			}
			_ = chunkRows.Close()
		}

		totalChunks := len(chunks) + chunksSkipped

		if len(chunks) == 0 {
			eventCh <- EmbedEvent{
				Type:          "done",
				ChunksSkipped: chunksSkipped,
				TotalChunks:   totalChunks,
			}
			return
		}

		// Process in batches
		batchTotal := (len(chunks) + opts.BatchSize - 1) / opts.BatchSize

		// Track chunks per repo for embedded_hash update
		repoChunkCounts := make(map[int64]struct{ total, processed int })
		for _, chunk := range chunks {
			counts := repoChunkCounts[chunk.RepoID]
			counts.total++
			repoChunkCounts[chunk.RepoID] = counts
		}

		for batchIndex := 0; batchIndex < batchTotal; batchIndex++ {
			start := batchIndex * opts.BatchSize
			end := start + opts.BatchSize
			if end > len(chunks) {
				end = len(chunks)
			}

			batch := chunks[start:end]

			// Prepare embed requests
			requests := make([]gemini.EmbedRequest, len(batch))
			for i, chunk := range batch {
				requests[i] = gemini.EmbedRequest{
					ID:      chunk.ID,
					Content: chunk.Content,
				}
			}

			// Call Gemini API
			result := gemini.EmbedChunks(ctx, opts.GeminiAPIKey, requests)

			// Process results
			for _, embedResult := range result.Results {
				// Store embedding in database
				embeddingBytes := gemini.Float32SliceToBytes(embedResult.Embedding)

				// Delete existing embedding if any
				_, _ = opts.DB.Exec("DELETE FROM chunk_embeddings WHERE chunk_id = ?", embedResult.ID)

				// Insert new embedding
				_, err := opts.DB.Exec(
					"INSERT INTO chunk_embeddings (chunk_id, embedding) VALUES (?, ?)",
					embedResult.ID, embeddingBytes,
				)
				if err != nil {
					chunksErrored++
					continue
				}

				chunksEmbedded++

				// Track repo progress
				var chunkRepoID int64
				for _, c := range batch {
					if c.ID == embedResult.ID {
						chunkRepoID = c.RepoID
						break
					}
				}
				if chunkRepoID > 0 {
					counts := repoChunkCounts[chunkRepoID]
					counts.processed++
					repoChunkCounts[chunkRepoID] = counts

					// Update embedded_hash if all chunks processed
					if counts.processed == counts.total {
						_, _ = opts.DB.Exec(
							"UPDATE repos SET embedded_hash = pushed_at_hash, embedded_at = ? WHERE id = ?",
							time.Now().UTC().Format(time.RFC3339), chunkRepoID,
						)
					}
				}
			}

			// Collect error messages
			var batchErrors []string
			for _, embedErr := range result.Errors {
				batchErrors = append(batchErrors, embedErr.Error)
			}
			chunksErrored += len(result.Errors)

			eventCh <- EmbedEvent{
				Type:           "batch",
				BatchIndex:     batchIndex + 1,
				BatchTotal:     batchTotal,
				ChunksEmbedded: chunksEmbedded,
				ChunksSkipped:  chunksSkipped,
				ChunksErrored:  chunksErrored,
				TotalChunks:    totalChunks,
				Errors:         batchErrors,
			}
		}

		eventCh <- EmbedEvent{
			Type:           "done",
			ChunksEmbedded: chunksEmbedded,
			ChunksSkipped:  chunksSkipped,
			ChunksErrored:  chunksErrored,
			TotalChunks:    totalChunks,
		}
	}()

	return eventCh
}
