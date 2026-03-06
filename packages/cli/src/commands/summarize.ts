import { Command } from 'commander';
import chalk from 'chalk';
import {
  isConfigured,
  loadConfig,
  getDb,
  summarizeRepo
} from '@repog/core';

/**
 * Validate repo format is owner/repo.
 */
function isValidRepoFormat(repo: string): boolean {
  const parts = repo.split('/');
  return parts.length === 2 && parts[0].length > 0 && parts[1].length > 0;
}

/**
 * Check if the repos table has at least one row.
 */
function hasRepos(): boolean {
  try {
    const config = loadConfig();
    const db = getDb(config.dbPath);
    const row = db.prepare('SELECT COUNT(*) as count FROM repos').get() as {
      count: number;
    };
    return row.count > 0;
  } catch {
    return false;
  }
}

/**
 * Check if a specific repo exists in the DB.
 */
function repoExists(repoFullName: string): boolean {
  try {
    const config = loadConfig();
    const db = getDb(config.dbPath);
    const row = db.prepare('SELECT 1 FROM repos WHERE LOWER(full_name) = LOWER(?)').get(repoFullName);
    return !!row;
  } catch {
    return false;
  }
}

/**
 * Check if chunks exist for a specific repo.
 */
function hasChunks(repoFullName: string): boolean {
  try {
    const config = loadConfig();
    const db = getDb(config.dbPath);
    const row = db.prepare(`
        SELECT COUNT(c.id) as count
        FROM chunks c
        JOIN repos r ON r.id = c.repo_id
        WHERE LOWER(r.full_name) = LOWER(?)
    `).get(repoFullName) as { count: number };
    return row.count > 0;
  } catch {
    return false;
  }
}

/**
 * Register the `repog summarize` command.
 * Generate an AI summary of a repository.
 */
export function register(program: Command): void {
  program
    .command('summarize <repo>')
    .description('Generate an AI summary of a repository')
    .action(async (repo) => {
      // 1. Validate format
      if (!isValidRepoFormat(repo)) {
        console.error(chalk.red('Invalid repo format. Use owner/repo (e.g. torvalds/linux).'));
        process.exit(1);
      }

      // 2. Check configuration
      if (!isConfigured()) {
        console.error(chalk.red('Run `repog init` first.'));
        process.exit(1);
      }

      // 3. Check if any repos exist
      if (!hasRepos()) {
        console.error(chalk.red('No repositories found. Run `repog sync` first.'));
        process.exit(1);
      }
      
      // 4. Check if specific repo exists
      if (!repoExists(repo)) {
         console.error(chalk.red(`Repository not found: ${repo}`));
         console.error('');
         console.error(chalk.dim('Run `repog sync` to ingest your repositories first.'));
         process.exit(1);
      }
      
      // 5. Check if chunks exist
      if (!hasChunks(repo)) {
        console.log(`No data found for: ${repo}`);
        console.log('');
        console.log('Make sure the repo was synced and embedded:');
        console.log('  repog sync --owned');
        console.log('  repog embed');
        process.exit(0);
      }

      // 6. Print header
      console.log(`Summary: ${repo}`);
      console.log(chalk.gray('─────────────────────────────────────────────'));
      console.log('');

      // 7. Handle Cancel
      const handleSignal = () => {
        console.log('');
        console.log(chalk.dim('Cancelled.'));
        process.exit(0);
      };
      process.on('SIGINT', handleSignal);
      process.on('SIGTERM', handleSignal);

      try {
        // 8. Start stream
        const onChunk = (text: string) => {
            process.stdout.write(text);
        };

        const result = await summarizeRepo({ repo }, onChunk);

        // 9. Print footer
        console.log('');
        console.log('');
        console.log(chalk.gray('─────────────────────────────────────────────'));
        console.log(
          chalk.dim(
            `${result.chunksUsed} chunks used  ·  ${result.inputTokens} tokens in  ·  ${result.outputTokens} tokens out  ·  ${Math.round(result.durationMs)}ms`
          )
        );

      } catch (error) {
        console.log('');
        if (error instanceof Error) {
           console.error(chalk.red(`Error: ${error.message}`));
        } else {
           console.error(chalk.red('An unknown error occurred.'));
        }
        process.exit(1);
      } finally {
        process.off('SIGINT', handleSignal);
        process.off('SIGTERM', handleSignal);
      }
    });
}
