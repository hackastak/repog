import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import nock from 'nock';
import { callLLM, streamLLM, isLLMError, type LLMResult, type LLMError } from './llm.js';

const TEST_API_KEY = 'test-api-key-12345';
const GEMINI_API_HOST = 'https://generativelanguage.googleapis.com';

describe('gemini/llm', () => {
  beforeEach(() => {
    nock.cleanAll();
  });

  afterEach(() => {
    nock.cleanAll();
  });

  describe('callLLM', () => {
    it('returns a valid LLMResult with text, inputTokens, outputTokens, and durationMs on success', async () => {
      // Mock the Gemini API response
      nock(GEMINI_API_HOST)
        .post(/.*/)
        .reply(200, {
          candidates: [
            {
              content: {
                parts: [{ text: 'Hello, world!' }],
                role: 'model',
              },
              finishReason: 'STOP',
            },
          ],
          usageMetadata: {
            promptTokenCount: 10,
            candidatesTokenCount: 5,
            totalTokenCount: 15,
          },
        });

      const result = await callLLM(TEST_API_KEY, 'Say hello');

      expect(isLLMError(result)).toBe(false);
      const llmResult = result as LLMResult;
      expect(llmResult.text).toBe('Hello, world!');
      expect(llmResult.inputTokens).toBe(10);
      expect(llmResult.outputTokens).toBe(5);
      expect(llmResult.durationMs).toBeGreaterThanOrEqual(0);
    });

    it('returns LLMError on API failure - does not throw', async () => {
      // Mock an API error
      nock(GEMINI_API_HOST)
        .post(/.*/)
        .reply(500, { error: { message: 'Internal server error' } });

      const result = await callLLM(TEST_API_KEY, 'Say hello');

      expect(isLLMError(result)).toBe(true);
      const llmError = result as LLMError;
      expect(llmError.error).toBeDefined();
      expect(typeof llmError.error).toBe('string');
      expect(llmError.error.length).toBeGreaterThan(0);
    });

    it('durationMs is a non-negative number on success', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*/)
        .reply(200, {
          candidates: [
            {
              content: {
                parts: [{ text: 'Response text' }],
                role: 'model',
              },
              finishReason: 'STOP',
            },
          ],
          usageMetadata: {
            promptTokenCount: 5,
            candidatesTokenCount: 3,
            totalTokenCount: 8,
          },
        });

      const result = await callLLM(TEST_API_KEY, 'Test prompt');

      expect(isLLMError(result)).toBe(false);
      const llmResult = result as LLMResult;
      expect(typeof llmResult.durationMs).toBe('number');
      expect(llmResult.durationMs).toBeGreaterThanOrEqual(0);
    });

    it('durationMs is a non-negative number on error', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*/)
        .reply(400, { error: { message: 'Bad request' } });

      const result = await callLLM(TEST_API_KEY, 'Test prompt');

      expect(isLLMError(result)).toBe(true);
      const llmError = result as LLMError;
      expect(typeof llmError.durationMs).toBe('number');
      expect(llmError.durationMs).toBeGreaterThanOrEqual(0);
    });

    it('handles system prompt correctly', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*/)
        .reply(200, {
          candidates: [
            {
              content: {
                parts: [{ text: 'System prompt response' }],
                role: 'model',
              },
              finishReason: 'STOP',
            },
          ],
          usageMetadata: {
            promptTokenCount: 15,
            candidatesTokenCount: 4,
            totalTokenCount: 19,
          },
        });

      const result = await callLLM(
        TEST_API_KEY,
        'User prompt',
        'You are a helpful assistant.'
      );

      expect(isLLMError(result)).toBe(false);
      const llmResult = result as LLMResult;
      expect(llmResult.text).toBe('System prompt response');
    });
  });

  describe('isLLMError', () => {
    it('returns true for an LLMError object', () => {
      const error: LLMError = {
        error: 'Something went wrong',
        durationMs: 100,
      };

      expect(isLLMError(error)).toBe(true);
    });

    it('returns false for a valid LLMResult', () => {
      const result: LLMResult = {
        text: 'Hello',
        inputTokens: 10,
        outputTokens: 5,
        durationMs: 200,
      };

      expect(isLLMError(result)).toBe(false);
    });
  });

  describe('streamLLM', () => {
    it('assembles streamed chunks into a complete LLMResult', async () => {
      // Mock streaming response with SSE format (newline-delimited JSON)
      const streamResponse = [
        'data: {"candidates":[{"content":{"parts":[{"text":"Hello, "}],"role":"model"}}]}\n\n',
        'data: {"candidates":[{"content":{"parts":[{"text":"world!"}],"role":"model"}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}\n\n',
      ].join('');

      nock(GEMINI_API_HOST)
        .post(/.*streamGenerateContent.*/)
        .reply(200, streamResponse, {
          'Content-Type': 'text/event-stream',
        });

      const result = await streamLLM(TEST_API_KEY, 'Say hello');

      expect(isLLMError(result)).toBe(false);
      const llmResult = result as LLMResult;
      expect(llmResult.text).toBe('Hello, world!');
      expect(llmResult.inputTokens).toBe(10);
      expect(llmResult.outputTokens).toBe(5);
      expect(llmResult.durationMs).toBeGreaterThanOrEqual(0);
    });

    it('calls onChunk for each streamed text chunk', async () => {
      const streamResponse = [
        'data: {"candidates":[{"content":{"parts":[{"text":"First "}],"role":"model"}}]}\n\n',
        'data: {"candidates":[{"content":{"parts":[{"text":"Second"}],"role":"model"}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3,"totalTokenCount":8}}\n\n',
      ].join('');

      nock(GEMINI_API_HOST)
        .post(/.*streamGenerateContent.*/)
        .reply(200, streamResponse, {
          'Content-Type': 'text/event-stream',
        });

      const chunks: string[] = [];
      const onChunk = (text: string): void => {
        chunks.push(text);
      };

      await streamLLM(TEST_API_KEY, 'Test prompt', undefined, onChunk);

      expect(chunks).toHaveLength(2);
      expect(chunks[0]).toBe('First ');
      expect(chunks[1]).toBe('Second');
    });

    it('returns LLMError on API failure - does not throw', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*streamGenerateContent.*/)
        .reply(500, { error: { message: 'Internal server error' } });

      const result = await streamLLM(TEST_API_KEY, 'Say hello');

      expect(isLLMError(result)).toBe(true);
      const llmError = result as LLMError;
      expect(llmError.error).toBeDefined();
      expect(typeof llmError.error).toBe('string');
      expect(llmError.error.length).toBeGreaterThan(0);
      expect(llmError.durationMs).toBeGreaterThanOrEqual(0);
    });

    it('returns a complete result even if onChunk is not provided', async () => {
      const streamResponse = [
        'data: {"candidates":[{"content":{"parts":[{"text":"Silent "}],"role":"model"}}]}\n\n',
        'data: {"candidates":[{"content":{"parts":[{"text":"response"}],"role":"model"}}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":4,"totalTokenCount":12}}\n\n',
      ].join('');

      nock(GEMINI_API_HOST)
        .post(/.*streamGenerateContent.*/)
        .reply(200, streamResponse, {
          'Content-Type': 'text/event-stream',
        });

      // Call without onChunk callback
      const result = await streamLLM(TEST_API_KEY, 'Test prompt');

      expect(isLLMError(result)).toBe(false);
      const llmResult = result as LLMResult;
      expect(llmResult.text).toBe('Silent response');
      expect(llmResult.inputTokens).toBe(8);
      expect(llmResult.outputTokens).toBe(4);
    });
  });
});
