/**
 * Integration tests for the sync → embed → search pipeline.
 *
 * Since sync is not yet implemented and sqlite-vec has limitations with
 * parameterized inserts, these tests:
 * 1. Manually seed the database with repos and chunks (simulating sync)
 * 2. Directly insert embeddings (simulating what embed pipeline would do)
 * 3. Test the search functionality end-to-end
 *
 * The embed pipeline's behavior is tested separately to verify it calls
 * the Gemini API correctly.
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import os from 'os';
import path from 'path';
import fs from 'fs';
import { initDb } from '../db/init.js';
import { getDb, closeDb } from '../db/index.js';
import { runEmbedPipeline, type EmbedProgress } from '../embed/pipeline.js';
import { searchRepos } from '../search/query.js';

// Mock config module
vi.mock('../config/config.js', () => ({
  loadConfig: vi.fn(),
}));

// Mock Gemini embeddings
vi.mock('../gemini/embeddings.js', () => ({
  embedChunks: vi.fn(),
  embedQuery: vi.fn(),
}));

import { loadConfig } from '../config/config.js';
import { embedChunks, embedQuery } from '../gemini/embeddings.js';

// Constants
const EMBEDDING_DIMENSIONS = 768;

/**
 * Generate a deterministic mock embedding based on a seed.
 * Different seeds produce different embeddings for similarity testing.
 */
function createMockEmbedding(seed: number = 0): number[] {
  return Array(EMBEDDING_DIMENSIONS)
    .fill(0)
    .map((_, i) => Math.sin(seed + i * 0.01) * 0.5 + 0.5);
}

/**
 * Helper to collect all events from the async generator.
 */
async function collectEvents(generator: AsyncGenerator<EmbedProgress>): Promise<EmbedProgress[]> {
  const events: EmbedProgress[] = [];
  for await (const event of generator) {
    events.push(event);
  }
  return events;
}

/**
 * Insert a test repository into the database.
 */
function insertRepo(
  db: ReturnType<typeof getDb>,
  data: {
    githubId: number;
    owner: string;
    name: string;
    description?: string;
    language?: string;
    stars?: number;
    isStarred?: boolean;
    isOwned?: boolean;
    pushedAtHash?: string | null;
    embeddedHash?: string | null;
  }
): number {
  const result = db
    .prepare(
      `
    INSERT INTO repos (
      github_id, owner, name, full_name, url, description,
      primary_language, stars, is_starred, is_owned,
      pushed_at, synced_at, pushed_at_hash, embedded_hash
    ) VALUES (
      @githubId, @owner, @name, @fullName, @url, @description,
      @language, @stars, @isStarred, @isOwned,
      datetime('now'), datetime('now'), @pushedAtHash, @embeddedHash
    )
  `
    )
    .run({
      githubId: data.githubId,
      owner: data.owner,
      name: data.name,
      fullName: `${data.owner}/${data.name}`,
      url: `https://github.com/${data.owner}/${data.name}`,
      description: data.description ?? null,
      language: data.language ?? null,
      stars: data.stars ?? 0,
      isStarred: data.isStarred ? 1 : 0,
      isOwned: data.isOwned ? 1 : 0,
      pushedAtHash: data.pushedAtHash ?? null,
      embeddedHash: data.embeddedHash ?? null,
    });

  return result.lastInsertRowid as number;
}

/**
 * Insert a chunk for a repository.
 */
function insertChunk(
  db: ReturnType<typeof getDb>,
  data: {
    repoId: number;
    chunkType: 'metadata' | 'readme' | 'file_tree';
    chunkIndex: number;
    content: string;
    tokenEstimate?: number;
  }
): number {
  const result = db
    .prepare(
      `
    INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
    VALUES (@repoId, @chunkType, @chunkIndex, @content, @tokenEstimate)
  `
    )
    .run({
      repoId: data.repoId,
      chunkType: data.chunkType,
      chunkIndex: data.chunkIndex,
      content: data.content,
      tokenEstimate: data.tokenEstimate ?? Math.ceil(data.content.length / 4),
    });

  return result.lastInsertRowid as number;
}

/**
 * Insert an embedding for a chunk.
 * Uses literal SQL for chunk_id due to sqlite-vec limitations.
 */
function insertEmbedding(db: ReturnType<typeof getDb>, chunkId: number, embedding: number[]): void {
  const embeddingBuffer = Buffer.from(new Float32Array(embedding).buffer);
  // sqlite-vec requires literal integers for primary key, not parameters
  db.prepare(`INSERT INTO chunk_embeddings (chunk_id, embedding) VALUES (${chunkId}, ?)`).run(
    embeddingBuffer
  );
}

describe('Integration: Sync → Embed → Search Pipeline', () => {
  let testDir: string;
  let dbPath: string;

  beforeEach(() => {
    // Create unique test directory
    testDir = path.join(
      os.tmpdir(),
      `repog-integration-test-${process.pid}-${Date.now()}-${Math.random().toString(36).slice(2)}`
    );
    fs.mkdirSync(testDir, { recursive: true });
    dbPath = path.join(testDir, 'test.db');

    // Initialize database
    initDb(dbPath);

    // Setup config mock
    vi.mocked(loadConfig).mockReturnValue({
      githubPat: 'ghp_test_token',
      geminiKey: 'test-gemini-key',
      dbPath,
    });

    // Reset mocks
    vi.clearAllMocks();
  });

  afterEach(() => {
    closeDb();
    try {
      fs.rmSync(testDir, { recursive: true, force: true });
    } catch {
      // Ignore cleanup errors
    }
  });

  describe('Full Pipeline: Seed → Embed → Search', () => {
    it('searches seeded repos with embeddings and returns results', async () => {
      const db = getDb(dbPath);

      // Phase 1: Seed database (simulating sync)
      const repo1Id = insertRepo(db, {
        githubId: 1001,
        owner: 'testuser',
        name: 'auth-library',
        description: 'A robust authentication library for Node.js applications',
        language: 'TypeScript',
        stars: 150,
        isOwned: true,
        pushedAtHash: 'hash-v1',
      });

      const repo2Id = insertRepo(db, {
        githubId: 1002,
        owner: 'testuser',
        name: 'database-utils',
        description: 'Database utilities and ORM helpers',
        language: 'TypeScript',
        stars: 75,
        isOwned: true,
        pushedAtHash: 'hash-v1',
      });

      const repo3Id = insertRepo(db, {
        githubId: 1003,
        owner: 'otheruser',
        name: 'react-components',
        description: 'Reusable React UI components',
        language: 'JavaScript',
        stars: 500,
        isStarred: true,
        pushedAtHash: 'hash-v1',
      });

      // Insert chunks for each repo
      const chunk1Id = insertChunk(db, {
        repoId: repo1Id,
        chunkType: 'metadata',
        chunkIndex: 0,
        content:
          'auth-library: Authentication middleware with JWT support, OAuth2 integration, and session management',
      });

      const chunk2Id = insertChunk(db, {
        repoId: repo2Id,
        chunkType: 'metadata',
        chunkIndex: 0,
        content:
          'database-utils: SQL query builder, connection pooling, and migration tools for PostgreSQL and MySQL',
      });

      const chunk3Id = insertChunk(db, {
        repoId: repo3Id,
        chunkType: 'metadata',
        chunkIndex: 0,
        content:
          'react-components: Button, Modal, Form, Table, and other UI components with accessibility support',
      });

      // Phase 2: Insert embeddings (simulating embed pipeline)
      const authEmbedding = createMockEmbedding(100);
      const dbEmbedding = createMockEmbedding(200);
      const reactEmbedding = createMockEmbedding(300);

      insertEmbedding(db, chunk1Id, authEmbedding);
      insertEmbedding(db, chunk2Id, dbEmbedding);
      insertEmbedding(db, chunk3Id, reactEmbedding);

      // Phase 3: Search for authentication-related repos
      // Mock query embedding to match auth embedding
      vi.mocked(embedQuery).mockResolvedValue(authEmbedding);

      const authResults = await searchRepos('authentication JWT middleware', { limit: 3 });

      expect(authResults.results.length).toBeGreaterThan(0);
      // Auth repo should be first since query embedding matches auth embedding exactly
      expect(authResults.results[0].repoFullName).toBe('testuser/auth-library');
      expect(authResults.results[0].similarity).toBe(1); // Exact match
    });

    it('filters search results by language', async () => {
      const db = getDb(dbPath);

      // Seed repos with different languages
      const tsRepoId = insertRepo(db, {
        githubId: 2001,
        owner: 'dev',
        name: 'ts-project',
        language: 'TypeScript',
        pushedAtHash: 'hash-v1',
      });

      const jsRepoId = insertRepo(db, {
        githubId: 2002,
        owner: 'dev',
        name: 'js-project',
        language: 'JavaScript',
        pushedAtHash: 'hash-v1',
      });

      const tsChunkId = insertChunk(db, {
        repoId: tsRepoId,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'TypeScript web application framework',
      });

      const jsChunkId = insertChunk(db, {
        repoId: jsRepoId,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'JavaScript web application framework',
      });

      const embedding = createMockEmbedding(1);
      insertEmbedding(db, tsChunkId, embedding);
      insertEmbedding(db, jsChunkId, embedding);

      vi.mocked(embedQuery).mockResolvedValue(embedding);

      const results = await searchRepos('web framework', { language: 'TypeScript', limit: 10 });

      expect(results.results.length).toBe(1);
      expect(results.results[0].repoFullName).toBe('dev/ts-project');
      expect(results.results[0].language).toBe('TypeScript');
    });

    it('filters search results by owned status', async () => {
      const db = getDb(dbPath);

      const ownedRepoId = insertRepo(db, {
        githubId: 3001,
        owner: 'me',
        name: 'my-project',
        isOwned: true,
        pushedAtHash: 'hash-v1',
      });

      const notOwnedRepoId = insertRepo(db, {
        githubId: 3002,
        owner: 'other',
        name: 'their-project',
        isOwned: false,
        pushedAtHash: 'hash-v1',
      });

      const ownedChunkId = insertChunk(db, {
        repoId: ownedRepoId,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'My personal project',
      });

      const notOwnedChunkId = insertChunk(db, {
        repoId: notOwnedRepoId,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'Someone elses project',
      });

      const embedding = createMockEmbedding(1);
      insertEmbedding(db, ownedChunkId, embedding);
      insertEmbedding(db, notOwnedChunkId, embedding);

      vi.mocked(embedQuery).mockResolvedValue(embedding);

      const results = await searchRepos('project', { owned: true, limit: 10 });

      expect(results.results.length).toBe(1);
      expect(results.results[0].repoFullName).toBe('me/my-project');
      expect(results.results[0].isOwned).toBe(true);
    });

    it('filters search results by starred status', async () => {
      const db = getDb(dbPath);

      const starredRepoId = insertRepo(db, {
        githubId: 4001,
        owner: 'popular',
        name: 'starred-repo',
        isStarred: true,
        pushedAtHash: 'hash-v1',
      });

      const notStarredRepoId = insertRepo(db, {
        githubId: 4002,
        owner: 'unpopular',
        name: 'unstarred-repo',
        isStarred: false,
        pushedAtHash: 'hash-v1',
      });

      const starredChunkId = insertChunk(db, {
        repoId: starredRepoId,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'A starred repository',
      });

      const notStarredChunkId = insertChunk(db, {
        repoId: notStarredRepoId,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'An unstarred repository',
      });

      const embedding = createMockEmbedding(1);
      insertEmbedding(db, starredChunkId, embedding);
      insertEmbedding(db, notStarredChunkId, embedding);

      vi.mocked(embedQuery).mockResolvedValue(embedding);

      const results = await searchRepos('repository', { starred: true, limit: 10 });

      expect(results.results.length).toBe(1);
      expect(results.results[0].repoFullName).toBe('popular/starred-repo');
      expect(results.results[0].isStarred).toBe(true);
    });

    it('filters search results by owner', async () => {
      const db = getDb(dbPath);

      const repo1Id = insertRepo(db, {
        githubId: 5001,
        owner: 'alice',
        name: 'project-a',
        pushedAtHash: 'hash-v1',
      });

      const repo2Id = insertRepo(db, {
        githubId: 5002,
        owner: 'bob',
        name: 'project-b',
        pushedAtHash: 'hash-v1',
      });

      const chunk1Id = insertChunk(db, {
        repoId: repo1Id,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'Alice project',
      });

      const chunk2Id = insertChunk(db, {
        repoId: repo2Id,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'Bob project',
      });

      const embedding = createMockEmbedding(1);
      insertEmbedding(db, chunk1Id, embedding);
      insertEmbedding(db, chunk2Id, embedding);

      vi.mocked(embedQuery).mockResolvedValue(embedding);

      const results = await searchRepos('project', { owner: 'alice', limit: 10 });

      expect(results.results.length).toBe(1);
      expect(results.results[0].repoFullName).toBe('alice/project-a');
      expect(results.results[0].owner).toBe('alice');
    });

    it('combines multiple filters', async () => {
      const db = getDb(dbPath);

      // Create 4 repos with different combinations of properties
      const matchingRepoId = insertRepo(db, {
        githubId: 6001,
        owner: 'targetuser',
        name: 'typescript-starred',
        language: 'TypeScript',
        isStarred: true,
        pushedAtHash: 'hash-v1',
      });

      const wrongLangRepoId = insertRepo(db, {
        githubId: 6002,
        owner: 'targetuser',
        name: 'javascript-starred',
        language: 'JavaScript',
        isStarred: true,
        pushedAtHash: 'hash-v1',
      });

      const wrongOwnerRepoId = insertRepo(db, {
        githubId: 6003,
        owner: 'otheruser',
        name: 'typescript-starred-other',
        language: 'TypeScript',
        isStarred: true,
        pushedAtHash: 'hash-v1',
      });

      const notStarredRepoId = insertRepo(db, {
        githubId: 6004,
        owner: 'targetuser',
        name: 'typescript-not-starred',
        language: 'TypeScript',
        isStarred: false,
        pushedAtHash: 'hash-v1',
      });

      const chunks = [
        { repoId: matchingRepoId, content: 'Matching repo' },
        { repoId: wrongLangRepoId, content: 'Wrong language' },
        { repoId: wrongOwnerRepoId, content: 'Wrong owner' },
        { repoId: notStarredRepoId, content: 'Not starred' },
      ];

      const chunkIds: number[] = [];
      for (const chunk of chunks) {
        const id = insertChunk(db, {
          repoId: chunk.repoId,
          chunkType: 'metadata',
          chunkIndex: 0,
          content: chunk.content,
        });
        chunkIds.push(id);
      }

      const embedding = createMockEmbedding(1);
      for (const chunkId of chunkIds) {
        insertEmbedding(db, chunkId, embedding);
      }

      vi.mocked(embedQuery).mockResolvedValue(embedding);

      const results = await searchRepos('repo', {
        language: 'TypeScript',
        starred: true,
        owner: 'targetuser',
        limit: 10,
      });

      expect(results.results.length).toBe(1);
      expect(results.results[0].repoFullName).toBe('targetuser/typescript-starred');
    });
  });

  describe('Embed Pipeline: API Integration', () => {
    it('calls embedChunks with correct data for repos needing embedding', async () => {
      const db = getDb(dbPath);

      // Insert a repo that needs embedding (no embedded_hash)
      insertRepo(db, {
        githubId: 7001,
        owner: 'user',
        name: 'needs-embedding',
        pushedAtHash: 'content-hash',
        embeddedHash: null,
      });

      insertChunk(db, {
        repoId: 1,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'Repository content to embed',
      });

      // Mock embedChunks to return empty (avoids sqlite-vec insert issues)
      vi.mocked(embedChunks).mockResolvedValue({
        results: [],
        errors: [],
      });

      await collectEvents(runEmbedPipeline({ includeFileTree: false }));

      // Verify embedChunks was called with correct data
      expect(vi.mocked(embedChunks)).toHaveBeenCalledTimes(1);
      expect(vi.mocked(embedChunks)).toHaveBeenCalledWith('test-gemini-key', [
        { id: 1, content: 'Repository content to embed' },
      ]);
    });

    it('skips repos with matching embed hash', async () => {
      const db = getDb(dbPath);

      // Repo already embedded (hashes match)
      insertRepo(db, {
        githubId: 8001,
        owner: 'user',
        name: 'already-embedded',
        pushedAtHash: 'same-hash',
        embeddedHash: 'same-hash',
      });

      insertChunk(db, {
        repoId: 1,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'Already embedded content',
      });

      const events = await collectEvents(runEmbedPipeline({ includeFileTree: false }));

      // Should skip this repo
      const skipEvent = events.find((e) => e.type === 'repo_skip');
      expect(skipEvent).toBeDefined();
      expect(skipEvent?.repoFullName).toBe('user/already-embedded');

      // embedChunks should not be called
      expect(vi.mocked(embedChunks)).not.toHaveBeenCalled();
    });

    it('excludes file_tree chunks when includeFileTree is false', async () => {
      const db = getDb(dbPath);

      insertRepo(db, {
        githubId: 9001,
        owner: 'user',
        name: 'with-file-tree',
        pushedAtHash: 'hash',
        embeddedHash: null,
      });

      insertChunk(db, {
        repoId: 1,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'Metadata content',
      });

      insertChunk(db, {
        repoId: 1,
        chunkType: 'file_tree',
        chunkIndex: 1,
        content: 'File tree content',
      });

      vi.mocked(embedChunks).mockResolvedValue({
        results: [],
        errors: [],
      });

      await collectEvents(runEmbedPipeline({ includeFileTree: false }));

      // Only metadata chunk should be sent
      expect(vi.mocked(embedChunks)).toHaveBeenCalledWith('test-gemini-key', [
        { id: 1, content: 'Metadata content' },
      ]);
    });

    it('includes file_tree chunks when includeFileTree is true', async () => {
      const db = getDb(dbPath);

      insertRepo(db, {
        githubId: 10001,
        owner: 'user',
        name: 'with-file-tree',
        pushedAtHash: 'hash',
        embeddedHash: null,
      });

      insertChunk(db, {
        repoId: 1,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'Metadata content',
      });

      insertChunk(db, {
        repoId: 1,
        chunkType: 'file_tree',
        chunkIndex: 1,
        content: 'File tree content',
      });

      vi.mocked(embedChunks).mockResolvedValue({
        results: [],
        errors: [],
      });

      await collectEvents(runEmbedPipeline({ includeFileTree: true }));

      // Both chunks should be sent
      expect(vi.mocked(embedChunks)).toHaveBeenCalledWith('test-gemini-key', [
        { id: 1, content: 'Metadata content' },
        { id: 2, content: 'File tree content' },
      ]);
    });

    it('handles API failures gracefully', async () => {
      const db = getDb(dbPath);

      insertRepo(db, {
        githubId: 11001,
        owner: 'user',
        name: 'test-repo',
        pushedAtHash: 'hash',
        embeddedHash: null,
      });

      insertChunk(db, {
        repoId: 1,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'Test content',
      });

      vi.mocked(embedChunks).mockRejectedValue(new Error('API rate limit exceeded'));

      const events = await collectEvents(runEmbedPipeline({ includeFileTree: false }));

      const errorEvent = events.find((e) => e.type === 'error');
      expect(errorEvent).toBeDefined();
      expect(errorEvent?.errorMessage).toBe('API rate limit exceeded');
    });
  });

  describe('Search: Edge Cases', () => {
    it('returns empty results when no embeddings exist', async () => {
      const db = getDb(dbPath);

      // Insert repo and chunk but don't insert embedding
      const repoId = insertRepo(db, {
        githubId: 12001,
        owner: 'user',
        name: 'no-embeddings',
        pushedAtHash: 'hash-v1',
      });

      insertChunk(db, {
        repoId,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'Content without embedding',
      });

      vi.mocked(embedQuery).mockResolvedValue(createMockEmbedding(1));

      const results = await searchRepos('test query', { limit: 10 });

      expect(results.results).toHaveLength(0);
    });

    it('returns empty results when query embedding fails', async () => {
      const db = getDb(dbPath);

      const repoId = insertRepo(db, {
        githubId: 13001,
        owner: 'user',
        name: 'has-embeddings',
        pushedAtHash: 'hash-v1',
      });

      const chunkId = insertChunk(db, {
        repoId,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'Some content',
      });

      insertEmbedding(db, chunkId, createMockEmbedding(1));

      // Query embedding fails
      vi.mocked(embedQuery).mockResolvedValue(null);

      const results = await searchRepos('test query', { limit: 10 });

      expect(results.results).toHaveLength(0);
    });

    it('deduplicates results by repository', async () => {
      const db = getDb(dbPath);

      // One repo with multiple chunks
      const repoId = insertRepo(db, {
        githubId: 14001,
        owner: 'user',
        name: 'multi-chunk-repo',
        pushedAtHash: 'hash-v1',
      });

      const chunk1Id = insertChunk(db, {
        repoId,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'Metadata content',
      });

      const chunk2Id = insertChunk(db, {
        repoId,
        chunkType: 'readme',
        chunkIndex: 0,
        content: 'README content',
      });

      const embedding = createMockEmbedding(1);
      insertEmbedding(db, chunk1Id, embedding);
      insertEmbedding(db, chunk2Id, embedding);

      vi.mocked(embedQuery).mockResolvedValue(embedding);

      const results = await searchRepos('content', { limit: 10 });

      // Should only return one result for the repo (deduplicated)
      expect(results.results.length).toBe(1);
      expect(results.results[0].repoFullName).toBe('user/multi-chunk-repo');
    });

    it('respects limit parameter', async () => {
      const db = getDb(dbPath);

      // Create 5 repos
      for (let i = 0; i < 5; i++) {
        const repoId = insertRepo(db, {
          githubId: 15000 + i,
          owner: 'user',
          name: `repo-${i}`,
          pushedAtHash: 'hash-v1',
        });

        const chunkId = insertChunk(db, {
          repoId,
          chunkType: 'metadata',
          chunkIndex: 0,
          content: `Repository ${i} content`,
        });

        insertEmbedding(db, chunkId, createMockEmbedding(1));
      }

      vi.mocked(embedQuery).mockResolvedValue(createMockEmbedding(1));

      const results = await searchRepos('repository', { limit: 2 });

      expect(results.results.length).toBe(2);
    });

    it('returns results sorted by similarity', async () => {
      const db = getDb(dbPath);

      // Create repos with different embeddings
      const closeRepoId = insertRepo(db, {
        githubId: 16001,
        owner: 'user',
        name: 'close-match',
        pushedAtHash: 'hash-v1',
      });

      const farRepoId = insertRepo(db, {
        githubId: 16002,
        owner: 'user',
        name: 'far-match',
        pushedAtHash: 'hash-v1',
      });

      const closeChunkId = insertChunk(db, {
        repoId: closeRepoId,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'Close match content',
      });

      const farChunkId = insertChunk(db, {
        repoId: farRepoId,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'Far match content',
      });

      // Use different seeds to create different embeddings
      const queryEmbedding = createMockEmbedding(100);
      const closeEmbedding = createMockEmbedding(100); // Same as query
      const farEmbedding = createMockEmbedding(500); // Different from query

      insertEmbedding(db, closeChunkId, closeEmbedding);
      insertEmbedding(db, farChunkId, farEmbedding);

      vi.mocked(embedQuery).mockResolvedValue(queryEmbedding);

      const results = await searchRepos('content', { limit: 10 });

      expect(results.results.length).toBe(2);
      // Close match should be first (higher similarity)
      expect(results.results[0].repoFullName).toBe('user/close-match');
      expect(results.results[0].similarity).toBeGreaterThan(results.results[1].similarity);
    });
  });

  describe('Search: Timing Metrics', () => {
    it('reports timing metrics for search operations', async () => {
      const db = getDb(dbPath);

      const repoId = insertRepo(db, {
        githubId: 17001,
        owner: 'user',
        name: 'test-repo',
        pushedAtHash: 'hash-v1',
      });

      const chunkId = insertChunk(db, {
        repoId,
        chunkType: 'metadata',
        chunkIndex: 0,
        content: 'Test content',
      });

      const embedding = createMockEmbedding(1);
      insertEmbedding(db, chunkId, embedding);

      vi.mocked(embedQuery).mockResolvedValue(embedding);

      const results = await searchRepos('test', { limit: 10 });

      expect(results.queryEmbeddingMs).toBeGreaterThanOrEqual(0);
      expect(results.searchMs).toBeGreaterThanOrEqual(0);
      expect(results.totalConsidered).toBeGreaterThanOrEqual(0);
    });
  });
});
