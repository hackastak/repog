import type { Chunk, EmbedOptions, Repo } from '../types/index.js';
import { loadConfig } from '../config/config.js';
import { getDb } from '../db/index.js';

// Re-export pipeline types and functions
export {
  runEmbedPipeline,
  getPendingEmbedCount,
  hasRepos,
  type EmbedPipelineOptions,
  type EmbedProgress,
} from './pipeline.js';

/**
 * Generate embeddings for repositories.
 *
 * @param options - Embed options
 * @returns Number of chunks embedded
 */
export async function embedRepos(_options: EmbedOptions): Promise<number> {
  throw new Error('Not implemented');
}

/**
 * Generate embeddings for a single repository.
 *
 * @param repoId - The repository ID
 * @param force - Whether to re-embed existing chunks
 * @returns Number of chunks embedded
 */
export async function embedRepo(_repoId: number, _force: boolean): Promise<number> {
  throw new Error('Not implemented');
}

/**
 * Chunk repository content into smaller pieces.
 *
 * @param repo - The repository to chunk
 * @returns Array of chunks
 */
export function chunkRepo(_repo: Repo): Chunk[] {
  throw new Error('Not implemented');
}

/**
 * Generate embeddings for text content.
 *
 * @param text - The text to embed
 * @returns The embedding vector
 */
export async function generateEmbedding(_text: string): Promise<number[]> {
  throw new Error('Not implemented');
}

/**
 * Delete embeddings for a repository.
 *
 * @param repoId - The repository ID
 */
export function deleteEmbeddings(_repoId: number): void {
  const config = loadConfig();
  const db = getDb(config.dbPath);

  // Get all chunk IDs for this repo
  const chunks = db
    .prepare('SELECT id FROM chunks WHERE repo_id = ?')
    .all(_repoId) as { id: number }[];

  if (chunks.length === 0) {
    return;
  }

  // Delete embeddings for these chunks
  const placeholders = chunks.map(() => '?').join(',');
  const chunkIds = chunks.map((c) => c.id);

  db.prepare(`DELETE FROM chunk_embeddings WHERE chunk_id IN (${placeholders})`).run(...chunkIds);

  // Clear embedded_hash on the repo
  db.prepare('UPDATE repos SET embedded_hash = NULL, embedded_at = NULL WHERE id = ?').run(
    _repoId
  );
}
