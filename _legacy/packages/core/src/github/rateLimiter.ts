/**
 * Token bucket rate limiter for GitHub API requests.
 *
 * GitHub API allows 5000 requests per hour for authenticated users.
 * This limiter tracks requests in a rolling 1-hour window and throttles
 * requests when the limit is approached to prevent 403 errors.
 */

/**
 * Default requests per hour (conservative, leaves buffer from GitHub's 5000 limit).
 */
const DEFAULT_REQUESTS_PER_HOUR = 4500;

/**
 * Rolling window duration in milliseconds (1 hour).
 */
const WINDOW_MS = 60 * 60 * 1000;

/**
 * A simple token bucket rate limiter for GitHub API.
 */
export class RateLimiter {
  private requestTimestamps: number[] = [];
  private readonly requestsPerHour: number;

  /**
   * Create a new rate limiter.
   *
   * @param requestsPerHour - Maximum requests allowed per hour (default: 4500)
   */
  constructor(requestsPerHour: number = DEFAULT_REQUESTS_PER_HOUR) {
    this.requestsPerHour = requestsPerHour;
  }

  /**
   * Remove timestamps outside the rolling window.
   */
  private pruneOldTimestamps(): void {
    const cutoff = Date.now() - WINDOW_MS;
    this.requestTimestamps = this.requestTimestamps.filter((ts) => ts > cutoff);
  }

  /**
   * Wait if necessary before allowing a request.
   *
   * If the rate limit has been reached, this method will wait until
   * enough time has passed for an old request to fall outside the
   * rolling window.
   *
   * @returns Promise that resolves when a request can proceed
   */
  async throttle(): Promise<void> {
    this.pruneOldTimestamps();

    if (this.requestTimestamps.length < this.requestsPerHour) {
      // Under the limit, can proceed immediately
      return;
    }

    // At the limit - calculate wait time
    // Wait until the oldest request falls outside the window
    const oldestTimestamp = this.requestTimestamps[0];
    const waitUntil = oldestTimestamp + WINDOW_MS;
    const waitMs = waitUntil - Date.now();

    if (waitMs > 0) {
      await new Promise((resolve) => setTimeout(resolve, waitMs));
      // Prune again after waiting
      this.pruneOldTimestamps();
    }
  }

  /**
   * Record a successful request.
   *
   * Call this method after each successful GitHub API request to
   * track usage in the rolling window.
   */
  recordRequest(): void {
    this.requestTimestamps.push(Date.now());
  }

  /**
   * Get the estimated number of remaining requests.
   *
   * @returns Number of requests remaining before the limit is reached
   */
  getRemainingEstimate(): number {
    this.pruneOldTimestamps();
    return Math.max(0, this.requestsPerHour - this.requestTimestamps.length);
  }

  /**
   * Get the number of requests made in the current window.
   *
   * @returns Number of requests in the rolling window
   */
  getRequestCount(): number {
    this.pruneOldTimestamps();
    return this.requestTimestamps.length;
  }

  /**
   * Reset the rate limiter.
   *
   * Clears all tracked requests. Useful for testing.
   */
  reset(): void {
    this.requestTimestamps = [];
  }
}

/**
 * Singleton rate limiter instance for GitHub API.
 * Uses a conservative limit of 4500 requests per hour to leave buffer.
 */
export const githubRateLimiter = new RateLimiter(DEFAULT_REQUESTS_PER_HOUR);
