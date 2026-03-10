import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import nock from 'nock';
import { validateGeminiKey, getValidationModel } from './auth.js';

const GEMINI_API_HOST = 'https://generativelanguage.googleapis.com';

describe('gemini/auth', () => {
  beforeEach(() => {
    nock.cleanAll();
  });

  afterEach(() => {
    nock.cleanAll();
  });

  describe('validateGeminiKey', () => {
    it('returns invalid for empty API key', async () => {
      const result = await validateGeminiKey('');

      expect(result.valid).toBe(false);
      expect(result.error).toBe('API key is empty');
    });

    it('returns invalid for whitespace-only API key', async () => {
      const result = await validateGeminiKey('   ');

      expect(result.valid).toBe(false);
      expect(result.error).toBe('API key is empty');
    });

    it('returns valid for working API key', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*countTokens.*/)
        .reply(200, { totalTokens: 1 });

      const result = await validateGeminiKey('valid-api-key');

      expect(result.valid).toBe(true);
      expect(result.model).toBe('gemini-1.5-flash');
      expect(result.error).toBeUndefined();
    });

    it('returns invalid for 401 authentication error', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*countTokens.*/)
        .reply(401, { error: { message: 'Invalid API key' } });

      const result = await validateGeminiKey('invalid-key');

      expect(result.valid).toBe(false);
      expect(result.error).toBe('Invalid API key');
    });

    it('returns valid but with error message for rate limit (429)', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*countTokens.*/)
        .reply(429, { error: { message: 'Rate limit exceeded' } });

      const result = await validateGeminiKey('rate-limited-key');

      expect(result.valid).toBe(true);
      expect(result.model).toBe('gemini-1.5-flash');
      expect(result.error).toContain('rate limited');
    });

    it('returns valid but with error message for quota exceeded', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*countTokens.*/)
        .reply(429, { error: { message: 'Quota exceeded for the day' } });

      const result = await validateGeminiKey('quota-exceeded-key');

      expect(result.valid).toBe(true);
      expect(result.error).toContain('quota exceeded');
    });

    it('returns invalid for 403 permission error', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*countTokens.*/)
        .reply(403, { error: { message: 'Permission denied' } });

      const result = await validateGeminiKey('no-permission-key');

      expect(result.valid).toBe(false);
      expect(result.error).toBe('API key lacks required permissions');
    });

    it('returns invalid with error message for other errors', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*countTokens.*/)
        .reply(500, { error: { message: 'Internal server error' } });

      const result = await validateGeminiKey('some-key');

      expect(result.valid).toBe(false);
      expect(result.error).toContain('Failed to validate API key');
    });

    it('returns invalid for network errors', async () => {
      nock(GEMINI_API_HOST)
        .post(/.*countTokens.*/)
        .replyWithError('Network connection failed');

      const result = await validateGeminiKey('some-key');

      expect(result.valid).toBe(false);
      expect(result.error).toContain('Failed to validate API key');
    });
  });

  describe('getValidationModel', () => {
    it('returns the validation model name', () => {
      const model = getValidationModel();

      expect(model).toBe('gemini-1.5-flash');
    });
  });
});
