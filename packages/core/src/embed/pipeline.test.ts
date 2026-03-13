import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import {
  runEmbedPipeline,
  getPendingEmbedCount,
  hasRepos,
  type EmbedProgress,
} from './pipeline.js';
import { initDb } from '../db/init.js';
import { getDb, closeDb } from '../db/index.js';
import os from 'os';
import path from 'path';
import fs from 'fs';

// Mock config module
vi.mock('../config/config.js', () => ({
  loadConfig: vi.fn(),
  loadConfigAsync: vi.fn(),
}));

// Mock embedChunks
vi.mock('../gemini/embeddings.js', () => ({
  embedChunks: vi.fn(),
}));

import { loadConfig, loadConfigAsync } from '../config/config.js';
import { embedChunks } from '../gemini/embeddings.js';

// Generate a mock embedding with 768 dimensions
function createMockEmbedding(): number[] {
  return Array(768).fill(0).map((_, i) => i * 0.001);
}

// Helper to collect all events from the async generator
async function collectEvents(generator: AsyncGenerator<EmbedProgress>): Promise<EmbedProgress[]> {
  const events: EmbedProgress[] = [];
  for await (const event of generator) {
    events.push(event);
  }
  return events;
}

describe('embed/pipeline', () => {
  let dbPath: string;
  let testDir: string;

  beforeEach(() => {
    // Use unique directory per test
    testDir = path.join(os.tmpdir(), `repog-embed-test-${process.pid}-${Date.now()}-${Math.random().toString(36).slice(2)}`);
    fs.mkdirSync(testDir, { recursive: true });
    dbPath = path.join(testDir, 'test.db');

    // Initialize the database
    initDb(dbPath);

    // Setup default config mock
    vi.mocked(loadConfig).mockReturnValue({
      githubPat: null,
      geminiKey: null,
      dbPath,
    });
    vi.mocked(loadConfigAsync).mockResolvedValue({
      githubPat: 'ghp_test',
      geminiKey: 'test-gemini-key',
      dbPath,
    });
  });

  afterEach(() => {
    vi.clearAllMocks();
    // Close the database connection before cleanup
    closeDb();
    try {
      fs.rmSync(testDir, { recursive: true, force: true });
    } catch {
      // Ignore cleanup errors
    }
  });

  describe('runEmbedPipeline', () => {
    it('yields error when no Gemini API key is configured', async () => {
      vi.mocked(loadConfigAsync).mockResolvedValue({
        githubPat: 'ghp_test',
        geminiKey: null,
        dbPath,
      });

      const events = await collectEvents(runEmbedPipeline({ includeFileTree: false }));

      const errorEvent = events.find(e => e.type === 'error');
      expect(errorEvent).toBeDefined();
      expect(errorEvent?.errorMessage).toBe('Gemini API key not configured');
    });

    it('yields done with zero counts when no repos exist', async () => {
      const events = await collectEvents(runEmbedPipeline({ includeFileTree: false }));

      const doneEvent = events.find(e => e.type === 'done');
      expect(doneEvent).toBeDefined();
      expect(doneEvent?.chunksEmbedded).toBe(0);
      expect(doneEvent?.chunksSkipped).toBe(0);
      expect(doneEvent?.totalChunks).toBe(0);
    });

    it('skips repos that are already embedded with same hash', async () => {
      const db = getDb(dbPath);

      // Insert a repo that's already fully embedded
      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, pushed_at, synced_at,
          pushed_at_hash, embedded_hash
        ) VALUES (
          1, 'owner', 'repo', 'owner/repo', 'https://github.com/owner/repo',
          datetime('now'), datetime('now'), 'hash123', 'hash123'
        )
      `).run();

      // Insert a chunk for this repo
      db.prepare(`
        INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
        VALUES (1, 'metadata', 0, 'Test content', 10)
      `).run();

      const events = await collectEvents(runEmbedPipeline({ includeFileTree: false }));

      const skipEvent = events.find(e => e.type === 'repo_skip');
      expect(skipEvent).toBeDefined();
      expect(skipEvent?.repoFullName).toBe('owner/repo');

      const doneEvent = events.find(e => e.type === 'done');
      expect(doneEvent?.chunksSkipped).toBe(1);
      expect(doneEvent?.chunksEmbedded).toBe(0);
    });

    it('processes chunks for repos with different hash', async () => {
      // Return empty results since sqlite-vec virtual tables have issues with parameterized inserts
      // The test focuses on verifying embedChunks is called with correct data
      vi.mocked(embedChunks).mockResolvedValue({
        results: [],
        errors: [],
      });

      const db = getDb(dbPath);

      // Insert a repo that needs embedding
      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, pushed_at, synced_at,
          pushed_at_hash, embedded_hash
        ) VALUES (
          1, 'owner', 'repo', 'owner/repo', 'https://github.com/owner/repo',
          datetime('now'), datetime('now'), 'newhash', NULL
        )
      `).run();

      // Insert a chunk
      db.prepare(`
        INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
        VALUES (1, 'metadata', 0, 'Test content', 10)
      `).run();

      const events = await collectEvents(runEmbedPipeline({ includeFileTree: false }));

      // Verify embedChunks was called with correct data
      expect(vi.mocked(embedChunks)).toHaveBeenCalledTimes(1);
      expect(vi.mocked(embedChunks)).toHaveBeenCalledWith(
        'test-gemini-key',
        [{ id: 1, content: 'Test content' }]
      );

      // Verify events
      const startEvent = events.find(e => e.type === 'start');
      expect(startEvent).toBeDefined();
      expect(startEvent?.totalChunks).toBe(1);

      const doneEvent = events.find(e => e.type === 'done');
      expect(doneEvent).toBeDefined();
    });

    it('excludes file_tree chunks when includeFileTree is false', async () => {
      const mockEmbedding = createMockEmbedding();
      vi.mocked(embedChunks).mockResolvedValue({
        results: [{ chunkId: 1, embedding: mockEmbedding }],
        errors: [],
      });

      const db = getDb(dbPath);

      // Insert a repo
      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, pushed_at, synced_at,
          pushed_at_hash, embedded_hash
        ) VALUES (
          1, 'owner', 'repo', 'owner/repo', 'https://github.com/owner/repo',
          datetime('now'), datetime('now'), 'hash', NULL
        )
      `).run();

      // Insert a metadata chunk and a file_tree chunk
      db.prepare(`
        INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
        VALUES (1, 'metadata', 0, 'Metadata content', 10)
      `).run();
      db.prepare(`
        INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
        VALUES (1, 'file_tree', 1, 'File tree content', 20)
      `).run();

      // Consume the async generator to completion
      await collectEvents(runEmbedPipeline({ includeFileTree: false }));

      // embedChunks should only be called with 1 chunk (metadata, not file_tree)
      expect(vi.mocked(embedChunks)).toHaveBeenCalledTimes(1);
      const callArgs = vi.mocked(embedChunks).mock.calls[0][1];
      expect(callArgs).toHaveLength(1);
      expect(callArgs[0].id).toBe(1); // Only metadata chunk
    });

    it('includes file_tree chunks when includeFileTree is true', async () => {
      const mockEmbedding = createMockEmbedding();
      vi.mocked(embedChunks).mockResolvedValue({
        results: [
          { chunkId: 1, embedding: mockEmbedding },
          { chunkId: 2, embedding: mockEmbedding },
        ],
        errors: [],
      });

      const db = getDb(dbPath);

      // Insert a repo
      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, pushed_at, synced_at,
          pushed_at_hash, embedded_hash
        ) VALUES (
          1, 'owner', 'repo', 'owner/repo', 'https://github.com/owner/repo',
          datetime('now'), datetime('now'), 'hash', NULL
        )
      `).run();

      // Insert both chunk types
      db.prepare(`
        INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
        VALUES (1, 'metadata', 0, 'Metadata content', 10)
      `).run();
      db.prepare(`
        INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
        VALUES (1, 'file_tree', 1, 'File tree content', 20)
      `).run();

      // Consume the async generator to completion
      await collectEvents(runEmbedPipeline({ includeFileTree: true }));

      // embedChunks should be called with 2 chunks
      expect(vi.mocked(embedChunks)).toHaveBeenCalledTimes(1);
      const callArgs = vi.mocked(embedChunks).mock.calls[0][1];
      expect(callArgs).toHaveLength(2);
    });

    it('yields batch progress events', async () => {
      // Return empty results to avoid sqlite-vec virtual table issues
      vi.mocked(embedChunks).mockResolvedValue({
        results: [],
        errors: [],
      });

      const db = getDb(dbPath);

      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, pushed_at, synced_at,
          pushed_at_hash, embedded_hash
        ) VALUES (
          1, 'owner', 'repo', 'owner/repo', 'https://github.com/owner/repo',
          datetime('now'), datetime('now'), 'hash', NULL
        )
      `).run();

      db.prepare(`
        INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
        VALUES (1, 'metadata', 0, 'Test content', 10)
      `).run();

      const events = await collectEvents(runEmbedPipeline({ includeFileTree: false }));

      // Verify embedChunks was called
      expect(vi.mocked(embedChunks)).toHaveBeenCalled();

      const batchEvent = events.find(e => e.type === 'batch');
      expect(batchEvent).toBeDefined();
      expect(batchEvent?.batchIndex).toBe(1);
      expect(batchEvent?.batchTotal).toBe(1);
    });

    it('handles batch embedding errors gracefully', async () => {
      vi.mocked(embedChunks).mockResolvedValue({
        results: [],
        errors: [{ chunkId: 1, error: 'API error' }],
      });

      const db = getDb(dbPath);

      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, pushed_at, synced_at,
          pushed_at_hash, embedded_hash
        ) VALUES (
          1, 'owner', 'repo', 'owner/repo', 'https://github.com/owner/repo',
          datetime('now'), datetime('now'), 'hash', NULL
        )
      `).run();

      db.prepare(`
        INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
        VALUES (1, 'metadata', 0, 'Test content', 10)
      `).run();

      const events = await collectEvents(runEmbedPipeline({ includeFileTree: false }));

      const doneEvent = events.find(e => e.type === 'done');
      expect(doneEvent?.chunksErrored).toBe(1);
      expect(doneEvent?.chunksEmbedded).toBe(0);
    });

    it('handles batch API failure gracefully', async () => {
      vi.mocked(embedChunks).mockRejectedValue(new Error('Network error'));

      const db = getDb(dbPath);

      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, pushed_at, synced_at,
          pushed_at_hash, embedded_hash
        ) VALUES (
          1, 'owner', 'repo', 'owner/repo', 'https://github.com/owner/repo',
          datetime('now'), datetime('now'), 'hash', NULL
        )
      `).run();

      db.prepare(`
        INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
        VALUES (1, 'metadata', 0, 'Test content', 10)
      `).run();

      const events = await collectEvents(runEmbedPipeline({ includeFileTree: false }));

      const errorEvent = events.find(e => e.type === 'error');
      expect(errorEvent).toBeDefined();
      expect(errorEvent?.errorMessage).toBe('Network error');
    });

    it('yields start event with total chunks count', async () => {
      const mockEmbedding = createMockEmbedding();
      vi.mocked(embedChunks).mockResolvedValue({
        results: [{ chunkId: 1, embedding: mockEmbedding }],
        errors: [],
      });

      const db = getDb(dbPath);

      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, pushed_at, synced_at,
          pushed_at_hash, embedded_hash
        ) VALUES (
          1, 'owner', 'repo', 'owner/repo', 'https://github.com/owner/repo',
          datetime('now'), datetime('now'), 'hash', NULL
        )
      `).run();

      db.prepare(`
        INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
        VALUES (1, 'metadata', 0, 'Test content', 10)
      `).run();

      const events = await collectEvents(runEmbedPipeline({ includeFileTree: false }));

      const startEvent = events.find(e => e.type === 'start');
      expect(startEvent).toBeDefined();
      expect(startEvent?.totalChunks).toBe(1);
    });
  });

  describe('getPendingEmbedCount', () => {
    it('returns 0 when no chunks exist', () => {
      const count = getPendingEmbedCount();
      expect(count).toBe(0);
    });

    it('returns count of chunks without embeddings', () => {
      const db = getDb(dbPath);

      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, pushed_at, synced_at
        ) VALUES (
          1, 'owner', 'repo', 'owner/repo', 'https://github.com/owner/repo',
          datetime('now'), datetime('now')
        )
      `).run();

      // Insert 3 chunks
      db.prepare(`INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate) VALUES (1, 'metadata', 0, 'content', 10)`).run();
      db.prepare(`INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate) VALUES (1, 'readme', 1, 'content', 10)`).run();
      db.prepare(`INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate) VALUES (1, 'file_tree', 2, 'content', 10)`).run();

      // Embed only 1 chunk
      const mockEmbedding = createMockEmbedding();
      const embeddingBuffer = Buffer.from(new Float32Array(mockEmbedding).buffer);
      db.prepare(`INSERT INTO chunk_embeddings (chunk_id, embedding) VALUES (1, ?)`).run(embeddingBuffer);

      const count = getPendingEmbedCount();
      expect(count).toBe(2); // 3 chunks - 1 embedded = 2 pending
    });

    it('returns 0 when all chunks are embedded', () => {
      const db = getDb(dbPath);

      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, pushed_at, synced_at
        ) VALUES (
          1, 'owner', 'repo', 'owner/repo', 'https://github.com/owner/repo',
          datetime('now'), datetime('now')
        )
      `).run();

      db.prepare(`INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate) VALUES (1, 'metadata', 0, 'content', 10)`).run();

      const mockEmbedding = createMockEmbedding();
      const embeddingBuffer = Buffer.from(new Float32Array(mockEmbedding).buffer);
      db.prepare(`INSERT INTO chunk_embeddings (chunk_id, embedding) VALUES (1, ?)`).run(embeddingBuffer);

      const count = getPendingEmbedCount();
      expect(count).toBe(0);
    });
  });

  describe('hasRepos', () => {
    it('returns false when no repos exist', () => {
      expect(hasRepos()).toBe(false);
    });

    it('returns true when repos exist', () => {
      const db = getDb(dbPath);

      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, pushed_at, synced_at
        ) VALUES (
          1, 'owner', 'repo', 'owner/repo', 'https://github.com/owner/repo',
          datetime('now'), datetime('now')
        )
      `).run();

      expect(hasRepos()).toBe(true);
    });
  });
});
