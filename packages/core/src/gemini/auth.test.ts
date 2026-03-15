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

    it('returns valid for working API key with primary model', async () => {
      // Mock for gemini-2.5-flash
      nock(GEMINI_API_HOST)
        .post('/v1beta/models/gemini-2.5-flash:countTokens')
        .reply(200, { totalTokens: 1 });

      const result = await validateGeminiKey('valid-api-key');

      expect(result.valid).toBe(true);
      expect(result.model).toBe('gemini-2.5-flash');
      expect(result.error).toBeUndefined();
    });

    it('falls back to gemini-2.0-flash if gemini-2.5-flash is 404', async () => {
      // Mock for gemini-2.5-flash failing with 404
      nock(GEMINI_API_HOST)
        .post('/v1beta/models/gemini-2.5-flash:countTokens')
        .reply(404, { error: { message: 'Model not found' } });

      // Mock for gemini-2.0-flash succeeding
      nock(GEMINI_API_HOST)
        .post('/v1beta/models/gemini-2.0-flash:countTokens')
        .reply(200, { totalTokens: 1 });

      const result = await validateGeminiKey('valid-fallback-key');

      expect(result.valid).toBe(true);
      expect(result.model).toBe('gemini-2.0-flash');
      expect(result.error).toBeUndefined();
    });

     it('falls back to gemini-3.1-flash if others are 404', async () => {
      // Mock for gemini-2.5-flash failing with 404
      nock(GEMINI_API_HOST)
        .post('/v1beta/models/gemini-2.5-flash:countTokens')
        .reply(404, { error: { message: 'Model not found' } });

      // Mock for gemini-2.0-flash failing with 404
      nock(GEMINI_API_HOST)
        .post('/v1beta/models/gemini-2.0-flash:countTokens')
        .reply(404, { error: { message: 'Model not found' } });
        
      // Mock for gemini-3.1-flash succeeding
      nock(GEMINI_API_HOST)
        .post('/v1beta/models/gemini-3.1-flash:countTokens')
        .reply(200, { totalTokens: 1 });

      const result = await validateGeminiKey('valid-fallback-key-31');

      expect(result.valid).toBe(true);
      expect(result.model).toBe('gemini-3.1-flash');
      expect(result.error).toBeUndefined();
    });

    it('returns invalid if all models fail with 404', async () => {
        // Mock all models failing
        nock(GEMINI_API_HOST)
        .post(/.*/)
        .times(3) // 2.5, 2.0, 3.1
        .reply(404, { error: { message: 'Model not found' } });

        const result = await validateGeminiKey('no-models-key');

        expect(result.valid).toBe(false);
        expect(result.error).toContain('Failed to validate API key');
    });

    it('returns invalid for 401 authentication error immediately', async () => {
      // Even if 2.5 fails, a 401 should stop the loop
      nock(GEMINI_API_HOST)
        .post('/v1beta/models/gemini-2.5-flash:countTokens')
        .reply(401, { error: { message: 'Invalid API key' } });

      const result = await validateGeminiKey('invalid-key');

      expect(result.valid).toBe(false);
      expect(result.error).toBe('Invalid API key');
    });

    it('returns valid but with error message for rate limit (429)', async () => {
      nock(GEMINI_API_HOST)
        .post('/v1beta/models/gemini-2.5-flash:countTokens')
        .reply(429, { error: { message: 'Rate limit exceeded' } });

      const result = await validateGeminiKey('rate-limited-key');

      expect(result.valid).toBe(true);
      expect(result.model).toBe('gemini-2.5-flash');
      expect(result.error).toContain('rate limited');
    });

    it('returns invalid for 403 permission error', async () => {
      nock(GEMINI_API_HOST)
        .post('/v1beta/models/gemini-2.5-flash:countTokens')
        .reply(403, { error: { message: 'Permission denied' } });

      const result = await validateGeminiKey('no-permission-key');

      expect(result.valid).toBe(false);
      expect(result.error).toBe('API key lacks required permissions');
    });

    it('returns invalid for network errors', async () => {
      nock(GEMINI_API_HOST)
        .post('/v1beta/models/gemini-2.5-flash:countTokens')
        .replyWithError('Network connection failed');

      const result = await validateGeminiKey('some-key');

      expect(result.valid).toBe(false);
      expect(result.error).toContain('Failed to validate API key');
    });
  });

  describe('getValidationModel', () => {
    it('returns the validation model name', () => {
      const model = getValidationModel();

      expect(model).toBe('gemini-2.5-flash');
    });
  });
});
