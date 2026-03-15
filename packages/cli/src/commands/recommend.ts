import type { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';
import {
  isConfiguredAsync,
  loadConfig,
  getDb,
  recommendRepos,
  wrapText,
  type RecommendOptions,
  type RecommendResult,
} from '@repog/core';

/**
 * Options passed from command line.
 */
interface RecommendCommandOptions {
  limit?: number;
  language?: string;
  starred?: boolean;
  owned?: boolean;
  owner?: string;
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
 * Build a string describing active filters.
 */
function buildFilterString(options: RecommendCommandOptions): string | null {
  const parts: string[] = [];

  if (options.language) {
    parts.push(`language=${options.language}`);
  }
  if (options.starred) {
    parts.push('starred');
  }
  if (options.owned) {
    parts.push('owned');
  }
  if (options.owner) {
    parts.push(`owner=${options.owner}`);
  }

  return parts.length > 0 ? parts.join(', ') : null;
}

/**
 * Render a single recommendation.
 */
function renderRecommendation(
  rank: number,
  repoFullName: string,
  htmlUrl: string,
  reasoning: string
): void {
  console.log(`${rank}. ${chalk.bold.cyan(repoFullName)}`);
  console.log(`   ${chalk.gray(htmlUrl)}`);

  // Word wrap reasoning at 80 chars with 5-space indent for continuation
  const wrappedReasoning = wrapText(reasoning, {
    width: 80,
    firstLineIndent: '   Why: ',
    indent: '        ', // 8 spaces to align with "Why: "
  });
  console.log(wrappedReasoning);
  console.log('');
}

/**
 * Render the recommendation results.
 */
function renderResults(result: RecommendResult, options: RecommendCommandOptions): void {
  const { recommendations, query, candidatesConsidered, durationMs } = result;

  // Header
  console.log(`\n${chalk.bold(`Recommendations for: "${query}"`)}`);

  // Show filters if any are active
  const filterStr = buildFilterString(options);
  if (filterStr) {
    console.log(chalk.dim(`Filters: ${filterStr}`));
  }

  console.log(chalk.gray('─'.repeat(45)));
  console.log('');

  // Render each recommendation
  for (const rec of recommendations) {
    renderRecommendation(rec.rank, rec.repoFullName, rec.htmlUrl, rec.reasoning);
  }

  // Footer
  console.log(chalk.gray('─'.repeat(45)));
  console.log(
    chalk.dim(
      `${recommendations.length} ${recommendations.length === 1 ? 'recommendation' : 'recommendations'}  ·  ${candidatesConsidered} candidates considered  ·  ${Math.round(durationMs)}ms`
    )
  );
}

/**
 * Render the empty state message.
 */
function renderEmptyState(query: string, error?: string): void {
  console.log(`\n${chalk.yellow(`No recommendations found for: "${query}"`)}`);
  if (error) {
    console.log(chalk.red(`  Reason: ${error}`));
  }
  console.log('');
  console.log(
    chalk.dim(
      'Try broadening your query or running `repog sync` and `repog embed` to refresh your knowledge base.'
    )
  );
}

/**
 * Run the recommend command.
 */
async function runRecommend(query: string, options: RecommendCommandOptions): Promise<void> {
  // Check if configured
  if (!(await isConfiguredAsync())) {
    console.log(chalk.red('Run `repog init` first.'));
    process.exit(1);
  }

  // Check if embeddings exist
  if (!hasEmbeddings()) {
    console.log(chalk.red('No embeddings found. Run `repog sync` then `repog embed` first.'));
    process.exit(1);
  }

  // Build recommendation options
  const recommendOptions: RecommendOptions = {
    query,
    limit: options.limit ?? 3,
  };

  // Add filters if present
  if (options.language || options.starred || options.owned || options.owner) {
    recommendOptions.filters = {};

    if (options.language) {
      recommendOptions.filters.language = options.language;
    }
    if (options.starred) {
      recommendOptions.filters.starred = true;
    }
    if (options.owned) {
      recommendOptions.filters.owned = true;
    }
    if (options.owner) {
      recommendOptions.filters.owner = options.owner;
    }
  }

  // Run recommendation with spinner
  const spinner = ora('Finding recommendations...').start();

  // Handle Ctrl+C gracefully
  const handleSignal = (): void => {
    spinner.stop();
    console.log(chalk.dim('\nCancelled.'));
    process.exit(0);
  };

  process.on('SIGINT', handleSignal);
  process.on('SIGTERM', handleSignal);

  try {
    const result = await recommendRepos(recommendOptions);
    spinner.stop();

    // Remove signal handlers after completion
    process.off('SIGINT', handleSignal);
    process.off('SIGTERM', handleSignal);

    if (result.recommendations.length === 0) {
      renderEmptyState(query, result.error);
    } else {
      renderResults(result, options);
    }
  } catch (error) {
    spinner.fail(chalk.red('Recommendation failed'));
    if (error instanceof Error) {
      console.error(chalk.red(`  Error: ${error.message}`));
    }
    process.exit(1);
  }
}

/**
 * Register the `repog recommend` command.
 * Gets AI-powered repository recommendations.
 */
export function register(program: Command): void {
  program
    .command('recommend <query>')
    .description('Get AI-powered repository recommendations for a query')
    .option('-l, --limit <number>', 'Number of recommendations to return', parseInt, 3)
    .option('--language <lang>', 'Filter by primary language')
    .option('--starred', 'Only recommend from starred repositories')
    .option('--owned', 'Only recommend from owned repositories')
    .option('--owner <owner>', 'Filter by repo owner username')
    .action(async (query: string, options: RecommendCommandOptions) => {
      await runRecommend(query, options);
    });
}
