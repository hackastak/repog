/**
 * Q&A module for answering natural language questions about repositories.
 * Uses RAG: vector search retrieves relevant chunks, then Gemini answers.
 */

import { loadConfigAsync, isConfigured } from '../config/config.js';
import { searchRepos, type SearchResult } from '../search/query.js';
import { streamLLM, isLLMError } from '../gemini/llm.js';

/**
 * Options for asking a question.
 */
export interface AskOptions {
  /** The natural language question to answer */
  question: string;
  /** Optional repo full name (owner/repo) to scope the search */
  repo?: string;
  /** Number of chunks to retrieve as context, default 10 */
  limit?: number;
}

/**
 * Attribution for a source used in the answer.
 */
export interface SourceAttribution {
  /** Repository full name (owner/repo) */
  repoFullName: string;
  /** Type of chunk: metadata, readme, or file_tree */
  chunkType: 'metadata' | 'readme' | 'file_tree';
  /** Cosine similarity score, 0-1 */
  similarity: number;
}

/**
 * Result of a Q&A query.
 */
export interface AskResult {
  /** The full assembled answer text */
  answer: string;
  /** Deduplicated list of sources used as context */
  sources: SourceAttribution[];
  /** The original question */
  question: string;
  /** Number of input tokens used */
  inputTokens: number;
  /** Number of output tokens generated */
  outputTokens: number;
  /** Duration of the call in milliseconds */
  durationMs: number;
}

/**
 * Callback function invoked for each streamed text chunk.
 */
export type OnChunkCallback = (text: string) => void;

/**
 * System prompt for the Q&A assistant.
 */
const SYSTEM_PROMPT = `You are a helpful assistant that answers questions about GitHub repositories.
Answer based only on the provided context. If the context does not contain enough
information to answer the question, say so clearly. Be concise and precise.`;

/**
 * Build the user prompt for the Q&A query.
 *
 * @param question - The user's question
 * @param chunks - Search results to include as context
 * @returns The formatted user prompt
 */
export function buildAskPrompt(question: string, chunks: SearchResult[]): string {
  // Format each chunk
  const formattedChunks = chunks.map((chunk) => {
    return `--- ${chunk.repoFullName} (${chunk.chunkType}) ---\n${chunk.content}`;
  });

  // Join chunks with blank lines
  const context = formattedChunks.join('\n\n');

  return `Question: ${question}

Context from repositories:
${context}

Answer the question based on the context above. If you reference specific
repositories, mention them by name.`;
}

/**
 * Create an empty/error AskResult.
 *
 * @param question - The original question
 * @param answer - The answer text (usually an error message)
 * @param durationMs - Duration of the operation
 * @returns A safe AskResult
 */
function safeResult(question: string, answer: string, durationMs: number = 0): AskResult {
  return {
    answer,
    sources: [],
    question,
    inputTokens: 0,
    outputTokens: 0,
    durationMs,
  };
}

/**
 * Build source attributions from search results.
 * Deduplicates by repoFullName, sorts by similarity descending, caps at 5.
 *
 * @param results - Search results to attribute
 * @returns Deduplicated, sorted, capped source attributions
 */
function buildSourceAttributions(results: SearchResult[]): SourceAttribution[] {
  // Deduplicate by repoFullName, keeping highest similarity
  const seen = new Map<string, SourceAttribution>();

  for (const result of results) {
    const existing = seen.get(result.repoFullName);
    if (!existing || result.similarity > existing.similarity) {
      seen.set(result.repoFullName, {
        repoFullName: result.repoFullName,
        chunkType: result.chunkType,
        similarity: result.similarity,
      });
    }
  }

  // Convert to array, sort by similarity descending, cap at 5
  return Array.from(seen.values())
    .sort((a, b) => b.similarity - a.similarity)
    .slice(0, 5);
}

/**
 * Ask a question about repositories using RAG.
 *
 * This function:
 * 1. Validates configuration
 * 2. Retrieves relevant chunks via vector search
 * 3. Builds a Q&A prompt with context
 * 4. Streams the LLM response
 * 5. Returns a fully populated AskResult
 *
 * @param options - The question and optional filters
 * @param onChunk - Optional callback for streaming tokens
 * @returns AskResult with answer and sources - never throws
 */
export async function askQuestion(
  options: AskOptions,
  onChunk?: OnChunkCallback
): Promise<AskResult> {
  const startTime = performance.now();

  try {
    // Validate configuration
    if (!isConfigured()) {
      return safeResult(
        options.question,
        'RepoG is not configured. Run `repog init` first.',
        performance.now() - startTime
      );
    }

    const config = await loadConfigAsync();
    if (!config.geminiKey) {
      return safeResult(
        options.question,
        'Gemini API key is not configured.',
        performance.now() - startTime
      );
    }

    // Search for relevant chunks
    const limit = options.limit ?? 10;
    const searchResult = await searchRepos(options.question, { limit });

    // Filter by repo if specified
    let results = searchResult.results;
    if (options.repo) {
      results = results.filter((r) => r.repoFullName === options.repo);
    }

    // Handle empty results
    if (results.length === 0) {
      const answer =
        "I couldn't find any relevant information in your knowledge base to answer this question.";
      if (onChunk) {
        onChunk(answer);
      }
      return safeResult(options.question, answer, performance.now() - startTime);
    }

    // Build the prompt
    const prompt = buildAskPrompt(options.question, results);

    // Stream the LLM response
    const llmResult = await streamLLM(config.geminiKey, prompt, SYSTEM_PROMPT, onChunk);

    const durationMs = performance.now() - startTime;

    // Handle LLM error
    if (isLLMError(llmResult)) {
      return {
        answer: `Error generating answer: ${llmResult.error}`,
        sources: [],
        question: options.question,
        inputTokens: 0,
        outputTokens: 0,
        durationMs,
      };
    }

    // Build source attributions
    const sources = buildSourceAttributions(results);

    return {
      answer: llmResult.text,
      sources,
      question: options.question,
      inputTokens: llmResult.inputTokens,
      outputTokens: llmResult.outputTokens,
      durationMs,
    };
  } catch (error) {
    const durationMs = performance.now() - startTime;
    const errorMessage = error instanceof Error ? error.message : 'Unknown error';
    return safeResult(options.question, `Error: ${errorMessage}`, durationMs);
  }
}
