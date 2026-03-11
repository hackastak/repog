import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { validateGitHubToken, hasScope, getRequiredScopes } from './auth.js';

// Mock the Octokit module
vi.mock('octokit', () => {
  return {
    Octokit: vi.fn(),
  };
});

// Import after mock setup
import { Octokit } from 'octokit';

const mockOctokit = vi.mocked(Octokit);

describe('validateGitHubToken', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('fine-grained PATs', () => {
    it('returns valid: true and tokenType: fine-grained when x-oauth-scopes header is absent', async () => {
      mockOctokit.mockImplementation(() => ({
        rest: {
          users: {
            getAuthenticated: vi.fn().mockResolvedValue({
              data: { login: 'testuser', id: 12345 },
              headers: {}, // No x-oauth-scopes header
            }),
          },
          repos: {
            listForAuthenticatedUser: vi.fn().mockResolvedValue({
              data: [],
            }),
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await validateGitHubToken('github_pat_validtoken123');

      expect(result.valid).toBe(true);
      expect(result.login).toBe('testuser');
      expect(result.tokenType).toBe('fine-grained');
      expect(result.error).toBeNull();
    });
  });

  describe('classic PATs', () => {
    it('returns valid: true and tokenType: classic when x-oauth-scopes header is present', async () => {
      mockOctokit.mockImplementation(() => ({
        rest: {
          users: {
            getAuthenticated: vi.fn().mockResolvedValue({
              data: { login: 'classicuser', id: 67890 },
              headers: { 'x-oauth-scopes': 'repo, read:user' },
            }),
          },
          repos: {
            listForAuthenticatedUser: vi.fn().mockResolvedValue({
              data: [],
            }),
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await validateGitHubToken('ghp_classictoken123');

      expect(result.valid).toBe(true);
      expect(result.login).toBe('classicuser');
      expect(result.tokenType).toBe('classic');
      expect(result.error).toBeNull();
    });
  });

  describe('permission errors', () => {
    it('returns valid: false with permissions error when GET /user/repos returns 403', async () => {
      const reposError = new Error('Resource not accessible by integration');
      (reposError as Error & { status: number }).status = 403;

      mockOctokit.mockImplementation(() => ({
        rest: {
          users: {
            getAuthenticated: vi.fn().mockResolvedValue({
              data: { login: 'limiteduser', id: 11111 },
              headers: {}, // Fine-grained PAT
            }),
          },
          repos: {
            listForAuthenticatedUser: vi.fn().mockRejectedValue(reposError),
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await validateGitHubToken('github_pat_limitedtoken');

      expect(result.valid).toBe(false);
      expect(result.login).toBe('limiteduser');
      expect(result.tokenType).toBe('fine-grained');
      expect(result.error).toContain('Contents: Read-only');
    });

    it('returns valid: false with permissions error when GET /user returns 403', async () => {
      const userError = new Error('Resource not accessible by integration');
      (userError as Error & { status: number }).status = 403;

      mockOctokit.mockImplementation(() => ({
        rest: {
          users: {
            getAuthenticated: vi.fn().mockRejectedValue(userError),
          },
          repos: {
            listForAuthenticatedUser: vi.fn(),
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await validateGitHubToken('github_pat_nometadata');

      expect(result.valid).toBe(false);
      expect(result.login).toBeNull();
      expect(result.error).toContain('Metadata: Read-only');
    });
  });

  describe('invalid tokens', () => {
    it('returns valid: false when GET /user returns 401', async () => {
      const error = new Error('Bad credentials');
      (error as Error & { status: number }).status = 401;

      mockOctokit.mockImplementation(() => ({
        rest: {
          users: {
            getAuthenticated: vi.fn().mockRejectedValue(error),
          },
          repos: {
            listForAuthenticatedUser: vi.fn(),
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await validateGitHubToken('ghp_invalidtoken');

      expect(result.valid).toBe(false);
      expect(result.error).toContain('Invalid token');
      expect(result.login).toBeNull();
      expect(result.tokenType).toBe('unknown');
    });

    it('returns valid: false when token is empty', async () => {
      const result = await validateGitHubToken('');

      expect(result.valid).toBe(false);
      expect(result.error).toBe('Token is empty');
      expect(result.login).toBeNull();
      expect(result.tokenType).toBe('unknown');
    });

    it('returns valid: false when token is whitespace only', async () => {
      const result = await validateGitHubToken('   ');

      expect(result.valid).toBe(false);
      expect(result.error).toBe('Token is empty');
      expect(result.login).toBeNull();
      expect(result.tokenType).toBe('unknown');
    });
  });

  describe('network errors', () => {
    it('returns valid: false on network error and does not throw', async () => {
      mockOctokit.mockImplementation(() => ({
        rest: {
          users: {
            getAuthenticated: vi.fn().mockRejectedValue(new Error('Network connection failed')),
          },
          repos: {
            listForAuthenticatedUser: vi.fn(),
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await validateGitHubToken('ghp_sometoken');

      expect(result.valid).toBe(false);
      expect(result.error).toBeDefined();
      expect(result.error).toContain('Failed to validate token');
    });

    it('returns valid: false on timeout and does not throw', async () => {
      mockOctokit.mockImplementation(() => ({
        rest: {
          users: {
            getAuthenticated: vi.fn().mockRejectedValue(new Error('ETIMEDOUT')),
          },
          repos: {
            listForAuthenticatedUser: vi.fn(),
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await validateGitHubToken('ghp_timeouttoken');

      expect(result.valid).toBe(false);
      expect(result.error).toBeDefined();
    });
  });

  describe('login parsing', () => {
    it('correctly parses login from GET /user response body', async () => {
      mockOctokit.mockImplementation(() => ({
        rest: {
          users: {
            getAuthenticated: vi.fn().mockResolvedValue({
              data: { login: 'my-github-username', id: 99999 },
              headers: {},
            }),
          },
          repos: {
            listForAuthenticatedUser: vi.fn().mockResolvedValue({
              data: [],
            }),
          },
        },
      }) as unknown as InstanceType<typeof Octokit>);

      const result = await validateGitHubToken('github_pat_validtoken');

      expect(result.login).toBe('my-github-username');
    });
  });
});

describe('hasScope', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('returns true when classic token has the specified scope', async () => {
    mockOctokit.mockImplementation(() => ({
      rest: {
        users: {
          getAuthenticated: vi.fn().mockResolvedValue({
            data: { login: 'user', id: 1 },
            headers: { 'x-oauth-scopes': 'repo, read:user, admin:org' },
          }),
        },
      },
    }) as unknown as InstanceType<typeof Octokit>);

    const result = await hasScope('ghp_token', 'admin:org');

    expect(result).toBe(true);
  });

  it('returns false when classic token lacks the specified scope', async () => {
    mockOctokit.mockImplementation(() => ({
      rest: {
        users: {
          getAuthenticated: vi.fn().mockResolvedValue({
            data: { login: 'user', id: 1 },
            headers: { 'x-oauth-scopes': 'repo, read:user' },
          }),
        },
      },
    }) as unknown as InstanceType<typeof Octokit>);

    const result = await hasScope('ghp_token', 'admin:org');

    expect(result).toBe(false);
  });

  it('returns false for fine-grained PATs (no scopes)', async () => {
    mockOctokit.mockImplementation(() => ({
      rest: {
        users: {
          getAuthenticated: vi.fn().mockResolvedValue({
            data: { login: 'user', id: 1 },
            headers: {}, // No x-oauth-scopes header
          }),
        },
      },
    }) as unknown as InstanceType<typeof Octokit>);

    const result = await hasScope('github_pat_token', 'repo');

    expect(result).toBe(false);
  });

  it('returns false for invalid token', async () => {
    const error = new Error('Bad credentials');
    (error as Error & { status: number }).status = 401;

    mockOctokit.mockImplementation(() => ({
      rest: {
        users: {
          getAuthenticated: vi.fn().mockRejectedValue(error),
        },
      },
    }) as unknown as InstanceType<typeof Octokit>);

    const result = await hasScope('ghp_invalid', 'repo');

    expect(result).toBe(false);
  });
});

describe('getRequiredScopes', () => {
  it('returns array containing repo scope', () => {
    const scopes = getRequiredScopes();

    expect(Array.isArray(scopes)).toBe(true);
    expect(scopes).toContain('repo');
  });

  it('returns a new array each time (not mutable)', () => {
    const scopes1 = getRequiredScopes();
    const scopes2 = getRequiredScopes();

    expect(scopes1).not.toBe(scopes2);
    expect(scopes1).toEqual(scopes2);
  });
});
