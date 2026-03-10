import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import nock from 'nock';
import {
  embedChunks,
  embedQuery,
  getEmbeddingModel,
  getEmbeddingDimensions,
} from './embeddings.js';

const TEST_API_KEY = 'test-gemini-api-key';
const GEMINI_API_HOST = 'https://generativelanguage.googleapis.com';

// Generate a mock embedding with 768 dimensions
function createMockEmbedding(): number[] {
  return Array(768).fill(0).map((_, i) => i * 0.001);
}

describe('gemini/embeddings', () => {
  beforeEach(() => {
    nock.cleanAll();
  });

  afterEach(() => {
    nock.cleanAll();
  });

  describe('embedChunks', () => {
    it('returns empty results for empty chunks array', async () => {
      const result = await embedChunks(TEST_API_KEY, []);

      expect(result.results).toHaveLength(0);
      expect(result.errors).toHaveLength(0);
    });

    it('returns embeddings for valid chunks', async () => {
      const mockEmbedding = createMockEmbedding();

      nock(GEMINI_API_HOST)
        .post(/.*batchEmbedContents.*/)
        .reply(200, {
          embeddings: [
            { values: mockEmbedding },
            { values: mockEmbedding },
          ],
        });

      const result = await embedChunks(TEST_API_KEY, [
        { id: 1, content: 'First chunk content' },
        { id: 2, content: 'Second chunk content' },
      ]);

      expect(result.results).toHaveLength(2);
      expect(result.errors).toHaveLength(0);
      expect(result.results[0].chunkId).toBe(1);
      expect(result.results[0].embedding).toHaveLength(768);
      expect(result.results[1].chunkId).toBe(2);
    });

    it('returns error for chunk with missing embedding in response', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*batchEmbedContents.*/)
        .reply(200, {
          embeddings: [
            null, // Missing embedding
          ],
        });

      const result = await embedChunks(TEST_API_KEY, [
        { id: 1, content: 'Test content' },
      ]);

      expect(result.results).toHaveLength(0);
      expect(result.errors).toHaveLength(1);
      expect(result.errors[0].chunkId).toBe(1);
      expect(result.errors[0].error).toBe('No embedding returned from API');
    });

    it('returns error for chunk with invalid embedding dimensions', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*batchEmbedContents.*/)
        .reply(200, {
          embeddings: [
            { values: [0.1, 0.2, 0.3] }, // Wrong dimensions
          ],
        });

      const result = await embedChunks(TEST_API_KEY, [
        { id: 1, content: 'Test content' },
      ]);

      expect(result.results).toHaveLength(0);
      expect(result.errors).toHaveLength(1);
      expect(result.errors[0].chunkId).toBe(1);
      expect(result.errors[0].error).toContain('Invalid embedding dimensions');
    });

    it('returns all chunks as errors when API request fails', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*batchEmbedContents.*/)
        .reply(500, { error: { message: 'Internal server error' } });

      const result = await embedChunks(TEST_API_KEY, [
        { id: 1, content: 'First chunk' },
        { id: 2, content: 'Second chunk' },
      ]);

      expect(result.results).toHaveLength(0);
      expect(result.errors).toHaveLength(2);
      expect(result.errors[0].chunkId).toBe(1);
      expect(result.errors[1].chunkId).toBe(2);
    });

    it('handles partial success with mixed results and errors', async () => {
      const mockEmbedding = createMockEmbedding();

      nock(GEMINI_API_HOST)
        .post(/.*batchEmbedContents.*/)
        .reply(200, {
          embeddings: [
            { values: mockEmbedding }, // Valid
            { values: null },          // Missing values
            { values: mockEmbedding }, // Valid
          ],
        });

      const result = await embedChunks(TEST_API_KEY, [
        { id: 1, content: 'First chunk' },
        { id: 2, content: 'Second chunk' },
        { id: 3, content: 'Third chunk' },
      ]);

      expect(result.results).toHaveLength(2);
      expect(result.errors).toHaveLength(1);
      expect(result.results[0].chunkId).toBe(1);
      expect(result.results[1].chunkId).toBe(3);
      expect(result.errors[0].chunkId).toBe(2);
    });
  });

  describe('embedQuery', () => {
    it('returns embedding for valid query', async () => {
      const mockEmbedding = createMockEmbedding();

      nock(GEMINI_API_HOST)
        .post(/.*embedContent.*/)
        .reply(200, {
          embedding: { values: mockEmbedding },
        });

      const result = await embedQuery(TEST_API_KEY, 'What is machine learning?');

      expect(result).not.toBeNull();
      expect(result).toHaveLength(768);
    });

    it('returns null when embedding is missing from response', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*embedContent.*/)
        .reply(200, {
          embedding: null,
        });

      const result = await embedQuery(TEST_API_KEY, 'Test query');

      expect(result).toBeNull();
    });

    it('returns null when embedding has wrong dimensions', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*embedContent.*/)
        .reply(200, {
          embedding: { values: [0.1, 0.2, 0.3] },
        });

      const result = await embedQuery(TEST_API_KEY, 'Test query');

      expect(result).toBeNull();
    });

    it('returns null on API error - does not throw', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*embedContent.*/)
        .reply(500, { error: { message: 'Internal server error' } });

      const result = await embedQuery(TEST_API_KEY, 'Test query');

      expect(result).toBeNull();
    });

    it('returns null on network error - does not throw', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*embedContent.*/)
        .replyWithError('Network connection failed');

      const result = await embedQuery(TEST_API_KEY, 'Test query');

      expect(result).toBeNull();
    });
  });

  describe('getEmbeddingModel', () => {
    it('returns the embedding model name', () => {
      const model = getEmbeddingModel();

      expect(model).toBe('text-embedding-004');
    });
  });

  describe('getEmbeddingDimensions', () => {
    it('returns 768 dimensions', () => {
      const dimensions = getEmbeddingDimensions();

      expect(dimensions).toBe(768);
    });
  });
});
