import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { recommendRepos, buildRecommendPrompt, type RecommendOptions } from './recommend.js';
import type { SearchResult, SearchQueryResult } from '../search/query.js';
import type { LLMResult, LLMError } from '../gemini/llm.js';

// Mock dependencies
vi.mock('../config/config.js', () => ({
  loadConfig: vi.fn(),
  loadConfigAsync: vi.fn(),
}));

vi.mock('../search/query.js', () => ({
  searchRepos: vi.fn(),
}));

vi.mock('../gemini/llm.js', () => ({
  callLLM: vi.fn(),
  isLLMError: vi.fn((result: LLMResult | LLMError) => 'error' in result && !('text' in result)),
}));

// Import mocked modules
import { loadConfig, loadConfigAsync } from '../config/config.js';
import { searchRepos } from '../search/query.js';
import { callLLM } from '../gemini/llm.js';

const mockLoadConfig = vi.mocked(loadConfig);
const mockLoadConfigAsync = vi.mocked(loadConfigAsync);
const mockSearchRepos = vi.mocked(searchRepos);
const mockCallLLM = vi.mocked(callLLM);

/**
 * Create a mock SearchResult.
 */
function createMockSearchResult(overrides: Partial<SearchResult> = {}): SearchResult {
  return {
    repoFullName: 'owner/repo',
    owner: 'owner',
    repoName: 'repo',
    description: 'A test repository',
    language: 'TypeScript',
    stars: 100,
    isStarred: false,
    isOwned: true,
    htmlUrl: 'https://github.com/owner/repo',
    chunkType: 'metadata',
    content: 'Repository content',
    similarity: 0.95,
    ...overrides,
  };
}

/**
 * Create a mock SearchQueryResult.
 */
function createMockSearchQueryResult(results: SearchResult[]): SearchQueryResult {
  return {
    results,
    totalConsidered: results.length,
    queryEmbeddingMs: 50,
    searchMs: 20,
  };
}

/**
 * Create a valid LLM JSON response.
 */
function createValidLLMResponse(recommendations: Array<{
  rank: number;
  repoFullName: string;
  htmlUrl: string;
  reasoning: string;
}>): string {
  return JSON.stringify(recommendations);
}

describe('recommend/recommend', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockLoadConfig.mockReturnValue({
      githubPat: null,
      geminiKey: null,
      dbPath: '/test/path.db',
    });
    mockLoadConfigAsync.mockResolvedValue({
      githubPat: 'test-token',
      geminiKey: 'test-gemini-key',
      dbPath: '/test/path.db',
    });
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('recommendRepos', () => {
    it('returns ranked Recommendation[] correctly parsed from a valid LLM JSON response', async () => {
      const candidates = [
        createMockSearchResult({ repoFullName: 'owner/repo1', htmlUrl: 'https://github.com/owner/repo1' }),
        createMockSearchResult({ repoFullName: 'owner/repo2', htmlUrl: 'https://github.com/owner/repo2' }),
        createMockSearchResult({ repoFullName: 'owner/repo3', htmlUrl: 'https://github.com/owner/repo3' }),
      ];

      mockSearchRepos.mockResolvedValue(createMockSearchQueryResult(candidates));

      const llmResponse = createValidLLMResponse([
        { rank: 1, repoFullName: 'owner/repo1', htmlUrl: 'https://github.com/owner/repo1', reasoning: 'Best match for the query.' },
        { rank: 2, repoFullName: 'owner/repo2', htmlUrl: 'https://github.com/owner/repo2', reasoning: 'Good secondary option.' },
        { rank: 3, repoFullName: 'owner/repo3', htmlUrl: 'https://github.com/owner/repo3', reasoning: 'Also relevant.' },
      ]);

      mockCallLLM.mockResolvedValue({
        text: llmResponse,
        inputTokens: 100,
        outputTokens: 50,
        durationMs: 500,
      } as LLMResult);

      const options: RecommendOptions = { query: 'machine learning tools', limit: 3 };
      const result = await recommendRepos(options);

      expect(result.recommendations).toHaveLength(3);
      expect(result.recommendations[0].rank).toBe(1);
      expect(result.recommendations[0].repoFullName).toBe('owner/repo1');
      expect(result.recommendations[0].reasoning).toBe('Best match for the query.');
      expect(result.query).toBe('machine learning tools');
    });

    it('caps results at options.limit even if LLM returns more', async () => {
      const candidates = Array.from({ length: 10 }, (_, i) =>
        createMockSearchResult({
          repoFullName: `owner/repo${i + 1}`,
          htmlUrl: `https://github.com/owner/repo${i + 1}`,
        })
      );

      mockSearchRepos.mockResolvedValue(createMockSearchQueryResult(candidates));

      // LLM returns 5 recommendations
      const llmResponse = createValidLLMResponse([
        { rank: 1, repoFullName: 'owner/repo1', htmlUrl: 'https://github.com/owner/repo1', reasoning: 'Reason 1' },
        { rank: 2, repoFullName: 'owner/repo2', htmlUrl: 'https://github.com/owner/repo2', reasoning: 'Reason 2' },
        { rank: 3, repoFullName: 'owner/repo3', htmlUrl: 'https://github.com/owner/repo3', reasoning: 'Reason 3' },
        { rank: 4, repoFullName: 'owner/repo4', htmlUrl: 'https://github.com/owner/repo4', reasoning: 'Reason 4' },
        { rank: 5, repoFullName: 'owner/repo5', htmlUrl: 'https://github.com/owner/repo5', reasoning: 'Reason 5' },
      ]);

      mockCallLLM.mockResolvedValue({
        text: llmResponse,
        inputTokens: 100,
        outputTokens: 50,
        durationMs: 500,
      } as LLMResult);

      // Request only 2
      const options: RecommendOptions = { query: 'test query', limit: 2 };
      const result = await recommendRepos(options);

      expect(result.recommendations).toHaveLength(2);
      expect(result.recommendations[0].rank).toBe(1);
      expect(result.recommendations[1].rank).toBe(2);
    });

    it('returns empty result when searchRepos returns no candidates', async () => {
      mockSearchRepos.mockResolvedValue(createMockSearchQueryResult([]));

      const options: RecommendOptions = { query: 'obscure query', limit: 3 };
      const result = await recommendRepos(options);

      expect(result.recommendations).toHaveLength(0);
      expect(result.candidatesConsidered).toBe(0);
      expect(result.query).toBe('obscure query');
      // callLLM should not have been called
      expect(mockCallLLM).not.toHaveBeenCalled();
    });

    it('returns empty result when callLLM returns an LLMError - does not throw', async () => {
      const candidates = [createMockSearchResult()];
      mockSearchRepos.mockResolvedValue(createMockSearchQueryResult(candidates));

      mockCallLLM.mockResolvedValue({
        error: 'API rate limit exceeded',
        durationMs: 100,
      } as LLMError);

      const options: RecommendOptions = { query: 'test', limit: 3 };
      const result = await recommendRepos(options);

      expect(result.recommendations).toHaveLength(0);
      expect(result.candidatesConsidered).toBe(1);
      // Should not throw
      expect(result.query).toBe('test');
    });

    it('strips markdown code fences before JSON parsing', async () => {
      const candidates = [createMockSearchResult()];
      mockSearchRepos.mockResolvedValue(createMockSearchQueryResult(candidates));

      // LLM wraps response in code fences
      const llmResponse = '```json\n[{"rank":1,"repoFullName":"owner/repo","htmlUrl":"https://github.com/owner/repo","reasoning":"Good repo."}]\n```';

      mockCallLLM.mockResolvedValue({
        text: llmResponse,
        inputTokens: 50,
        outputTokens: 30,
        durationMs: 200,
      } as LLMResult);

      const options: RecommendOptions = { query: 'test', limit: 3 };
      const result = await recommendRepos(options);

      expect(result.recommendations).toHaveLength(1);
      expect(result.recommendations[0].repoFullName).toBe('owner/repo');
    });

    it('returns empty result when LLM response is invalid JSON - does not throw', async () => {
      const candidates = [createMockSearchResult()];
      mockSearchRepos.mockResolvedValue(createMockSearchQueryResult(candidates));

      // Invalid JSON
      mockCallLLM.mockResolvedValue({
        text: 'This is not valid JSON at all!',
        inputTokens: 50,
        outputTokens: 10,
        durationMs: 200,
      } as LLMResult);

      const options: RecommendOptions = { query: 'test', limit: 3 };
      const result = await recommendRepos(options);

      expect(result.recommendations).toHaveLength(0);
      // Should not throw
      expect(result.query).toBe('test');
    });

    it('filters are correctly passed through to searchRepos', async () => {
      mockSearchRepos.mockResolvedValue(createMockSearchQueryResult([]));

      const options: RecommendOptions = {
        query: 'typescript libraries',
        limit: 3,
        filters: {
          language: 'TypeScript',
          starred: true,
          owned: false,
          owner: 'microsoft',
        },
      };

      await recommendRepos(options);

      expect(mockSearchRepos).toHaveBeenCalledWith('typescript libraries', {
        limit: 15, // limit * 5
        language: 'TypeScript',
        starred: true,
        owned: false,
        owner: 'microsoft',
      });
    });

    it('candidatesConsidered reflects the number of repos passed to the LLM', async () => {
      const candidates = Array.from({ length: 7 }, (_, i) =>
        createMockSearchResult({
          repoFullName: `owner/repo${i + 1}`,
          htmlUrl: `https://github.com/owner/repo${i + 1}`,
        })
      );

      mockSearchRepos.mockResolvedValue(createMockSearchQueryResult(candidates));

      const llmResponse = createValidLLMResponse([
        { rank: 1, repoFullName: 'owner/repo1', htmlUrl: 'https://github.com/owner/repo1', reasoning: 'Best' },
      ]);

      mockCallLLM.mockResolvedValue({
        text: llmResponse,
        inputTokens: 100,
        outputTokens: 30,
        durationMs: 300,
      } as LLMResult);

      const options: RecommendOptions = { query: 'test', limit: 3 };
      const result = await recommendRepos(options);

      expect(result.candidatesConsidered).toBe(7);
    });

    it('durationMs is present and non-negative', async () => {
      const candidates = [createMockSearchResult()];
      mockSearchRepos.mockResolvedValue(createMockSearchQueryResult(candidates));

      mockCallLLM.mockResolvedValue({
        text: '[]',
        inputTokens: 10,
        outputTokens: 5,
        durationMs: 100,
      } as LLMResult);

      const options: RecommendOptions = { query: 'test', limit: 3 };
      const result = await recommendRepos(options);

      expect(typeof result.durationMs).toBe('number');
      expect(result.durationMs).toBeGreaterThanOrEqual(0);
    });
  });

  describe('buildRecommendPrompt', () => {
    it('builds a properly formatted prompt with candidates', () => {
      const candidates: SearchResult[] = [
        createMockSearchResult({
          repoFullName: 'facebook/react',
          description: 'A JavaScript library for building user interfaces',
          language: 'JavaScript',
          htmlUrl: 'https://github.com/facebook/react',
        }),
        createMockSearchResult({
          repoFullName: 'vuejs/vue',
          description: null,
          language: null,
          htmlUrl: 'https://github.com/vuejs/vue',
        }),
      ];

      const prompt = buildRecommendPrompt('frontend frameworks', candidates, 2);

      expect(prompt).toContain('Query: frontend frameworks');
      expect(prompt).toContain('- facebook/react: A JavaScript library for building user interfaces (JavaScript)');
      expect(prompt).toContain('- vuejs/vue: No description (Unknown language)');
      expect(prompt).toContain('recommend the top 2 most relevant repositories');
      expect(prompt).toContain('"rank": 1');
    });
  });
});
