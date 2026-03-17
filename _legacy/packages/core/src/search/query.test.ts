import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { searchRepos } from './query.js';
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

// Mock embedQuery
vi.mock('../gemini/embeddings.js', () => ({
  embedQuery: vi.fn(),
}));

import { loadConfig, loadConfigAsync } from '../config/config.js';
import { embedQuery } from '../gemini/embeddings.js';

// Generate a mock embedding with 768 dimensions
function createMockEmbedding(): number[] {
  return Array(768).fill(0).map((_, i) => i * 0.001);
}

describe('search/query', () => {
  let dbPath: string;
  let testDir: string;

  beforeEach(() => {
    // Use unique directory per test
    testDir = path.join(os.tmpdir(), `repog-search-test-${process.pid}-${Date.now()}-${Math.random().toString(36).slice(2)}`);
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

  describe('searchRepos', () => {
    it('returns empty results when no Gemini API key is configured', async () => {
      vi.mocked(loadConfigAsync).mockResolvedValue({
        githubPat: 'ghp_test',
        geminiKey: null,
        dbPath,
      });

      const result = await searchRepos('test query');

      expect(result.results).toHaveLength(0);
      expect(result.totalConsidered).toBe(0);
    });

    it('returns empty results when embedQuery returns null', async () => {
      vi.mocked(embedQuery).mockResolvedValue(null);

      const result = await searchRepos('test query');

      expect(result.results).toHaveLength(0);
      expect(result.queryEmbeddingMs).toBeGreaterThanOrEqual(0);
    });

    it('returns empty results when database has no embeddings', async () => {
      const mockEmbedding = createMockEmbedding();
      vi.mocked(embedQuery).mockResolvedValue(mockEmbedding);

      const result = await searchRepos('test query');

      expect(result.results).toHaveLength(0);
    });

    it('returns search results with similarity scores', async () => {
      const mockEmbedding = createMockEmbedding();
      vi.mocked(embedQuery).mockResolvedValue(mockEmbedding);

      // Seed the database with test data
      const db = getDb(dbPath);

      // Insert a test repo
      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, primary_language,
          stars, is_starred, is_owned, pushed_at, synced_at, pushed_at_hash
        ) VALUES (
          1, 'owner1', 'repo1', 'owner1/repo1', 'https://github.com/owner1/repo1',
          'TypeScript', 100, 1, 0, datetime('now'), datetime('now'), 'hash1'
        )
      `).run();

      // Insert a test chunk
      db.prepare(`
        INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
        VALUES (1, 'metadata', 0, 'Test repository content', 10)
      `).run();

      // Insert embedding
      const embeddingBuffer = Buffer.from(new Float32Array(mockEmbedding).buffer);
      db.prepare(`
        INSERT INTO chunk_embeddings (rowid, embedding)
        VALUES (1, ?)
      `).run(embeddingBuffer);

      const result = await searchRepos('test query');

      expect(result.results.length).toBeGreaterThan(0);
      expect(result.results[0].repoFullName).toBe('owner1/repo1');
      expect(result.results[0].similarity).toBeGreaterThanOrEqual(0);
      expect(result.results[0].similarity).toBeLessThanOrEqual(1);
    });

    it('deduplicates results by repository', async () => {
      const mockEmbedding = createMockEmbedding();
      vi.mocked(embedQuery).mockResolvedValue(mockEmbedding);

      const db = getDb(dbPath);

      // Insert a repo with multiple chunks
      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, primary_language,
          stars, is_starred, is_owned, pushed_at, synced_at, pushed_at_hash
        ) VALUES (
          1, 'owner1', 'repo1', 'owner1/repo1', 'https://github.com/owner1/repo1',
          'TypeScript', 100, 1, 0, datetime('now'), datetime('now'), 'hash1'
        )
      `).run();

      // Insert two chunks for the same repo
      db.prepare(`
        INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
        VALUES (1, 'metadata', 0, 'Metadata content', 10)
      `).run();
      db.prepare(`
        INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
        VALUES (1, 'readme', 1, 'README content', 20)
      `).run();

      // Insert embeddings for both chunks
      const embeddingBuffer = Buffer.from(new Float32Array(mockEmbedding).buffer);
      db.prepare(`INSERT INTO chunk_embeddings (rowid, embedding) VALUES (1, ?)`).run(embeddingBuffer);
      db.prepare(`INSERT INTO chunk_embeddings (rowid, embedding) VALUES (2, ?)`).run(embeddingBuffer);

      const result = await searchRepos('test query', { limit: 10 });

      // Should return only one result per repo (deduplicated)
      expect(result.results).toHaveLength(1);
      expect(result.totalConsidered).toBe(2); // Both chunks were considered
    });

    it('applies language filter correctly', async () => {
      const mockEmbedding = createMockEmbedding();
      vi.mocked(embedQuery).mockResolvedValue(mockEmbedding);

      const db = getDb(dbPath);

      // Insert TypeScript repo
      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, primary_language,
          stars, is_starred, is_owned, pushed_at, synced_at, pushed_at_hash
        ) VALUES (
          1, 'owner1', 'ts-repo', 'owner1/ts-repo', 'https://github.com/owner1/ts-repo',
          'TypeScript', 100, 1, 0, datetime('now'), datetime('now'), 'hash1'
        )
      `).run();

      // Insert Python repo
      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, primary_language,
          stars, is_starred, is_owned, pushed_at, synced_at, pushed_at_hash
        ) VALUES (
          2, 'owner2', 'py-repo', 'owner2/py-repo', 'https://github.com/owner2/py-repo',
          'Python', 200, 0, 1, datetime('now'), datetime('now'), 'hash2'
        )
      `).run();

      // Insert chunks and embeddings
      const embeddingBuffer = Buffer.from(new Float32Array(mockEmbedding).buffer);
      db.prepare(`INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate) VALUES (1, 'metadata', 0, 'TS content', 10)`).run();
      db.prepare(`INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate) VALUES (2, 'metadata', 0, 'Python content', 10)`).run();
      db.prepare(`INSERT INTO chunk_embeddings (rowid, embedding) VALUES (1, ?)`).run(embeddingBuffer);
      db.prepare(`INSERT INTO chunk_embeddings (rowid, embedding) VALUES (2, ?)`).run(embeddingBuffer);

      // Search with TypeScript filter
      const result = await searchRepos('test', { language: 'TypeScript' });

      expect(result.results).toHaveLength(1);
      expect(result.results[0].language).toBe('TypeScript');
    });

    it('applies starred filter correctly', async () => {
      const mockEmbedding = createMockEmbedding();
      vi.mocked(embedQuery).mockResolvedValue(mockEmbedding);

      const db = getDb(dbPath);

      // Insert starred repo
      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, primary_language,
          stars, is_starred, is_owned, pushed_at, synced_at, pushed_at_hash
        ) VALUES (
          1, 'owner1', 'starred-repo', 'owner1/starred-repo', 'https://github.com/owner1/starred-repo',
          'TypeScript', 100, 1, 0, datetime('now'), datetime('now'), 'hash1'
        )
      `).run();

      // Insert non-starred repo
      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, primary_language,
          stars, is_starred, is_owned, pushed_at, synced_at, pushed_at_hash
        ) VALUES (
          2, 'owner2', 'unstarred-repo', 'owner2/unstarred-repo', 'https://github.com/owner2/unstarred-repo',
          'Python', 200, 0, 1, datetime('now'), datetime('now'), 'hash2'
        )
      `).run();

      // Insert chunks and embeddings
      const embeddingBuffer = Buffer.from(new Float32Array(mockEmbedding).buffer);
      db.prepare(`INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate) VALUES (1, 'metadata', 0, 'content', 10)`).run();
      db.prepare(`INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate) VALUES (2, 'metadata', 0, 'content', 10)`).run();
      db.prepare(`INSERT INTO chunk_embeddings (rowid, embedding) VALUES (1, ?)`).run(embeddingBuffer);
      db.prepare(`INSERT INTO chunk_embeddings (rowid, embedding) VALUES (2, ?)`).run(embeddingBuffer);

      const result = await searchRepos('test', { starred: true });

      expect(result.results).toHaveLength(1);
      expect(result.results[0].isStarred).toBe(true);
    });

    it('applies owned filter correctly', async () => {
      const mockEmbedding = createMockEmbedding();
      vi.mocked(embedQuery).mockResolvedValue(mockEmbedding);

      const db = getDb(dbPath);

      // Insert owned repo
      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, primary_language,
          stars, is_starred, is_owned, pushed_at, synced_at, pushed_at_hash
        ) VALUES (
          1, 'owner1', 'owned-repo', 'owner1/owned-repo', 'https://github.com/owner1/owned-repo',
          'TypeScript', 100, 0, 1, datetime('now'), datetime('now'), 'hash1'
        )
      `).run();

      // Insert non-owned repo
      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, primary_language,
          stars, is_starred, is_owned, pushed_at, synced_at, pushed_at_hash
        ) VALUES (
          2, 'owner2', 'not-owned', 'owner2/not-owned', 'https://github.com/owner2/not-owned',
          'Python', 200, 1, 0, datetime('now'), datetime('now'), 'hash2'
        )
      `).run();

      // Insert chunks and embeddings
      const embeddingBuffer = Buffer.from(new Float32Array(mockEmbedding).buffer);
      db.prepare(`INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate) VALUES (1, 'metadata', 0, 'content', 10)`).run();
      db.prepare(`INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate) VALUES (2, 'metadata', 0, 'content', 10)`).run();
      db.prepare(`INSERT INTO chunk_embeddings (rowid, embedding) VALUES (1, ?)`).run(embeddingBuffer);
      db.prepare(`INSERT INTO chunk_embeddings (rowid, embedding) VALUES (2, ?)`).run(embeddingBuffer);

      const result = await searchRepos('test', { owned: true });

      expect(result.results).toHaveLength(1);
      expect(result.results[0].isOwned).toBe(true);
    });

    it('applies owner filter correctly', async () => {
      const mockEmbedding = createMockEmbedding();
      vi.mocked(embedQuery).mockResolvedValue(mockEmbedding);

      const db = getDb(dbPath);

      // Insert repos from different owners
      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, primary_language,
          stars, is_starred, is_owned, pushed_at, synced_at, pushed_at_hash
        ) VALUES (
          1, 'alice', 'repo1', 'alice/repo1', 'https://github.com/alice/repo1',
          'TypeScript', 100, 1, 0, datetime('now'), datetime('now'), 'hash1'
        )
      `).run();

      db.prepare(`
        INSERT INTO repos (
          github_id, owner, name, full_name, url, primary_language,
          stars, is_starred, is_owned, pushed_at, synced_at, pushed_at_hash
        ) VALUES (
          2, 'bob', 'repo2', 'bob/repo2', 'https://github.com/bob/repo2',
          'Python', 200, 0, 1, datetime('now'), datetime('now'), 'hash2'
        )
      `).run();

      // Insert chunks and embeddings
      const embeddingBuffer = Buffer.from(new Float32Array(mockEmbedding).buffer);
      db.prepare(`INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate) VALUES (1, 'metadata', 0, 'content', 10)`).run();
      db.prepare(`INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate) VALUES (2, 'metadata', 0, 'content', 10)`).run();
      db.prepare(`INSERT INTO chunk_embeddings (rowid, embedding) VALUES (1, ?)`).run(embeddingBuffer);
      db.prepare(`INSERT INTO chunk_embeddings (rowid, embedding) VALUES (2, ?)`).run(embeddingBuffer);

      const result = await searchRepos('test', { owner: 'alice' });

      expect(result.results).toHaveLength(1);
      expect(result.results[0].owner).toBe('alice');
    });

    it('respects limit parameter', async () => {
      const mockEmbedding = createMockEmbedding();
      vi.mocked(embedQuery).mockResolvedValue(mockEmbedding);

      const db = getDb(dbPath);
      const embeddingBuffer = Buffer.from(new Float32Array(mockEmbedding).buffer);

      // Insert 5 repos with chunks and embeddings
      for (let i = 1; i <= 5; i++) {
        db.prepare(`
          INSERT INTO repos (
            github_id, owner, name, full_name, url, primary_language,
            stars, is_starred, is_owned, pushed_at, synced_at, pushed_at_hash
          ) VALUES (
            ?, 'owner', ?, ?, ?,
            'TypeScript', ?, 1, 0, datetime('now'), datetime('now'), ?
          )
        `).run(i, `repo${i}`, `owner/repo${i}`, `https://github.com/owner/repo${i}`, i * 100, `hash${i}`);

        db.prepare(`
          INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
          VALUES (?, 'metadata', 0, 'content', 10)
        `).run(i);

        // Use literal SQL for sqlite-vec virtual table (doesn't accept parameterized integers)
        db.prepare(`INSERT INTO chunk_embeddings (rowid, embedding) VALUES (${i}, ?)`).run(embeddingBuffer);
      }

      const result = await searchRepos('test', { limit: 2 });

      expect(result.results).toHaveLength(2);
    });

    it('uses default limit of 3 when not specified', async () => {
      const mockEmbedding = createMockEmbedding();
      vi.mocked(embedQuery).mockResolvedValue(mockEmbedding);

      const db = getDb(dbPath);
      const embeddingBuffer = Buffer.from(new Float32Array(mockEmbedding).buffer);

      // Insert 5 repos with chunks and embeddings
      for (let i = 1; i <= 5; i++) {
        db.prepare(`
          INSERT INTO repos (
            github_id, owner, name, full_name, url, primary_language,
            stars, is_starred, is_owned, pushed_at, synced_at, pushed_at_hash
          ) VALUES (
            ?, 'owner', ?, ?, ?,
            'TypeScript', ?, 1, 0, datetime('now'), datetime('now'), ?
          )
        `).run(i, `repo${i}`, `owner/repo${i}`, `https://github.com/owner/repo${i}`, i * 100, `hash${i}`);

        db.prepare(`
          INSERT INTO chunks (repo_id, chunk_type, chunk_index, content, token_estimate)
          VALUES (?, 'metadata', 0, 'content', 10)
        `).run(i);

        // Use literal SQL for sqlite-vec virtual table (doesn't accept parameterized integers)
        db.prepare(`INSERT INTO chunk_embeddings (rowid, embedding) VALUES (${i}, ?)`).run(embeddingBuffer);
      }

      const result = await searchRepos('test');

      expect(result.results).toHaveLength(3);
    });

    it('returns timing metrics', async () => {
      const mockEmbedding = createMockEmbedding();
      vi.mocked(embedQuery).mockResolvedValue(mockEmbedding);

      const result = await searchRepos('test query');

      expect(typeof result.queryEmbeddingMs).toBe('number');
      expect(typeof result.searchMs).toBe('number');
      expect(result.queryEmbeddingMs).toBeGreaterThanOrEqual(0);
      expect(result.searchMs).toBeGreaterThanOrEqual(0);
    });

    it('returns empty results on database error - does not throw', async () => {
      const mockEmbedding = createMockEmbedding();
      vi.mocked(embedQuery).mockResolvedValue(mockEmbedding);

      // Use a non-existent database path
      vi.mocked(loadConfigAsync).mockResolvedValue({
        githubPat: 'ghp_test',
        geminiKey: 'test-key',
        dbPath: '/nonexistent/path/db.sqlite',
      });

      const result = await searchRepos('test query');

      expect(result.results).toHaveLength(0);
      expect(result.totalConsidered).toBe(0);
    });
  });
});
