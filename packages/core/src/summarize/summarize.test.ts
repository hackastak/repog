import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { summarizeRepo, buildSummarizePrompt } from './summarize.js';
import { getDb, closeDb } from '../db/index.js';
import { initDb } from '../db/init.js';

// Mock dependencies
vi.mock('../config/config.js', () => ({
  loadConfig: vi.fn(),
  isConfigured: vi.fn(),
}));

vi.mock('../gemini/llm.js', () => ({
  streamLLM: vi.fn(),
  isLLMError: vi.fn((result) => 'error' in result && !('text' in result)),
}));

// Import mocked modules
import { loadConfig } from '../config/config.js';
import { streamLLM } from '../gemini/llm.js';

const mockedLoadConfig = vi.mocked(loadConfig);
const mockedStreamLLM = vi.mocked(streamLLM);

describe('summarize/summarize', () => {
  const DB_PATH = ':memory:';

  beforeEach(() => {
    vi.clearAllMocks();

    // Default mocks
    mockedLoadConfig.mockReturnValue({
      githubPat: 'test-pat',
      geminiKey: 'test-gemini-key',
      dbPath: DB_PATH,
    });

    // Initialize DB
    initDb(DB_PATH);
    const db = getDb(DB_PATH);
    // Seed data
    db.prepare(`
      INSERT INTO repos (github_id, owner, name, full_name, url, pushed_at, synced_at)
      VALUES (1, 'owner', 'repo', 'owner/repo', 'https://github.com/owner/repo', datetime('now'), datetime('now'))
    `).run();

    db.prepare(`
      INSERT INTO chunks (repo_id, chunk_type, chunk_index, content)
      VALUES 
      (1, 'readme', 0, 'README content'),
      (1, 'file_tree', 0, 'src/index.ts'),
      (1, 'file_tree', 1, 'package.json')
    `).run();
  });

  afterEach(() => {
    closeDb();
    vi.clearAllMocks();
  });

  describe('summarizeRepo', () => {
    it('returns a populated SummarizeResult with summary and stats on success', async () => {
      mockedStreamLLM.mockResolvedValue({
        text: '## Overview\nSummary.\n## Tech Stack\nStack.\n## Use Cases\nCases.',
        inputTokens: 100,
        outputTokens: 50,
        durationMs: 500,
      });

      const result = await summarizeRepo({ repo: 'owner/repo' });

      expect(result.summary).toBe('## Overview\nSummary.\n## Tech Stack\nStack.\n## Use Cases\nCases.');
      expect(result.repo).toBe('owner/repo');
      expect(result.chunksUsed).toBe(3); // readme + 2 file_tree
      expect(result.inputTokens).toBe(100);
      expect(result.outputTokens).toBe(50);
      expect(result.durationMs).toBeGreaterThanOrEqual(0);
    });

    it('calls onChunk for each streamed chunk during summarizeRepo', async () => {
      mockedStreamLLM.mockImplementation(async (_apiKey, _prompt, _systemPrompt, onChunk) => {
        if (onChunk) {
          onChunk('Chunk 1');
          onChunk('Chunk 2');
        }
        return {
          text: 'Chunk 1Chunk 2',
          inputTokens: 10,
          outputTokens: 10,
          durationMs: 100,
        };
      });

      const chunks: string[] = [];
      await summarizeRepo({ repo: 'owner/repo' }, (text) => chunks.push(text));

      expect(chunks).toHaveLength(2);
      expect(chunks[0]).toBe('Chunk 1');
      expect(chunks[1]).toBe('Chunk 2');
    });

    it('fetches all chunks for the specified repo and includes them in context', async () => {
      mockedStreamLLM.mockResolvedValue({
        text: 'Summary',
        inputTokens: 10,
        outputTokens: 10,
        durationMs: 100,
      });

      await summarizeRepo({ repo: 'owner/repo' });

      expect(mockedStreamLLM).toHaveBeenCalled();
      const [, prompt] = mockedStreamLLM.mock.calls[0];
      
      expect(prompt).toContain('Repository: owner/repo');
      expect(prompt).toContain('--- readme ---');
      expect(prompt).toContain('README content');
      expect(prompt).toContain('--- file_tree ---');
      expect(prompt).toContain('src/index.ts');
      expect(prompt).toContain('package.json');
    });

    it('repo name matching is case-insensitive', async () => {
      mockedStreamLLM.mockResolvedValue({
        text: 'Summary',
        inputTokens: 10,
        outputTokens: 10,
        durationMs: 100,
      });

      const result = await summarizeRepo({ repo: 'Owner/Repo' });

      expect(result.chunksUsed).toBe(3);
      expect(result.repo).toBe('Owner/Repo'); // Returns the input repo name
    });

    it('returns safe result with no-data message when repo has no chunks', async () => {
      // Create a repo with no chunks
      const db = getDb(DB_PATH);
      db.prepare(`
        INSERT INTO repos (github_id, owner, name, full_name, url, pushed_at, synced_at)
        VALUES (2, 'empty', 'repo', 'empty/repo', 'https://github.com/empty/repo', datetime('now'), datetime('now'))
      `).run();

      const result = await summarizeRepo({ repo: 'empty/repo' });

      expect(result.summary).toContain('No data found for this repository');
      expect(result.chunksUsed).toBe(0);
      expect(mockedStreamLLM).not.toHaveBeenCalled();
    });
    
    it('returns safe result with no-data message when repo does not exist', async () => {
      const result = await summarizeRepo({ repo: 'nonexistent/repo' });

      expect(result.summary).toContain('No data found for this repository');
      expect(result.chunksUsed).toBe(0);
      expect(mockedStreamLLM).not.toHaveBeenCalled();
    });

    it('returns safe result with error message when streamLLM returns LLMError', async () => {
      mockedStreamLLM.mockResolvedValue({
        error: 'API error',
        durationMs: 100,
      });

      const result = await summarizeRepo({ repo: 'owner/repo' });

      expect(result.summary).toContain('Error generating summary: API error');
      expect(result.chunksUsed).toBe(3);
    });
    
    it('returns safe result with error message when not configured', async () => {
       mockedLoadConfig.mockReturnValue({
          githubPat: 'test-pat',
          geminiKey: null,
          dbPath: DB_PATH,
       });
       
       const result = await summarizeRepo({ repo: 'owner/repo' });
       
       expect(result.summary).toContain('Error: Gemini API key is not configured');
    });
  });

  describe('buildSummarizePrompt', () => {
    it('includes all chunk contents formatted with headers', () => {
      const chunks = [
        { chunk_type: 'readme', content: 'README' },
        { chunk_type: 'file_tree', content: 'TREE' }
      ];
      const prompt = buildSummarizePrompt('owner/repo', chunks);
      
      expect(prompt).toContain('Repository: owner/repo');
      expect(prompt).toContain('--- readme ---');
      expect(prompt).toContain('README');
      expect(prompt).toContain('--- file_tree ---');
      expect(prompt).toContain('TREE');
    });
  });
});
