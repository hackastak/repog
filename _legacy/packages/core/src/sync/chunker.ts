/**
 * Simple text chunking utility for RepoG.
 * Splits large text into smaller chunks for embedding.
 */

export interface ChunkOptions {
  /** Maximum number of characters per chunk (default: 7000) */
  maxChunkSize?: number;
  /** Minimum number of characters to keep as a chunk (default: 100) */
  minChunkSize?: number;
}

const DEFAULT_MAX_CHUNK_SIZE = 7000; // Safely below Gemini's ~8k limit
const DEFAULT_MIN_CHUNK_SIZE = 100;

/**
 * Split text into chunks of roughly equal size.
 *
 * Tries to split at double newlines, then single newlines, then spaces,
 * and finally just cuts at the maximum size.
 *
 * @param text - The text to split
 * @param options - Chunking options
 * @returns Array of text chunks
 */
export function splitIntoChunks(text: string, options: ChunkOptions = {}): string[] {
  const maxSize = options.maxChunkSize ?? DEFAULT_MAX_CHUNK_SIZE;
  const minSize = options.minChunkSize ?? DEFAULT_MIN_CHUNK_SIZE;

  if (text.length <= maxSize) {
    return [text];
  }

  const chunks: string[] = [];
  let remaining = text;

  while (remaining.length > 0) {
    if (remaining.length <= maxSize) {
      chunks.push(remaining);
      break;
    }

    // Try to find a good split point in the first 'maxSize' characters
    let splitPoint = -1;
    const searchArea = remaining.slice(0, maxSize);

    // 1. Try double newline (paragraph)
    splitPoint = searchArea.lastIndexOf('\n\n');

    // 2. Try single newline
    if (splitPoint < minSize) {
      splitPoint = searchArea.lastIndexOf('\n');
    }

    // 3. Try space
    if (splitPoint < minSize) {
      splitPoint = searchArea.lastIndexOf(' ');
    }

    // 4. Force split at maxSize
    if (splitPoint < minSize) {
      splitPoint = maxSize;
    }

    chunks.push(remaining.slice(0, splitPoint).trim());
    remaining = remaining.slice(splitPoint).trim();
  }

  return chunks.filter((c) => c.length > 0);
}
