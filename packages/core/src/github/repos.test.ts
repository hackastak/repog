import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { fetchOwnedRepos, fetchStarredRepos, fetchReadme, fetchFileTree } from './repos.js';
import { GitHubClient } from './client.js';

// Mock the rate limiter to avoid delays in tests
vi.mock('./rateLimiter.js', () => ({
  githubRateLimiter: {
    throttle: vi.fn().mockResolvedValue(undefined),
    recordRequest: vi.fn(),
  },
}));

// Helper to create a mock GitHub repo
function createMockRepo(overrides: Record<string, unknown> = {}) {
  return {
    id: 123456,
    node_id: 'R_123456',
    name: 'test-repo',
    full_name: 'testuser/test-repo',
    description: 'A test repository',
    private: false,
    owner: { login: 'testuser', id: 1 },
    html_url: 'https://github.com/testuser/test-repo',
    clone_url: 'https://github.com/testuser/test-repo.git',
    ssh_url: 'git@github.com:testuser/test-repo.git',
    language: 'TypeScript',
    stargazers_count: 100,
    forks_count: 10,
    open_issues_count: 5,
    default_branch: 'main',
    topics: ['typescript', 'testing'],
    pushed_at: '2024-01-15T10:30:00Z',
    created_at: '2023-01-01T00:00:00Z',
    updated_at: '2024-01-15T10:30:00Z',
    archived: false,
    fork: false,
    size: 1024,
    ...overrides,
  };
}

// Create a mock GitHubClient
function createMockClient(mockRest: Record<string, unknown> = {}): GitHubClient {
  return {
    rest: {
      repos: {
        listForAuthenticatedUser: vi.fn(),
        getReadme: vi.fn(),
      },
      activity: {
        listReposStarredByAuthenticatedUser: vi.fn(),
      },
      git: {
        getTree: vi.fn(),
      },
      ...mockRest,
    },
  } as unknown as GitHubClient;
}

describe('fetchOwnedRepos', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('yields repos from a single page correctly', async () => {
    const mockRepos = [
      createMockRepo({ id: 1, full_name: 'user/repo1' }),
      createMockRepo({ id: 2, full_name: 'user/repo2' }),
    ];

    const client = createMockClient();
    vi.mocked(client.rest.repos.listForAuthenticatedUser).mockResolvedValueOnce({
      data: mockRepos,
      status: 200,
      headers: {},
      url: '',
    } as never);

    const repos: unknown[] = [];
    for await (const repo of fetchOwnedRepos(client)) {
      repos.push(repo);
    }

    expect(repos).toHaveLength(2);
    expect(repos[0]).toMatchObject({ full_name: 'user/repo1' });
    expect(repos[1]).toMatchObject({ full_name: 'user/repo2' });
  });

  it('paginates through multiple pages and yields all repos', async () => {
    // Create 100 repos for first page (full page)
    const page1Repos = Array.from({ length: 100 }, (_, i) =>
      createMockRepo({ id: i + 1, full_name: `user/repo${i + 1}` })
    );
    // Create 50 repos for second page (partial page - signals end)
    const page2Repos = Array.from({ length: 50 }, (_, i) =>
      createMockRepo({ id: i + 101, full_name: `user/repo${i + 101}` })
    );

    const client = createMockClient();
    vi.mocked(client.rest.repos.listForAuthenticatedUser)
      .mockResolvedValueOnce({ data: page1Repos, status: 200, headers: {}, url: '' } as never)
      .mockResolvedValueOnce({ data: page2Repos, status: 200, headers: {}, url: '' } as never);

    const repos: unknown[] = [];
    for await (const repo of fetchOwnedRepos(client)) {
      repos.push(repo);
    }

    expect(repos).toHaveLength(150);
    expect(client.rest.repos.listForAuthenticatedUser).toHaveBeenCalledTimes(2);
  });

  it('stops paginating when a page has fewer than 100 results', async () => {
    const mockRepos = Array.from({ length: 50 }, (_, i) =>
      createMockRepo({ id: i + 1, full_name: `user/repo${i + 1}` })
    );

    const client = createMockClient();
    vi.mocked(client.rest.repos.listForAuthenticatedUser).mockResolvedValueOnce({
      data: mockRepos,
      status: 200,
      headers: {},
      url: '',
    } as never);

    const repos: unknown[] = [];
    for await (const repo of fetchOwnedRepos(client)) {
      repos.push(repo);
    }

    expect(repos).toHaveLength(50);
    expect(client.rest.repos.listForAuthenticatedUser).toHaveBeenCalledTimes(1);
  });

  it('stops on error and does not throw', async () => {
    const client = createMockClient();
    vi.mocked(client.rest.repos.listForAuthenticatedUser).mockRejectedValueOnce(
      new Error('Network error')
    );

    const repos: unknown[] = [];
    // Should not throw
    for await (const repo of fetchOwnedRepos(client)) {
      repos.push(repo);
    }

    expect(repos).toHaveLength(0);
  });
});

describe('fetchStarredRepos', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('yields repos from a single page correctly', async () => {
    const mockRepos = [
      createMockRepo({ id: 1, full_name: 'popular/project1' }),
      createMockRepo({ id: 2, full_name: 'popular/project2' }),
    ];

    const client = createMockClient();
    vi.mocked(client.rest.activity.listReposStarredByAuthenticatedUser).mockResolvedValueOnce({
      data: mockRepos,
      status: 200,
      headers: {},
      url: '',
    } as never);

    const repos: unknown[] = [];
    for await (const repo of fetchStarredRepos(client)) {
      repos.push(repo);
    }

    expect(repos).toHaveLength(2);
    expect(repos[0]).toMatchObject({ full_name: 'popular/project1' });
  });

  it('paginates correctly across multiple pages', async () => {
    const page1Repos = Array.from({ length: 100 }, (_, i) =>
      createMockRepo({ id: i + 1, full_name: `starred/repo${i + 1}` })
    );
    const page2Repos = Array.from({ length: 30 }, (_, i) =>
      createMockRepo({ id: i + 101, full_name: `starred/repo${i + 101}` })
    );

    const client = createMockClient();
    vi.mocked(client.rest.activity.listReposStarredByAuthenticatedUser)
      .mockResolvedValueOnce({ data: page1Repos, status: 200, headers: {}, url: '' } as never)
      .mockResolvedValueOnce({ data: page2Repos, status: 200, headers: {}, url: '' } as never);

    const repos: unknown[] = [];
    for await (const repo of fetchStarredRepos(client)) {
      repos.push(repo);
    }

    expect(repos).toHaveLength(130);
    expect(client.rest.activity.listReposStarredByAuthenticatedUser).toHaveBeenCalledTimes(2);
  });

  it('stops on error and does not throw', async () => {
    const client = createMockClient();
    vi.mocked(client.rest.activity.listReposStarredByAuthenticatedUser).mockRejectedValueOnce(
      new Error('API error')
    );

    const repos: unknown[] = [];
    for await (const repo of fetchStarredRepos(client)) {
      repos.push(repo);
    }

    expect(repos).toHaveLength(0);
  });
});

describe('fetchReadme', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('returns decoded base64 content on 200 response', async () => {
    const readmeContent = '# Test README\n\nThis is a test README file.';
    const base64Content = Buffer.from(readmeContent).toString('base64');

    const client = createMockClient();
    vi.mocked(client.rest.repos.getReadme).mockResolvedValueOnce({
      data: { content: base64Content },
      status: 200,
      headers: {},
      url: '',
    } as never);

    const result = await fetchReadme(client, 'testuser', 'test-repo');

    expect(result).toBe(readmeContent);
  });

  it('strips trailing whitespace from decoded content', async () => {
    const readmeContent = '# Test README\n\n  \n';
    const base64Content = Buffer.from(readmeContent).toString('base64');

    const client = createMockClient();
    vi.mocked(client.rest.repos.getReadme).mockResolvedValueOnce({
      data: { content: base64Content },
      status: 200,
      headers: {},
      url: '',
    } as never);

    const result = await fetchReadme(client, 'testuser', 'test-repo');

    expect(result).toBe('# Test README');
  });

  it('returns null on 404 — does not throw', async () => {
    const error = new Error('Not Found') as Error & { status: number };
    error.status = 404;

    const client = createMockClient();
    vi.mocked(client.rest.repos.getReadme).mockRejectedValueOnce(error);

    const result = await fetchReadme(client, 'testuser', 'no-readme-repo');

    expect(result).toBeNull();
  });

  it('returns null on network error — does not throw', async () => {
    const client = createMockClient();
    vi.mocked(client.rest.repos.getReadme).mockRejectedValueOnce(new Error('Network error'));

    const result = await fetchReadme(client, 'testuser', 'test-repo');

    expect(result).toBeNull();
  });

  it('returns null when content field is empty', async () => {
    const client = createMockClient();
    vi.mocked(client.rest.repos.getReadme).mockResolvedValueOnce({
      data: { content: '' },
      status: 200,
      headers: {},
      url: '',
    } as never);

    const result = await fetchReadme(client, 'testuser', 'test-repo');

    expect(result).toBeNull();
  });
});

describe('fetchFileTree', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('returns only files at depth ≤ 2 (paths with at most one /)', async () => {
    const mockTree = {
      tree: [
        { path: 'README.md', type: 'blob' },
        { path: 'package.json', type: 'blob' },
        { path: 'src/index.ts', type: 'blob' },
        { path: 'src/utils/helper.ts', type: 'blob' }, // depth 3, should be excluded
        { path: 'src/components/ui/Button.tsx', type: 'blob' }, // depth 4, should be excluded
      ],
    };

    const client = createMockClient();
    vi.mocked(client.rest.git.getTree).mockResolvedValueOnce({
      data: mockTree,
      status: 200,
      headers: {},
      url: '',
    } as never);

    const result = await fetchFileTree(client, 'testuser', 'test-repo', 'main');

    expect(result).toBe('README.md\npackage.json\nsrc/index.ts');
  });

  it('returns paths sorted alphabetically joined by newlines', async () => {
    const mockTree = {
      tree: [
        { path: 'z-file.ts', type: 'blob' },
        { path: 'a-file.ts', type: 'blob' },
        { path: 'm-file.ts', type: 'blob' },
      ],
    };

    const client = createMockClient();
    vi.mocked(client.rest.git.getTree).mockResolvedValueOnce({
      data: mockTree,
      status: 200,
      headers: {},
      url: '',
    } as never);

    const result = await fetchFileTree(client, 'testuser', 'test-repo', 'main');

    expect(result).toBe('a-file.ts\nm-file.ts\nz-file.ts');
  });

  it('returns null on API error — does not throw', async () => {
    const client = createMockClient();
    vi.mocked(client.rest.git.getTree).mockRejectedValueOnce(new Error('API error'));

    const result = await fetchFileTree(client, 'testuser', 'test-repo', 'main');

    expect(result).toBeNull();
  });

  it('excludes directory entries (non-blob tree items)', async () => {
    const mockTree = {
      tree: [
        { path: 'README.md', type: 'blob' },
        { path: 'src', type: 'tree' }, // directory, should be excluded
        { path: 'src/index.ts', type: 'blob' },
        { path: 'docs', type: 'tree' }, // directory, should be excluded
      ],
    };

    const client = createMockClient();
    vi.mocked(client.rest.git.getTree).mockResolvedValueOnce({
      data: mockTree,
      status: 200,
      headers: {},
      url: '',
    } as never);

    const result = await fetchFileTree(client, 'testuser', 'test-repo', 'main');

    expect(result).toBe('README.md\nsrc/index.ts');
    expect(result).not.toContain('src\n'); // The directory itself shouldn't be listed
  });

  it('returns null when no files match the criteria', async () => {
    const mockTree = {
      tree: [
        { path: 'src/deep/nested/file.ts', type: 'blob' }, // too deep
        { path: 'src', type: 'tree' }, // directory
      ],
    };

    const client = createMockClient();
    vi.mocked(client.rest.git.getTree).mockResolvedValueOnce({
      data: mockTree,
      status: 200,
      headers: {},
      url: '',
    } as never);

    const result = await fetchFileTree(client, 'testuser', 'test-repo', 'main');

    expect(result).toBeNull();
  });
});
