import { describe, it, expect } from 'vitest';
import { spawn } from 'child_process';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const CLI_PATH = path.resolve(__dirname, '../dist/index.js');

/**
 * Helper to run CLI commands and capture output.
 */
function runCli(args: string[], timeout = 5000): Promise<{ stdout: string; stderr: string; code: number | null }> {
  return new Promise((resolve) => {
    const proc = spawn('node', [CLI_PATH, ...args], {
      env: { ...process.env, NO_COLOR: '1' },
      timeout,
    });

    let stdout = '';
    let stderr = '';

    proc.stdout.on('data', (data) => {
      stdout += data.toString();
    });

    proc.stderr.on('data', (data) => {
      stderr += data.toString();
    });

    proc.on('close', (code) => {
      resolve({ stdout, stderr, code });
    });

    proc.on('error', (err) => {
      stderr += err.message;
      resolve({ stdout, stderr, code: 1 });
    });
  });
}

describe('CLI E2E', () => {
  describe('repog --help', () => {
    it('displays help text with all commands', async () => {
      const { stdout, code } = await runCli(['--help']);

      expect(code).toBe(0);
      expect(stdout).toContain('Usage: repog [options] [command]');
      expect(stdout).toContain('AI-powered knowledge base');
      expect(stdout).toContain('init');
      expect(stdout).toContain('sync');
      expect(stdout).toContain('embed');
      expect(stdout).toContain('search');
      expect(stdout).toContain('recommend');
      expect(stdout).toContain('ask');
      expect(stdout).toContain('summarize');
      expect(stdout).toContain('status');
    });
  });

  describe('repog --version', () => {
    it('displays version number', async () => {
      const { stdout, code } = await runCli(['--version']);

      expect(code).toBe(0);
      expect(stdout.trim()).toMatch(/^\d+\.\d+\.\d+$/);
    });
  });

  describe('repog init --help', () => {
    it('displays init command help', async () => {
      const { stdout, code } = await runCli(['init', '--help']);

      expect(code).toBe(0);
      expect(stdout).toContain('Initialize RepoG');
      expect(stdout).toContain('--github-token');
      expect(stdout).toContain('--gemini-key');
      expect(stdout).toContain('--db-path');
      expect(stdout).toContain('--force');
    });
  });

  describe('repog sync --help', () => {
    it('displays sync command help', async () => {
      const { stdout, code } = await runCli(['sync', '--help']);

      expect(code).toBe(0);
      expect(stdout).toContain('Sync repositories');
    });
  });

  describe('repog embed --help', () => {
    it('displays embed command help', async () => {
      const { stdout, code } = await runCli(['embed', '--help']);

      expect(code).toBe(0);
      expect(stdout).toContain('Generate embeddings');
    });
  });

  describe('repog search --help', () => {
    it('displays search command help', async () => {
      const { stdout, code } = await runCli(['search', '--help']);

      expect(code).toBe(0);
      expect(stdout).toContain('Semantic search');
      expect(stdout).toContain('--limit');
      expect(stdout).toContain('--language');
      expect(stdout).toContain('--starred');
      expect(stdout).toContain('--owned');
      expect(stdout).toContain('--owner');
    });
  });

  describe('repog recommend --help', () => {
    it('displays recommend command help', async () => {
      const { stdout, code } = await runCli(['recommend', '--help']);

      expect(code).toBe(0);
      expect(stdout).toContain('recommendation');
    });
  });

  describe('repog ask --help', () => {
    it('displays ask command help', async () => {
      const { stdout, code } = await runCli(['ask', '--help']);

      expect(code).toBe(0);
      expect(stdout).toContain('question');
    });
  });

  describe('repog summarize --help', () => {
    it('displays summarize command help', async () => {
      const { stdout, code } = await runCli(['summarize', '--help']);

      expect(code).toBe(0);
      expect(stdout).toContain('summary');
    });
  });

  describe('repog status --help', () => {
    it('displays status command help', async () => {
      const { stdout, code } = await runCli(['status', '--help']);

      expect(code).toBe(0);
      expect(stdout).toContain('Show sync and embedding status');
      expect(stdout).toContain('--json');
    });
  });

  describe('unknown command', () => {
    it('shows error for unknown command', async () => {
      const { stderr, code } = await runCli(['unknown-command']);

      expect(code).toBe(1);
      expect(stderr).toContain("error: unknown command 'unknown-command'");
    });
  });

  describe('search without query', () => {
    it('shows error when query is missing', async () => {
      const { stderr, code } = await runCli(['search']);

      expect(code).toBe(1);
      expect(stderr).toContain("error: missing required argument 'query'");
    });
  });

  describe('summarize without repo', () => {
    it('shows error when repo is missing', async () => {
      const { stderr, code } = await runCli(['summarize']);

      expect(code).toBe(1);
      expect(stderr).toContain("error: missing required argument 'repo'");
    });
  });
});

/**
 * Helper to run CLI with isolated config (not configured state).
 */
function runCliUnconfigured(args: string[], timeout = 5000): Promise<{ stdout: string; stderr: string; code: number | null }> {
  return new Promise((resolve) => {
    const proc = spawn('node', [CLI_PATH, ...args], {
      env: {
        ...process.env,
        NO_COLOR: '1',
        // Use a temp directory that doesn't have config
        XDG_CONFIG_HOME: '/tmp/repog-test-nonexistent-config',
      },
      timeout,
    });

    let stdout = '';
    let stderr = '';

    proc.stdout.on('data', (data) => {
      stdout += data.toString();
    });

    proc.stderr.on('data', (data) => {
      stderr += data.toString();
    });

    proc.on('close', (code) => {
      resolve({ stdout, stderr, code });
    });

    proc.on('error', (err) => {
      stderr += err.message;
      resolve({ stdout, stderr, code: 1 });
    });
  });
}

describe('CLI E2E - Unconfigured State', () => {
  describe('repog status (unconfigured)', () => {
    it('shows error prompting to run init first', async () => {
      const { stdout, stderr, code } = await runCliUnconfigured(['status']);

      expect(code).toBe(1);
      // The error could be in stdout or stderr depending on chalk output
      const output = stdout + stderr;
      expect(output).toContain('init');
    });
  });

  describe('repog search (unconfigured)', () => {
    it('shows error prompting to run init first', async () => {
      const { stdout, stderr, code } = await runCliUnconfigured(['search', 'test query']);

      expect(code).toBe(1);
      const output = stdout + stderr;
      expect(output).toContain('init');
    });
  });

  describe('repog sync (unconfigured)', () => {
    it('exits without crashing (sync not yet fully implemented)', async () => {
      const { code } = await runCliUnconfigured(['sync']);

      // sync command is not yet implemented, so it exits cleanly
      expect(code).toBe(0);
    });
  });

  describe('repog embed (unconfigured)', () => {
    it('shows error prompting to run init first', async () => {
      const { stdout, stderr, code } = await runCliUnconfigured(['embed']);

      expect(code).toBe(1);
      const output = stdout + stderr;
      expect(output).toContain('init');
    });
  });

  describe('repog recommend (unconfigured)', () => {
    it('shows error prompting to run init first', async () => {
      const { stdout, stderr, code } = await runCliUnconfigured(['recommend', 'test']);

      expect(code).toBe(1);
      const output = stdout + stderr;
      expect(output).toContain('init');
    });
  });

  describe('repog ask (unconfigured)', () => {
    it('shows error prompting to run init first', async () => {
      const { stdout, stderr, code } = await runCliUnconfigured(['ask', 'test question']);

      expect(code).toBe(1);
      const output = stdout + stderr;
      expect(output).toContain('init');
    });
  });

  describe('repog summarize (unconfigured)', () => {
    it('shows error prompting to run init first', async () => {
      const { stdout, stderr, code } = await runCliUnconfigured(['summarize', 'owner/repo']);

      expect(code).toBe(1);
      const output = stdout + stderr;
      expect(output).toContain('init');
    });
  });
});
