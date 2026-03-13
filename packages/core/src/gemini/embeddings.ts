import { GoogleGenerativeAI, TaskType } from '@google/generative-ai';

/**
 * The embedding model to use.
 * gemini-embedding-2-preview is a multimodal model that supports flexible dimensions.
 */
const EMBEDDING_MODEL = 'gemini-embedding-2-preview';

/**
 * Expected embedding dimensions.
 * We stick to 768 as it's efficient and matches our database schema.
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
      outputDimensionality: EMBEDDING_DIMENSIONS,
    } as any));

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
    // If the entire batch request fails (e.g. batch poisoning or quota),
    // we attempt each chunk individually to isolate the failure
    for (const chunk of chunks) {
      try {
        const genAI = new GoogleGenerativeAI(apiKey);
        const model = genAI.getGenerativeModel({ model: EMBEDDING_MODEL });

        const result = await model.embedContent({
          content: { parts: [{ text: chunk.content }], role: 'user' },
          taskType: TaskType.RETRIEVAL_DOCUMENT,
          outputDimensionality: EMBEDDING_DIMENSIONS,
        } as any);

        const embedding = result.embedding?.values;
        if (embedding && embedding.length === EMBEDDING_DIMENSIONS) {
          results.push({ chunkId: chunk.id, embedding });
        } else {
          errors.push({
            chunkId: chunk.id,
            error: !embedding
              ? 'No embedding returned'
              : `Invalid dimensions: ${embedding.length}`,
          });
        }
      } catch (innerError) {
        errors.push({
          chunkId: chunk.id,
          error: innerError instanceof Error ? innerError.message : 'Unknown error',
        });
      }
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
      outputDimensionality: EMBEDDING_DIMENSIONS,
    } as any);

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
