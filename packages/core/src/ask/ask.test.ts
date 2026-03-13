import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { askQuestion, buildAskPrompt } from './ask.js';
import type { SearchResult } from '../search/query.js';

// Mock dependencies
vi.mock('../config/config.js', () => ({
  loadConfig: vi.fn(),
  loadConfigAsync: vi.fn(),
  isConfigured: vi.fn(),
}));

vi.mock('../search/query.js', () => ({
  searchRepos: vi.fn(),
}));

vi.mock('../gemini/llm.js', () => ({
  streamLLM: vi.fn(),
  isLLMError: vi.fn((result) => 'error' in result && !('text' in result)),
}));

// Import mocked modules
import { loadConfig, loadConfigAsync, isConfigured } from '../config/config.js';
import { searchRepos } from '../search/query.js';
import { streamLLM } from '../gemini/llm.js';

const mockedLoadConfig = vi.mocked(loadConfig);
const mockedLoadConfigAsync = vi.mocked(loadConfigAsync);
const mockedIsConfigured = vi.mocked(isConfigured);
const mockedSearchRepos = vi.mocked(searchRepos);
const mockedStreamLLM = vi.mocked(streamLLM);

/**
 * Helper to create a mock SearchResult.
 */
function createMockSearchResult(
  repoFullName: string,
  chunkType: 'metadata' | 'readme' | 'file_tree',
  similarity: number,
  content: string = 'Test content'
): SearchResult {
  const [owner, name] = repoFullName.split('/');
  return {
    repoFullName,
    owner,
    repoName: name,
    description: 'Test description',
    language: 'TypeScript',
    stars: 100,
    isStarred: true,
    isOwned: false,
    htmlUrl: `https://github.com/${repoFullName}`,
    chunkType,
    content,
    similarity,
  };
}

describe('ask/ask', () => {
  beforeEach(() => {
    vi.clearAllMocks();

    // Default mocks
    mockedIsConfigured.mockReturnValue(true);
    mockedLoadConfig.mockReturnValue({
      githubPat: null,
      geminiKey: null,
      dbPath: '/test/db.db',
    });
    mockedLoadConfigAsync.mockResolvedValue({
      githubPat: 'test-pat',
      geminiKey: 'test-gemini-key',
      dbPath: '/test/db.db',
    });
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('askQuestion', () => {
    it('returns a populated AskResult with answer and sources on success', async () => {
      const mockResults: SearchResult[] = [
        createMockSearchResult('owner/repo1', 'readme', 0.95, 'README content'),
        createMockSearchResult('owner/repo2', 'metadata', 0.85, 'Metadata content'),
      ];

      mockedSearchRepos.mockResolvedValue({
        results: mockResults,
        totalConsidered: 2,
        queryEmbeddingMs: 50,
        searchMs: 10,
      });

      mockedStreamLLM.mockResolvedValue({
        text: 'This is the answer based on the context.',
        inputTokens: 100,
        outputTokens: 50,
        durationMs: 500,
      });

      const result = await askQuestion({ question: 'What is this project?' });

      expect(result.answer).toBe('This is the answer based on the context.');
      expect(result.question).toBe('What is this project?');
      expect(result.sources).toHaveLength(2);
      expect(result.sources[0].repoFullName).toBe('owner/repo1');
      expect(result.sources[0].similarity).toBe(0.95);
      expect(result.inputTokens).toBe(100);
      expect(result.outputTokens).toBe(50);
      expect(result.durationMs).toBeGreaterThanOrEqual(0);
    });

    it('calls onChunk for each streamed chunk during askQuestion', async () => {
      const mockResults: SearchResult[] = [
        createMockSearchResult('owner/repo1', 'readme', 0.9),
      ];

      mockedSearchRepos.mockResolvedValue({
        results: mockResults,
        totalConsidered: 1,
        queryEmbeddingMs: 50,
        searchMs: 10,
      });

      // Capture the onChunk callback and simulate calling it
      mockedStreamLLM.mockImplementation(async (_apiKey, _prompt, _systemPrompt, onChunk) => {
        if (onChunk) {
          onChunk('First ');
          onChunk('chunk.');
        }
        return {
          text: 'First chunk.',
          inputTokens: 50,
          outputTokens: 20,
          durationMs: 300,
        };
      });

      const chunks: string[] = [];
      const onChunk = (text: string): void => {
        chunks.push(text);
      };

      await askQuestion({ question: 'Test question' }, onChunk);

      expect(chunks).toHaveLength(2);
      expect(chunks[0]).toBe('First ');
      expect(chunks[1]).toBe('chunk.');
    });

    it('filters results to only specified repo when options.repo is provided', async () => {
      const mockResults: SearchResult[] = [
        createMockSearchResult('owner/repo1', 'readme', 0.95),
        createMockSearchResult('owner/repo2', 'metadata', 0.85),
        createMockSearchResult('other/repo3', 'file_tree', 0.80),
      ];

      mockedSearchRepos.mockResolvedValue({
        results: mockResults,
        totalConsidered: 3,
        queryEmbeddingMs: 50,
        searchMs: 10,
      });

      mockedStreamLLM.mockResolvedValue({
        text: 'Answer about repo1.',
        inputTokens: 80,
        outputTokens: 30,
        durationMs: 400,
      });

      const result = await askQuestion({
        question: 'What is repo1?',
        repo: 'owner/repo1',
      });

      // Verify streamLLM was called - the prompt should only include repo1
      expect(mockedStreamLLM).toHaveBeenCalled();
      const [, prompt] = mockedStreamLLM.mock.calls[0];
      expect(prompt).toContain('owner/repo1');
      expect(prompt).not.toContain('owner/repo2');
      expect(prompt).not.toContain('other/repo3');

      // Sources should only contain repo1
      expect(result.sources).toHaveLength(1);
      expect(result.sources[0].repoFullName).toBe('owner/repo1');
    });

    it('returns safe AskResult with no-information message when searchRepos returns empty', async () => {
      mockedSearchRepos.mockResolvedValue({
        results: [],
        totalConsidered: 0,
        queryEmbeddingMs: 50,
        searchMs: 10,
      });

      const result = await askQuestion({ question: 'Unknown topic?' });

      expect(result.answer).toBe(
        "I couldn't find any relevant information in your knowledge base to answer this question."
      );
      expect(result.sources).toHaveLength(0);
      expect(result.question).toBe('Unknown topic?');
      // Should not throw
      expect(result.inputTokens).toBe(0);
      expect(result.outputTokens).toBe(0);
    });

    it('returns safe AskResult with error message when streamLLM returns LLMError', async () => {
      const mockResults: SearchResult[] = [
        createMockSearchResult('owner/repo1', 'readme', 0.9),
      ];

      mockedSearchRepos.mockResolvedValue({
        results: mockResults,
        totalConsidered: 1,
        queryEmbeddingMs: 50,
        searchMs: 10,
      });

      mockedStreamLLM.mockResolvedValue({
        error: 'API rate limit exceeded',
        durationMs: 100,
      });

      const result = await askQuestion({ question: 'Test question' });

      expect(result.answer).toContain('Error generating answer');
      expect(result.answer).toContain('API rate limit exceeded');
      expect(result.sources).toHaveLength(0);
      // Should not throw
      expect(result.question).toBe('Test question');
    });

    it('deduplicates sources by repoFullName - only one entry per repo', async () => {
      const mockResults: SearchResult[] = [
        createMockSearchResult('owner/repo1', 'readme', 0.95),
        createMockSearchResult('owner/repo1', 'metadata', 0.90), // Same repo, different chunk
        createMockSearchResult('owner/repo1', 'file_tree', 0.85), // Same repo again
        createMockSearchResult('owner/repo2', 'readme', 0.80),
      ];

      mockedSearchRepos.mockResolvedValue({
        results: mockResults,
        totalConsidered: 4,
        queryEmbeddingMs: 50,
        searchMs: 10,
      });

      mockedStreamLLM.mockResolvedValue({
        text: 'Answer.',
        inputTokens: 100,
        outputTokens: 10,
        durationMs: 200,
      });

      const result = await askQuestion({ question: 'Test' });

      // Should have only 2 unique repos
      expect(result.sources).toHaveLength(2);
      const repoNames = result.sources.map((s) => s.repoFullName);
      expect(repoNames).toContain('owner/repo1');
      expect(repoNames).toContain('owner/repo2');
      // Should keep highest similarity for repo1 (0.95, not 0.90 or 0.85)
      const repo1Source = result.sources.find((s) => s.repoFullName === 'owner/repo1');
      expect(repo1Source?.similarity).toBe(0.95);
    });

    it('sorts sources by similarity descending and caps at 5', async () => {
      const mockResults: SearchResult[] = [
        createMockSearchResult('owner/repo1', 'readme', 0.60),
        createMockSearchResult('owner/repo2', 'readme', 0.95),
        createMockSearchResult('owner/repo3', 'readme', 0.70),
        createMockSearchResult('owner/repo4', 'readme', 0.80),
        createMockSearchResult('owner/repo5', 'readme', 0.85),
        createMockSearchResult('owner/repo6', 'readme', 0.50), // Should be excluded (6th)
        createMockSearchResult('owner/repo7', 'readme', 0.40), // Should be excluded (7th)
      ];

      mockedSearchRepos.mockResolvedValue({
        results: mockResults,
        totalConsidered: 7,
        queryEmbeddingMs: 50,
        searchMs: 10,
      });

      mockedStreamLLM.mockResolvedValue({
        text: 'Answer.',
        inputTokens: 150,
        outputTokens: 10,
        durationMs: 200,
      });

      const result = await askQuestion({ question: 'Test' });

      // Should be capped at 5
      expect(result.sources).toHaveLength(5);

      // Should be sorted by similarity descending
      expect(result.sources[0].similarity).toBe(0.95);
      expect(result.sources[1].similarity).toBe(0.85);
      expect(result.sources[2].similarity).toBe(0.80);
      expect(result.sources[3].similarity).toBe(0.70);
      expect(result.sources[4].similarity).toBe(0.60);

      // repo6 and repo7 should be excluded
      const repoNames = result.sources.map((s) => s.repoFullName);
      expect(repoNames).not.toContain('owner/repo6');
      expect(repoNames).not.toContain('owner/repo7');
    });

    it('durationMs is present and non-negative', async () => {
      mockedSearchRepos.mockResolvedValue({
        results: [],
        totalConsidered: 0,
        queryEmbeddingMs: 50,
        searchMs: 10,
      });

      const result = await askQuestion({ question: 'Test' });

      expect(typeof result.durationMs).toBe('number');
      expect(result.durationMs).toBeGreaterThanOrEqual(0);
    });
  });

  describe('buildAskPrompt', () => {
    it('includes all chunk contents in the formatted context block', () => {
      const chunks: SearchResult[] = [
        createMockSearchResult('owner/repo1', 'readme', 0.95, 'README content here'),
        createMockSearchResult('owner/repo2', 'metadata', 0.85, 'Metadata content here'),
        createMockSearchResult('owner/repo3', 'file_tree', 0.70, 'File tree content here'),
      ];

      const prompt = buildAskPrompt('What are these projects?', chunks);

      // Check question is included
      expect(prompt).toContain('Question: What are these projects?');

      // Check all chunks are formatted correctly
      expect(prompt).toContain('--- owner/repo1 (readme) ---');
      expect(prompt).toContain('README content here');

      expect(prompt).toContain('--- owner/repo2 (metadata) ---');
      expect(prompt).toContain('Metadata content here');

      expect(prompt).toContain('--- owner/repo3 (file_tree) ---');
      expect(prompt).toContain('File tree content here');

      // Check instructions are included
      expect(prompt).toContain('Answer the question based on the context above');
    });
  });
});
