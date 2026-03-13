import { Octokit } from 'octokit';
import { loadConfigAsync } from '../config/config.js';
import { githubRateLimiter } from './rateLimiter.js';

export class GitHubClient extends Octokit {
  constructor(token: string) {
    super({ auth: token });
  }
}

export class RateLimitError extends Error {
  constructor(public resetAt: Date) {
    super(`Rate limit exceeded. Resets at ${resetAt.toISOString()}`);
    this.name = 'RateLimitError';
  }
}

export async function withRateLimitRetry<T>(
  operation: () => Promise<T>,
  _retries = 3
): Promise<T> {
  await githubRateLimiter.throttle();

  try {
    const result = await operation();
    githubRateLimiter.recordRequest();
    return result;
  } catch (error: unknown) {
    // Check for rate limit error (403 or 429)
    const httpError = error as { status?: number; response?: { headers?: Record<string, string> } };
    if (httpError.status === 403 || httpError.status === 429) {
      const remaining = httpError.response?.headers?.['x-ratelimit-remaining'];
      if (remaining === '0') {
        const resetTime = httpError.response?.headers?.['x-ratelimit-reset'];
        if (resetTime) {
          throw new RateLimitError(new Date(parseInt(resetTime) * 1000));
        }
      }
    }
    throw error;
  }
}

export interface RateLimitStats {
  limit: number;
  remaining: number;
  resetAt: string;   // ISO datetime string
  available: boolean; // true if remaining > 0
}

/**
 * Get the current GitHub API rate limit status.
 *
 * @returns Rate limit statistics or null if the request fails
 */
export async function getRateLimitInfo(): Promise<RateLimitStats | null> {
  const config = await loadConfigAsync();
  
  // If no token is configured, we can't check rate limits accurately for authenticated user
  // (Unauthenticated requests have very low limits)
  if (!config.githubPat) {
    return null;
  }

  try {
    const octokit = new Octokit({ auth: config.githubPat });
    const { data } = await octokit.rest.rateLimit.get();
    
    // Use core rate limit which applies to most API calls
    const core = data.resources.core;
    
    return {
      limit: core.limit,
      remaining: core.remaining,
      resetAt: new Date(core.reset * 1000).toISOString(),
      available: core.remaining > 0
    };
  } catch {
    // If request fails (network error, auth error, etc.), return null
    // The caller is expected to handle this gracefully
    return null;
  }
}
