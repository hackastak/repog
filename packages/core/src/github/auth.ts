import { Octokit } from 'octokit';

/**
 * Result of GitHub token validation.
 */
export interface PatValidationResult {
  /** Whether the token is valid and has required permissions */
  valid: boolean;
  /** GitHub username if valid */
  login: string | null;
  /** Type of token detected */
  tokenType: 'fine-grained' | 'classic' | 'unknown';
  /** Error message if validation failed */
  error: string | null;
}

/**
 * @deprecated Use PatValidationResult instead. Kept for backwards compatibility.
 */
export interface GitHubAuthResult {
  /** Whether the token is valid and has required scopes */
  valid: boolean;
  /** GitHub username if valid */
  login?: string;
  /** Scopes granted to the token */
  scopes?: string[];
  /** Error message if validation failed */
  error?: string;
}

/**
 * Validate a GitHub Personal Access Token.
 *
 * Works with both fine-grained PATs and classic PATs:
 * - Fine-grained PATs: Validated by checking API access to /user and /user/repos
 * - Classic PATs: Still supported but detected via x-oauth-scopes header
 *
 * Does not throw - returns structured result.
 *
 * @param token - The GitHub PAT to validate
 * @returns Validation result with login info and token type
 */
export async function validateGitHubToken(token: string): Promise<PatValidationResult> {
  if (!token || token.trim() === '') {
    return {
      valid: false,
      login: null,
      tokenType: 'unknown',
      error: 'Token is empty',
    };
  }

  let tokenType: 'fine-grained' | 'classic' | 'unknown' = 'unknown';
  let login: string | null = null;

  try {
    const octokit = new Octokit({ auth: token });

    // Step 1: Call GET /user to verify token is valid and has Metadata: Read-only
    let userResponse;
    try {
      userResponse = await octokit.rest.users.getAuthenticated();
      login = userResponse.data.login;

      // Detect token type by checking for x-oauth-scopes header
      // Fine-grained PATs do not return this header
      const scopeHeader = userResponse.headers['x-oauth-scopes'];
      tokenType = scopeHeader !== undefined ? 'classic' : 'fine-grained';
    } catch (error) {
      // Handle specific error types for /user endpoint
      if (error instanceof Error && 'status' in error) {
        const status = (error as { status: number }).status;
        if (status === 401) {
          return {
            valid: false,
            login: null,
            tokenType: 'unknown',
            error: 'Invalid token - authentication failed',
          };
        }
        if (status === 403) {
          return {
            valid: false,
            login: null,
            tokenType,
            error: 'Token lacks required permission: Metadata: Read-only',
          };
        }
      }
      throw error; // Re-throw for network errors
    }

    // Step 2: Call GET /user/repos to verify token has Contents: Read-only
    try {
      await octokit.rest.repos.listForAuthenticatedUser({ per_page: 1 });
    } catch (error) {
      if (error instanceof Error && 'status' in error) {
        const status = (error as { status: number }).status;
        if (status === 403) {
          return {
            valid: false,
            login,
            tokenType,
            error: 'Token lacks required permission: Contents: Read-only',
          };
        }
      }
      throw error; // Re-throw for other errors
    }

    // If we get here, both checks passed
    return {
      valid: true,
      login,
      tokenType,
      error: null,
    };
  } catch (error) {
    // Handle network errors or other issues
    if (error instanceof Error) {
      return {
        valid: false,
        login,
        tokenType,
        error: `Failed to validate token: ${error.message}`,
      };
    }

    return {
      valid: false,
      login: null,
      tokenType: 'unknown',
      error: 'Unknown error validating token',
    };
  }
}

/**
 * Check if a token has a specific scope (classic PATs only).
 *
 * Note: This function only works with classic PATs that return x-oauth-scopes.
 * Fine-grained PATs do not have traditional scopes.
 *
 * @param token - The GitHub PAT to check
 * @param scope - The scope to check for
 * @returns True if the token has the scope (always false for fine-grained PATs)
 * @deprecated Fine-grained PATs do not use scopes. Use validateGitHubToken instead.
 */
export async function hasScope(token: string, scope: string): Promise<boolean> {
  if (!token || token.trim() === '') {
    return false;
  }

  try {
    const octokit = new Octokit({ auth: token });
    const response = await octokit.rest.users.getAuthenticated();

    const scopeHeader = response.headers['x-oauth-scopes'];
    if (!scopeHeader) {
      // Fine-grained PAT - no scopes available
      return false;
    }

    const scopes = scopeHeader.split(',').map((s: string) => s.trim());
    return scopes.includes(scope);
  } catch {
    return false;
  }
}

/**
 * Get the list of required scopes for RepoG.
 *
 * @returns Array of required OAuth scope names
 * @deprecated Fine-grained PATs do not use scopes. Use validateGitHubToken instead.
 */
export function getRequiredScopes(): string[] {
  // For classic PATs, repo scope is still needed
  return ['repo'];
}
