/**
 * GitHub API data fetching module.
 *
 * Provides functions to fetch repository data from GitHub's REST API,
 * including owned repos, starred repos, READMEs, and file trees.
 */

import { GitHubClient, withRateLimitRetry } from './client.js';

export interface GitHubRepo {
  id: number;
  node_id: string;
  name: string;
  full_name: string;
  description: string | null;
  private: boolean;
  owner: {
    login: string;
    id: number;
  };
  html_url: string;
  clone_url: string;
  ssh_url: string;
  language: string | null;
  stargazers_count: number;
  forks_count: number;
  open_issues_count: number;
  default_branch: string;
  topics: string[];
  pushed_at: string;
  created_at: string;
  updated_at: string;
  archived: boolean;
  fork: boolean;
  size: number;
}

export interface FetchReposOptions {
  owned?: boolean;
  starred?: boolean;
}

/**
 * Fetch owned repositories, paginating until done.
 *
 * @param client - Authenticated GitHub client
 * @returns Async generator yielding GitHubRepo objects
 */
export async function* fetchOwnedRepos(
  client: GitHubClient
): AsyncGenerator<GitHubRepo> {
  let page = 1;
  const perPage = 100;

  while (true) {
    try {
      const response = await withRateLimitRetry(() =>
        client.rest.repos.listForAuthenticatedUser({
          visibility: 'all',
          sort: 'pushed',
          direction: 'desc',
          per_page: perPage,
          page,
        })
      );

      const repos = response.data;

      if (!repos || repos.length === 0) {
        break;
      }

      for (const repo of repos) {
        // Cast strictly to our interface to ensure we have the fields we expect
        yield repo as unknown as GitHubRepo;
      }

      if (repos.length < perPage) {
        break;
      }

      page++;
    } catch (error) {
      console.error(`Error fetching owned repos page ${page}:`, error);
      // Stop pagination on error, but don't crash the whole process
      break;
    }
  }
}

/**
 * Fetch starred repositories, paginating until done.
 *
 * @param client - Authenticated GitHub client
 * @returns Async generator yielding GitHubRepo objects
 */
export async function* fetchStarredRepos(
  client: GitHubClient
): AsyncGenerator<GitHubRepo> {
  let page = 1;
  const perPage = 100;

  while (true) {
    try {
      const response = await withRateLimitRetry(() =>
        client.rest.activity.listReposStarredByAuthenticatedUser({
          sort: 'created', // standard sort for starred
          direction: 'desc',
          per_page: perPage,
          page,
        })
      );

      const repos = response.data;

      if (!repos || repos.length === 0) {
        break;
      }

      for (const repo of repos) {
        yield repo as unknown as GitHubRepo;
      }

      if (repos.length < perPage) {
        break;
      }

      page++;
    } catch (error) {
      console.error(`Error fetching starred repos page ${page}:`, error);
      break;
    }
  }
}

/**
 * Fetch the README content for a repository.
 *
 * @param client - Authenticated GitHub client
 * @param owner - Repository owner
 * @param repo - Repository name
 * @returns Decoded README content or null if not found
 */
export async function fetchReadme(
  client: GitHubClient,
  owner: string,
  repo: string
): Promise<string | null> {
  try {
    const response = await withRateLimitRetry(() =>
      client.rest.repos.getReadme({
        owner,
        repo,
      })
    );

    const content = response.data.content;
    if (!content) {
      return null;
    }

    const decoded = Buffer.from(content, 'base64').toString('utf-8');
    return decoded.trimEnd();
  } catch (error: unknown) {
    // 404 is expected for repos without README
    const httpError = error as { status?: number };
    if (httpError.status === 404) {
      return null;
    }
    // Log other errors but don't throw
    console.error(`Error fetching README for ${owner}/${repo}:`, error instanceof Error ? error.message : error);
    return null;
  }
}

/**
 * Fetch the file tree for a repository.
 *
 * @param client - Authenticated GitHub client
 * @param owner - Repository owner
 * @param repo - Repository name
 * @param branch - Branch to fetch tree for (usually default branch)
 * @returns List of file paths separated by newlines, or null if error
 */
export async function fetchFileTree(
  client: GitHubClient,
  owner: string,
  repo: string,
  branch: string
): Promise<string | null> {
  try {
    const response = await withRateLimitRetry(() =>
      client.rest.git.getTree({
        owner,
        repo,
        tree_sha: branch,
        recursive: '1',
      })
    );

    const tree = response.data.tree;
    if (!tree) {
      return null;
    }

    const filePaths = tree
      .filter((item) => {
        // Only include blobs (files)
        if (item.type !== 'blob' || !item.path) {
          return false;
        }
        
        // Filter depth <= 2 (at most one slash)
        // e.g. "README.md" (0 slashes) -> OK
        // e.g. "src/index.ts" (1 slash) -> OK
        // e.g. "src/components/Button.tsx" (2 slashes) -> Skip
        const slashCount = (item.path.match(/\//g) || []).length;
        return slashCount <= 1;
      })
      .map((item) => item.path!)
      .sort();

    if (filePaths.length === 0) {
      return null;
    }

    return filePaths.join('\n');
  } catch (error: unknown) {
    console.error(`Error fetching file tree for ${owner}/${repo}:`, error instanceof Error ? error.message : error);
    return null;
  }
}
