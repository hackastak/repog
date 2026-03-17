import { describe, it, expect, beforeEach, vi } from 'vitest';
import { RateLimiter, githubRateLimiter } from './rateLimiter.js';

describe('RateLimiter', () => {
  let limiter: RateLimiter;

  beforeEach(() => {
    // Create a new limiter for each test with a small limit for testing
    limiter = new RateLimiter(5);
  });

  describe('throttle', () => {
    it('resolves immediately when under the rate limit', async () => {
      const start = Date.now();
      await limiter.throttle();
      const elapsed = Date.now() - start;

      // Should resolve almost immediately (< 50ms)
      expect(elapsed).toBeLessThan(50);
    });

    it('identifies when rate limit is reached and would need to wait', async () => {
      // Use a tiny limit for testing
      const testLimiter = new RateLimiter(2);

      // Record 2 requests to hit the limit
      testLimiter.recordRequest();
      testLimiter.recordRequest();

      // Verify the limiter correctly identifies we're at the limit
      expect(testLimiter.getRemainingEstimate()).toBe(0);
      expect(testLimiter.getRequestCount()).toBe(2);

      // After time passes (simulated by manipulating timestamps),
      // old requests should be pruned and requests can proceed
      // This is tested in getRemainingEstimate tests below
    });

    it('resolves after recording requests within limit', async () => {
      // Record some requests but stay under limit
      limiter.recordRequest();
      limiter.recordRequest();
      limiter.recordRequest();

      // Should still resolve immediately since we're at 3/5
      const start = Date.now();
      await limiter.throttle();
      const elapsed = Date.now() - start;

      expect(elapsed).toBeLessThan(50);
    });
  });

  describe('recordRequest', () => {
    it('adds a timestamp to the request history', () => {
      expect(limiter.getRequestCount()).toBe(0);

      limiter.recordRequest();
      expect(limiter.getRequestCount()).toBe(1);

      limiter.recordRequest();
      expect(limiter.getRequestCount()).toBe(2);
    });
  });

  describe('getRemainingEstimate', () => {
    it('returns full limit when no requests have been made', () => {
      expect(limiter.getRemainingEstimate()).toBe(5);
    });

    it('decreases as requests are recorded', () => {
      limiter.recordRequest();
      expect(limiter.getRemainingEstimate()).toBe(4);

      limiter.recordRequest();
      expect(limiter.getRemainingEstimate()).toBe(3);

      limiter.recordRequest();
      limiter.recordRequest();
      limiter.recordRequest();
      expect(limiter.getRemainingEstimate()).toBe(0);
    });

    it('never returns negative values', () => {
      // Record more than the limit
      for (let i = 0; i < 10; i++) {
        limiter.recordRequest();
      }

      expect(limiter.getRemainingEstimate()).toBe(0);
    });

    it('prunes old timestamps outside the rolling window', () => {
      // Record a request
      limiter.recordRequest();
      expect(limiter.getRemainingEstimate()).toBe(4);

      // Mock time to be past the window
      const originalDateNow = Date.now;
      vi.spyOn(Date, 'now').mockImplementation(() => originalDateNow() + 60 * 60 * 1000 + 1000);

      // Old request should be pruned
      expect(limiter.getRemainingEstimate()).toBe(5);

      vi.restoreAllMocks();
    });
  });

  describe('getRequestCount', () => {
    it('returns 0 when no requests have been made', () => {
      expect(limiter.getRequestCount()).toBe(0);
    });

    it('returns the correct count of requests in the window', () => {
      limiter.recordRequest();
      limiter.recordRequest();
      limiter.recordRequest();

      expect(limiter.getRequestCount()).toBe(3);
    });
  });

  describe('reset', () => {
    it('clears all tracked requests', () => {
      limiter.recordRequest();
      limiter.recordRequest();
      expect(limiter.getRequestCount()).toBe(2);

      limiter.reset();
      expect(limiter.getRequestCount()).toBe(0);
      expect(limiter.getRemainingEstimate()).toBe(5);
    });
  });
});

describe('githubRateLimiter singleton', () => {
  beforeEach(() => {
    // Reset the singleton for each test
    githubRateLimiter.reset();
  });

  it('is a RateLimiter instance', () => {
    expect(githubRateLimiter).toBeInstanceOf(RateLimiter);
  });

  it('has a default limit of 4500 requests per hour', () => {
    expect(githubRateLimiter.getRemainingEstimate()).toBe(4500);
  });
});
