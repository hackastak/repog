import type { Command } from 'commander';
import chalk from 'chalk';
import {
  isConfigured,
  loadConfig,
  getDb,
  askQuestion,
  type AskResult,
} from '@repog/core';

/**
 * Options passed from command line.
 */
interface AskCommandOptions {
  repo?: string;
  limit?: number;
}

/**
 * Check if the chunk_embeddings table has at least one row.
 */
function hasEmbeddings(): boolean {
  try {
    const config = loadConfig();
    const db = getDb(config.dbPath);
    const row = db.prepare('SELECT COUNT(*) as count FROM chunk_embeddings').get() as {
      count: number;
    };
    return row.count > 0;
  } catch {
    return false;
  }
}

/**
 * Validate repo format is owner/repo.
 */
function isValidRepoFormat(repo: string): boolean {
  const parts = repo.split('/');
  return parts.length === 2 && parts[0].length > 0 && parts[1].length > 0;
}

/**
 * Format similarity as a percentage.
 */
function formatSimilarity(similarity: number): string {
  return `${Math.round(similarity * 100)}%`;
}

/**
 * Render the question header.
 */
function renderHeader(question: string, repo?: string): void {
  console.log(`Question: "${question}"`);
  console.log(chalk.gray('\u2500'.repeat(45)));
  console.log('');

  if (repo) {
    console.log(`Repo: ${repo}`);
    console.log(chalk.gray('\u2500'.repeat(45)));
    console.log('');
  }
}

/**
 * Render the source attribution footer.
 */
function renderFooter(result: AskResult): void {
  console.log('');
  console.log('');
  console.log(chalk.gray('\u2500'.repeat(45)));
  console.log('Sources:');

  for (const source of result.sources) {
    const similarity = formatSimilarity(source.similarity);
    console.log(chalk.dim(`  \u00b7 ${source.repoFullName} (${source.chunkType})  ${similarity}`));
  }

  console.log('');
  console.log(
    chalk.dim(
      `${result.inputTokens} tokens in  \u00b7  ${result.outputTokens} tokens out  \u00b7  ${Math.round(result.durationMs)}ms`
    )
  );
}

/**
 * Run the ask command.
 */
async function runAsk(question: string, options: AskCommandOptions): Promise<void> {
  // Check if configured
  if (!isConfigured()) {
    console.log(chalk.red('Run `repog init` first.'));
    process.exit(1);
  }

  // Check if embeddings exist
  if (!hasEmbeddings()) {
    console.log(chalk.red('No embeddings found. Run `repog sync` then `repog embed` first.'));
    process.exit(1);
  }

  // Validate repo format if provided
  if (options.repo && !isValidRepoFormat(options.repo)) {
    console.log(chalk.red('Invalid repo format. Use owner/repo (e.g. torvalds/linux).'));
    process.exit(1);
  }

  // Handle Ctrl+C gracefully
  const handleSignal = (): void => {
    console.log(''); // Newline to clean up mid-line streaming
    console.log(chalk.dim('Cancelled.'));
    process.exit(0);
  };

  process.on('SIGINT', handleSignal);
  process.on('SIGTERM', handleSignal);

  // Render header before starting stream
  renderHeader(question, options.repo);

  try {
    // Stream callback - write directly to stdout
    const onChunk = (text: string): void => {
      process.stdout.write(text);
    };

    const result = await askQuestion(
      {
        question,
        repo: options.repo,
        limit: options.limit ?? 10,
      },
      onChunk
    );

    // Remove signal handlers after completion
    process.off('SIGINT', handleSignal);
    process.off('SIGTERM', handleSignal);

    // Check if this is an empty result (no sources)
    if (result.sources.length === 0) {
      // The answer was already streamed, but we need special handling
      // If the answer indicates no information found, show the empty state
      if (
        result.answer.includes("couldn't find any relevant information") ||
        result.inputTokens === 0
      ) {
        // Clear what we've written and show empty state
        // Actually the answer was already streamed via onChunk, so just add the hint
        console.log('');
        console.log('');
        console.log(chalk.dim('Try running `repog sync` and `repog embed` to refresh your knowledge base.'));
      } else {
        // Error case - already printed via streaming
        console.log('');
      }
    } else {
      // Normal case - render footer with sources
      renderFooter(result);
    }
  } catch (error) {
    // Remove signal handlers on error
    process.off('SIGINT', handleSignal);
    process.off('SIGTERM', handleSignal);

    console.log('');
    if (error instanceof Error) {
      console.error(chalk.red(`Error: ${error.message}`));
    } else {
      console.error(chalk.red('An unknown error occurred.'));
    }
    process.exit(1);
  }
}

/**
 * Register the `repog ask` command.
 * Ask questions about your repositories using RAG.
 */
export function register(program: Command): void {
  program
    .command('ask <question>')
    .description('Ask a question about your repositories using AI')
    .option('-r, --repo <name>', 'Scope question to a specific repository (owner/repo)')
    .option('-l, --limit <number>', 'Number of context chunks to retrieve', (v) => parseInt(v, 10), 10)
    .action(async (question: string, options: AskCommandOptions) => {
      await runAsk(question, options);
    });
}
