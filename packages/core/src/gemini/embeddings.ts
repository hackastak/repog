import { GoogleGenerativeAI, TaskType } from '@google/generative-ai';

/**
 * The embedding model to use.
 * text-embedding-004 produces 768-dimensional vectors.
 */
const EMBEDDING_MODEL = 'text-embedding-004';

/**
 * Expected embedding dimensions from text-embedding-004.
 */
const EMBEDDING_DIMENSIONS = 768;

/**
 * Result of a successful embedding operation for a single chunk.
 */
export interface EmbeddingResult {
  chunkId: number;
  embedding: number[]; // 768 dimensions
}

/**
 * Error result for a chunk that failed to embed.
 */
export interface EmbeddingError {
  chunkId: number;
  error: string;
}

/**
 * Result of a batch embedding operation.
 */
export interface BatchEmbeddingResult {
  results: EmbeddingResult[];
  errors: EmbeddingError[];
}

/**
 * Embed a batch of chunks using the Gemini API.
 *
 * Uses batchEmbedContents for efficiency. The caller is responsible
 * for batching chunks (max 20 per call).
 *
 * @param apiKey - The Gemini API key
 * @param chunks - Array of chunks to embed (max 20)
 * @returns BatchEmbeddingResult with successful embeddings and errors
 */
export async function embedChunks(
  apiKey: string,
  chunks: Array<{ id: number; content: string }>
): Promise<BatchEmbeddingResult> {
  const results: EmbeddingResult[] = [];
  const errors: EmbeddingError[] = [];

  if (chunks.length === 0) {
    return { results, errors };
  }

  try {
    const genAI = new GoogleGenerativeAI(apiKey);
    const model = genAI.getGenerativeModel({ model: EMBEDDING_MODEL });

    // Build batch request
    const requests = chunks.map((chunk) => ({
      content: {
        parts: [{ text: chunk.content }],
        role: 'user' as const,
      },
      taskType: TaskType.RETRIEVAL_DOCUMENT,
    }));

    const response = await model.batchEmbedContents({ requests });

    // Process each embedding result
    for (let i = 0; i < chunks.length; i++) {
      const chunk = chunks[i];
      const embeddingData = response.embeddings[i];

      if (!embeddingData || !embeddingData.values) {
        errors.push({
          chunkId: chunk.id,
          error: 'No embedding returned from API',
        });
        continue;
      }

      const embedding = embeddingData.values;

      // Validate embedding dimensions
      if (embedding.length !== EMBEDDING_DIMENSIONS) {
        errors.push({
          chunkId: chunk.id,
          error: `Invalid embedding dimensions: expected ${EMBEDDING_DIMENSIONS}, got ${embedding.length}`,
        });
        continue;
      }

      results.push({
        chunkId: chunk.id,
        embedding,
      });
    }

    return { results, errors };
  } catch (error) {
    // If the entire batch request fails, return all chunks as errors
    const errorMessage = error instanceof Error ? error.message : 'Unknown error';
    for (const chunk of chunks) {
      errors.push({
        chunkId: chunk.id,
        error: errorMessage,
      });
    }
    return { results, errors };
  }
}

/**
 * Embed a single query string for retrieval.
 *
 * Uses taskType: RETRIEVAL_QUERY for optimal query embedding.
 *
 * @param apiKey - The Gemini API key
 * @param query - The query string to embed
 * @returns The 768-dimension vector or null on error
 */
export async function embedQuery(apiKey: string, query: string): Promise<number[] | null> {
  try {
    const genAI = new GoogleGenerativeAI(apiKey);
    const model = genAI.getGenerativeModel({ model: EMBEDDING_MODEL });

    const result = await model.embedContent({
      content: {
        parts: [{ text: query }],
        role: 'user',
      },
      taskType: TaskType.RETRIEVAL_QUERY,
    });

    const embedding = result.embedding?.values;

    if (!embedding) {
      return null;
    }

    // Validate embedding dimensions
    if (embedding.length !== EMBEDDING_DIMENSIONS) {
      return null;
    }

    return embedding;
  } catch {
    return null;
  }
}

/**
 * Get the embedding model name.
 *
 * @returns The Gemini embedding model name
 */
export function getEmbeddingModel(): string {
  return EMBEDDING_MODEL;
}

/**
 * Get the expected embedding dimensions.
 *
 * @returns The number of dimensions in embeddings
 */
export function getEmbeddingDimensions(): number {
  return EMBEDDING_DIMENSIONS;
}
