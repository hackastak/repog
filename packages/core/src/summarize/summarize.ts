import { getDb } from '../db/index.js';
import { loadConfig } from '../config/config.js';
import { streamLLM, isLLMError } from '../gemini/llm.js';

/**
 * Options for summarizing a repository.
 */
export interface SummarizeOptions {
  repo: string; // full_name e.g. "owner/repo" — required
}

/**
 * Result of a summarization operation.
 */
export interface SummarizeResult {
  summary: string;       // full assembled summary text
  repo: string;          // full_name of the summarized repo
  chunksUsed: number;    // how many chunks were used as context
  inputTokens: number;
  outputTokens: number;
  durationMs: number;
}

/**
 * System prompt for the summarization assistant.
 */
const SYSTEM_PROMPT = `You are a technical documentation assistant. Your job is to produce a clear,
structured summary of a GitHub repository based on its metadata, README, and
file tree. Always respond in exactly three sections with these exact headings:

## Overview
## Tech Stack
## Use Cases

Be concise. Use plain prose — no bullet points. Each section should be 2-4 sentences.`;

/**
 * Build the summarization prompt.
 *
 * @param repo - The repository name
 * @param chunks - The content chunks to include as context
 * @returns The formatted prompt
 */
export function buildSummarizePrompt(
  repo: string,
  chunks: Array<{ chunk_type: string; content: string }>
): string {
  const context = chunks
    .map((chunk) => `--- ${chunk.chunk_type} ---\n${chunk.content}`)
    .join('\n\n');

  return `Please summarize the following GitHub repository.

Repository: ${repo}

Context:
${context}`;
}

/**
 * Generate a structured AI summary of a repository.
 *
 * @param options - The summarization options
 * @param onChunk - Optional callback for streaming tokens
 * @returns SummarizeResult with the summary and stats - never throws
 */
export async function summarizeRepo(
  options: SummarizeOptions,
  onChunk?: (text: string) => void
): Promise<SummarizeResult> {
  const startTime = performance.now();
  const repoName = options.repo;

  try {
    const config = loadConfig();

    if (!config.geminiKey) {
      const summary = 'Error: Gemini API key is not configured. Run `repog init` first.';
      if (onChunk) {
        onChunk(summary);
      }
      return {
        summary,
        repo: repoName,
        chunksUsed: 0,
        inputTokens: 0,
        outputTokens: 0,
        durationMs: performance.now() - startTime,
      };
    }

    const db = getDb(config.dbPath);

    // Query chunks
    const chunks = db
      .prepare(
        `
      SELECT c.chunk_type, c.content
      FROM chunks c
      JOIN repos r ON r.id = c.repo_id
      WHERE LOWER(r.full_name) = LOWER(?)
      ORDER BY c.chunk_type ASC
    `
      )
      .all(repoName) as Array<{ chunk_type: string; content: string }>;

    // Handle no chunks
    if (chunks.length === 0) {
      const summary =
        'No data found for this repository. Try running `repog sync` and `repog embed` first.';
      if (onChunk) {
        onChunk(summary);
      }
      return {
        summary,
        repo: repoName,
        chunksUsed: 0,
        inputTokens: 0,
        outputTokens: 0,
        durationMs: performance.now() - startTime,
      };
    }

    // Build prompt
    const prompt = buildSummarizePrompt(repoName, chunks);

    // Call LLM
    const result = await streamLLM(config.geminiKey, prompt, SYSTEM_PROMPT, onChunk);

    // Handle LLM Error
    if (isLLMError(result)) {
      return {
        summary: `Error generating summary: ${result.error}`,
        repo: repoName,
        chunksUsed: chunks.length,
        inputTokens: 0,
        outputTokens: 0,
        durationMs: performance.now() - startTime,
      };
    }

    // Return success
    return {
      summary: result.text,
      repo: repoName,
      chunksUsed: chunks.length,
      inputTokens: result.inputTokens,
      outputTokens: result.outputTokens,
      durationMs: performance.now() - startTime,
    };
  } catch (error) {
    const errorMessage = error instanceof Error ? error.message : 'Unknown error';
    return {
      summary: `Error: ${errorMessage}`,
      repo: repoName,
      chunksUsed: 0,
      inputTokens: 0,
      outputTokens: 0,
      durationMs: performance.now() - startTime,
    };
  }
}
