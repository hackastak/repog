import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { getRateLimitInfo } from './client.js';

// Mock the config module
vi.mock('../config/config.js', () => ({
  loadConfig: vi.fn(),
  loadConfigAsync: vi.fn(),
}));

// Mock Octokit
vi.mock('octokit', () => ({
  Octokit: vi.fn(),
}));

import { loadConfig, loadConfigAsync } from '../config/config.js';
import { Octokit } from 'octokit';

describe('github/client', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('getRateLimitInfo', () => {
    it('returns null when no GitHub PAT is configured', async () => {
      vi.mocked(loadConfigAsync).mockResolvedValue({
        githubPat: null,
        geminiKey: null,
        dbPath: '/tmp/test.db',
      });

      const result = await getRateLimitInfo();

      expect(result).toBeNull();
    });

    it('returns rate limit stats when API call succeeds', async () => {
      const mockResetTime = Math.floor(Date.now() / 1000) + 3600; // 1 hour from now

      vi.mocked(loadConfigAsync).mockResolvedValue({
        githubPat: 'ghp_testtoken',
        geminiKey: null,
        dbPath: '/tmp/test.db',
      });

      const mockRateLimitGet = vi.fn().mockResolvedValue({
        data: {
          resources: {
            core: {
              limit: 5000,
              remaining: 4999,
              reset: mockResetTime,
            },
          },
        },
      });

      vi.mocked(Octokit).mockImplementation(() => ({
        rest: {
          rateLimit: {
            get: mockRateLimitGet,
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await getRateLimitInfo();

      expect(result).not.toBeNull();
      expect(result!.limit).toBe(5000);
      expect(result!.remaining).toBe(4999);
      expect(result!.available).toBe(true);
      expect(result!.resetAt).toBe(new Date(mockResetTime * 1000).toISOString());
    });

    it('returns available=false when remaining is 0', async () => {
      const mockResetTime = Math.floor(Date.now() / 1000) + 3600;

      vi.mocked(loadConfigAsync).mockResolvedValue({
        githubPat: 'ghp_testtoken',
        geminiKey: null,
        dbPath: '/tmp/test.db',
      });

      const mockRateLimitGet = vi.fn().mockResolvedValue({
        data: {
          resources: {
            core: {
              limit: 5000,
              remaining: 0,
              reset: mockResetTime,
            },
          },
        },
      });

      vi.mocked(Octokit).mockImplementation(() => ({
        rest: {
          rateLimit: {
            get: mockRateLimitGet,
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await getRateLimitInfo();

      expect(result).not.toBeNull();
      expect(result!.remaining).toBe(0);
      expect(result!.available).toBe(false);
    });

    it('returns null on API error - does not throw', async () => {
      vi.mocked(loadConfigAsync).mockResolvedValue({
        githubPat: 'ghp_testtoken',
        geminiKey: null,
        dbPath: '/tmp/test.db',
      });

      const mockRateLimitGet = vi.fn().mockRejectedValue(new Error('Network error'));

      vi.mocked(Octokit).mockImplementation(() => ({
        rest: {
          rateLimit: {
            get: mockRateLimitGet,
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await getRateLimitInfo();

      expect(result).toBeNull();
    });

    it('returns null on authentication error - does not throw', async () => {
      vi.mocked(loadConfigAsync).mockResolvedValue({
        githubPat: 'ghp_invalidtoken',
        geminiKey: null,
        dbPath: '/tmp/test.db',
      });

      const mockRateLimitGet = vi.fn().mockRejectedValue(new Error('Bad credentials'));

      vi.mocked(Octokit).mockImplementation(() => ({
        rest: {
          rateLimit: {
            get: mockRateLimitGet,
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await getRateLimitInfo();

      expect(result).toBeNull();
    });
  });
});
