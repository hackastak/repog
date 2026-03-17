import { loadConfig, loadConfigAsync } from '../config/config.js';
import { getDb } from '../db/index.js';
import { embedChunks } from '../gemini/embeddings.js';
import type { Repo } from '../types/index.js';

/**
 * Default batch size for embedding requests.
 * Gemini API supports up to 100 embeddings per batch request,
 * but we use 20 for better error handling and progress updates.
 */
const DEFAULT_BATCH_SIZE = 20;

/**
 * Maximum allowed batch size.
 */
const MAX_BATCH_SIZE = 100;

/**
 * Options for the embedding pipeline.
 */
export interface EmbedPipelineOptions {
  /** If false, skip chunks with chunk_type = 'file_tree' */
  includeFileTree: boolean;
  /** Batch size for embedding requests (default: 20, max: 100) */
  batchSize?: number;
}

/**
 * Progress event from the embedding pipeline.
 */
export interface EmbedProgress {
  type: 'start' | 'batch' | 'repo_skip' | 'error' | 'done';
  /** Repository full name (for repo_skip events) */
  repoFullName?: string;
  /** Current batch index (1-based, for batch events) */
  batchIndex?: number;
  /** Total number of batches (for batch events) */
  batchTotal?: number;
  /** Running total of chunks successfully embedded */
  chunksEmbedded?: number;
  /** Total chunks skipped due to repos with unchanged hash */
  chunksSkipped?: number;
  /** Total individual chunk embedding failures */
  chunksErrored?: number;
  /** Total chunks to process (null until calculated) */
  totalChunks?: number;
  /** Error message (for error events) */
  errorMessage?: string;
  /** Detailed errors for individual chunks in a batch (optional) */
  batchErrors?: Array<{ chunkId: number; error: string }>;
}

/**
 * Internal representation of a chunk with content.
 */
interface ChunkWithContent {
  id: number;
  repoId: number;
  content: string;
}

/**
 * Run the embedding pipeline.
 *
 * Processes all chunks across all synced repositories, generating embeddings
 * and storing them in the chunk_embeddings table. Yields progress events
 * for each significant operation.
 *
 * @param options - Pipeline options
 * @yields EmbedProgress events for each stage of processing
 */
export async function* runEmbedPipeline(
  options: EmbedPipelineOptions
): AsyncGenerator<EmbedProgress> {
  let chunksEmbedded = 0;
  let chunksSkipped = 0;
  let chunksErrored = 0;

  // Determine batch size with validation
  let batchSize = options.batchSize ?? DEFAULT_BATCH_SIZE;
  if (batchSize > MAX_BATCH_SIZE) {
    batchSize = MAX_BATCH_SIZE;
  }
  if (batchSize < 1) {
    batchSize = DEFAULT_BATCH_SIZE;
  }

  try {
    // Load config and get API key
    const config = await loadConfigAsync();
    if (!config.geminiKey) {
      yield {
        type: 'error',
        errorMessage: 'Gemini API key not configured',
        chunksEmbedded,
        chunksSkipped,
        chunksErrored,
      };
      return;
    }

    const apiKey = config.geminiKey;
    const db = getDb(config.dbPath);

    // Get all repos with pushed_at_hash
    const repos = db
      .prepare(
        `SELECT id, full_name, pushed_at_hash, embedded_hash
         FROM repos
         WHERE pushed_at_hash IS NOT NULL`
      )
      .all() as Pick<Repo, 'id' | 'full_name' | 'pushed_at_hash' | 'embedded_hash'>[];

    if (repos.length === 0) {
      yield {
        type: 'done',
        chunksEmbedded: 0,
        chunksSkipped: 0,
        chunksErrored: 0,
        totalChunks: 0,
      };
      return;
    }

    // Identify repos to skip (already fully embedded with same hash)
    const reposToProcess: typeof repos = [];
    for (const repo of repos) {
      if (repo.embedded_hash === repo.pushed_at_hash) {
        // Repo is already embedded with current hash - skip
        const skippedCount = db
          .prepare(
            `SELECT COUNT(*) as count FROM chunks WHERE repo_id = ?${options.includeFileTree ? '' : " AND chunk_type != 'file_tree'"}`
          )
          .get(repo.id) as { count: number };

        chunksSkipped += skippedCount.count;

        yield {
          type: 'repo_skip',
          repoFullName: repo.full_name,
          chunksEmbedded,
          chunksSkipped,
          chunksErrored,
        };
      } else {
        reposToProcess.push(repo);
      }
    }

    // Collect all eligible chunks from non-skipped repos
    const repoIds = reposToProcess.map((r) => r.id);
    if (repoIds.length === 0) {
      yield {
        type: 'done',
        chunksEmbedded,
        chunksSkipped,
        chunksErrored,
        totalChunks: chunksSkipped,
      };
      return;
    }

    const placeholders = repoIds.map(() => '?').join(',');
    const chunkTypeFilter = options.includeFileTree ? '' : " AND chunk_type != 'file_tree'";

    const chunks = db
      .prepare(
        `SELECT id, repo_id, content
         FROM chunks
         WHERE repo_id IN (${placeholders})${chunkTypeFilter}
         ORDER BY repo_id, id`
      )
      .all(...repoIds) as { id: number; repo_id: number; content: string }[];

    const totalChunks = chunks.length + chunksSkipped;

    // Yield start event
    yield {
      type: 'start',
      chunksEmbedded: 0,
      chunksSkipped,
      chunksErrored: 0,
      totalChunks,
    };

    if (chunks.length === 0) {
      yield {
        type: 'done',
        chunksEmbedded: 0,
        chunksSkipped,
        chunksErrored: 0,
        totalChunks,
      };
      return;
    }

    // Split chunks into batches
    const batches: ChunkWithContent[][] = [];
    for (let i = 0; i < chunks.length; i += batchSize) {
      batches.push(
        chunks.slice(i, i + batchSize).map((c) => ({
          id: c.id,
          repoId: c.repo_id,
          content: c.content,
        }))
      );
    }

    const batchTotal = batches.length;

    // Track which repos have been fully processed
    const repoChunkCounts = new Map<number, { total: number; processed: number }>();
    for (const chunk of chunks) {
      const existing = repoChunkCounts.get(chunk.repo_id);
      if (existing) {
        existing.total++;
      } else {
        repoChunkCounts.set(chunk.repo_id, { total: 1, processed: 0 });
      }
    }

    // Prepare update statement for embedded_hash
    const updateEmbeddedHash = db.prepare(
      `UPDATE repos SET embedded_hash = pushed_at_hash, embedded_at = ? WHERE id = ?`
    );

    // Process each batch
    for (let batchIndex = 0; batchIndex < batches.length; batchIndex++) {
      const batch = batches[batchIndex];

      try {
        const result = await embedChunks(
          apiKey,
          batch.map((c) => ({ id: c.id, content: c.content }))
        );

        // Insert successful embeddings
        for (const embedding of result.results) {
          try {
            const buffer = Buffer.from(new Float32Array(embedding.embedding).buffer);
            // sqlite-vec's vec0 virtual table sometimes has issues with INSERT OR REPLACE.
            // Using explicit DELETE then INSERT for maximum compatibility.
            db.prepare(`DELETE FROM chunk_embeddings WHERE rowid = ${Number(embedding.chunkId)}`).run();
            db.prepare(`INSERT INTO chunk_embeddings (rowid, embedding) VALUES (${Number(embedding.chunkId)}, ?)`).run(buffer);
            chunksEmbedded++;
          } catch (dbErr) {
            chunksErrored++;
            if (!result.errors.some(e => e.chunkId === embedding.chunkId)) {
              result.errors.push({
                chunkId: embedding.chunkId,
                error: dbErr instanceof Error ? `DB Error: ${dbErr.message}` : 'Unknown DB error',
              });
            }
          }

          // Track progress per repo
          const chunk = batch.find((c) => c.id === embedding.chunkId);
          if (chunk) {
            const counts = repoChunkCounts.get(chunk.repoId);
            if (counts) {
              counts.processed++;

              // Check if repo is fully processed
              if (counts.processed === counts.total) {
                updateEmbeddedHash.run(new Date().toISOString(), chunk.repoId);
              }
            }
          }
        }

        // Track errors
        chunksErrored += result.errors.length;

        // Yield batch progress
        yield {
          type: 'batch',
          batchIndex: batchIndex + 1,
          batchTotal,
          chunksEmbedded,
          chunksSkipped,
          chunksErrored,
          totalChunks,
          batchErrors: result.errors.length > 0 ? result.errors.map(e => ({ chunkId: e.chunkId, error: e.error })) : undefined,
        };
      } catch (error) {
        // Batch processing error - count all as errors
        chunksErrored += batch.length;

        yield {
          type: 'error',
          errorMessage: error instanceof Error ? error.message : 'Unknown batch error',
          chunksEmbedded,
          chunksSkipped,
          chunksErrored,
          totalChunks,
        };
      }
    }

    // Final done event
    yield {
      type: 'done',
      chunksEmbedded,
      chunksSkipped,
      chunksErrored,
      totalChunks,
    };
  } catch (error) {
    yield {
      type: 'error',
      errorMessage: error instanceof Error ? error.message : 'Unknown pipeline error',
      chunksEmbedded,
      chunksSkipped,
      chunksErrored,
    };

    yield {
      type: 'done',
      chunksEmbedded,
      chunksSkipped,
      chunksErrored,
    };
  }
}

/**
 * Get the count of chunks pending embedding.
 *
 * @returns Number of chunks without embeddings
 */
export function getPendingEmbedCount(): number {
  const config = loadConfig();
  const db = getDb(config.dbPath);

  const result = db
    .prepare(
      `SELECT COUNT(*) as count
       FROM chunks c
       LEFT JOIN chunk_embeddings ce ON c.id = ce.rowid
       WHERE ce.rowid IS NULL`
    )
    .get() as { count: number };

  return result.count;
}

/**
 * Check if there are any repositories in the database.
 *
 * @returns True if at least one repo exists
 */
export function hasRepos(): boolean {
  const config = loadConfig();
  const db = getDb(config.dbPath);

  const result = db.prepare('SELECT COUNT(*) as count FROM repos').get() as { count: number };

  return result.count > 0;
}
