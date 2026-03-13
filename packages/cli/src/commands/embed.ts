import type { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';
import { isConfigured, runEmbedPipeline, hasRepos, type EmbedProgress } from '@repog/core';

/**
 * Options passed from command line.
 */
interface EmbedOptions {
  includeFileTree?: boolean;
  verbose?: boolean;
  batchSize?: number;
}

/**
 * Run the embed command in default mode (with spinner).
 */
async function runDefaultMode(options: EmbedOptions): Promise<void> {
  const spinner = ora('Generating embeddings...').start();

  let lastProgress: EmbedProgress | null = null;

  // Validate and warn about batch size
  let batchSize = options.batchSize;
  if (batchSize !== undefined && batchSize > 100) {
    console.log(chalk.yellow(`Warning: Batch size ${batchSize} exceeds maximum, capping at 100`));
    batchSize = 100;
  }

  try {
    for await (const progress of runEmbedPipeline({
      includeFileTree: options.includeFileTree ?? false,
      batchSize,
    })) {
      lastProgress = progress;

      if (progress.type === 'batch') {
        const total = progress.totalChunks ?? '?';
        spinner.text = `Generating embeddings... ${progress.chunksEmbedded}/${total} chunks`;
      }
      // repo_skip and error events are silently tracked in default mode
    }

    spinner.succeed(chalk.green('Embeddings generated'));

    // Print summary
    if (lastProgress) {
      printSummary(lastProgress);
    }
  } catch (error) {
    spinner.fail(chalk.red('Embedding failed'));
    if (error instanceof Error) {
      console.error(chalk.red(`  Error: ${error.message}`));
    }
    process.exit(1);
  }
}

/**
 * Run the embed command in verbose mode (no spinner).
 */
async function runVerboseMode(options: EmbedOptions): Promise<void> {
  console.log(chalk.bold('\nGenerating embeddings...\n'));

  let lastProgress: EmbedProgress | null = null;

  // Validate and warn about batch size
  let batchSize = options.batchSize;
  if (batchSize !== undefined && batchSize > 100) {
    console.log(chalk.yellow(`Warning: Batch size ${batchSize} exceeds maximum, capping at 100`));
    batchSize = 100;
  }

  try {
    for await (const progress of runEmbedPipeline({
      includeFileTree: options.includeFileTree ?? false,
      batchSize,
    })) {
      lastProgress = progress;

      switch (progress.type) {
        case 'batch':
          console.log(
            chalk.green(
              `  + Batch ${progress.batchIndex}/${progress.batchTotal} - ${progress.chunksEmbedded} chunks embedded`
            )
          );

          // Report individual chunk errors in this batch if any
          if (progress.batchErrors && progress.batchErrors.length > 0) {
            progress.batchErrors.forEach((err) => {
              console.log(chalk.red(`    x Chunk ${err.chunkId}: ${err.error}`));
            });
          }
          break;

        case 'repo_skip':
          console.log(chalk.gray(`  ~ ${progress.repoFullName} (already embedded)`));
          break;

        case 'error':
          console.log(chalk.red(`  x Error: ${progress.errorMessage}`));
          break;
      }
    }

    console.log('');

    // Print summary
    if (lastProgress) {
      printSummary(lastProgress);
    }
  } catch (error) {
    console.error(chalk.red('\nEmbedding failed'));
    if (error instanceof Error) {
      console.error(chalk.red(`  Error: ${error.message}`));
    }
    process.exit(1);
  }
}

/**
 * Print the final summary.
 */
function printSummary(progress: EmbedProgress): void {
  console.log(chalk.bold('\nSummary:'));
  console.log(`  Chunks embedded: ${chalk.green(progress.chunksEmbedded ?? 0)}`);

  if ((progress.chunksSkipped ?? 0) > 0) {
    console.log(`  Chunks skipped:  ${chalk.gray(progress.chunksSkipped)} (repos unchanged)`);
  }

  if ((progress.chunksErrored ?? 0) > 0) {
    console.log(`  Chunks errored:  ${chalk.red(progress.chunksErrored)}`);
  }

  console.log('');
}

/**
 * Run the embed command logic.
 */
async function runEmbed(options: EmbedOptions): Promise<void> {
  // Check if configured
  if (!isConfigured()) {
    console.log(chalk.red('Run `repog init` first.'));
    process.exit(1);
  }

  // Check if any repos exist
  if (!hasRepos()) {
    console.log(chalk.red('No repositories found. Run `repog sync` first.'));
    process.exit(1);
  }

  if (options.verbose) {
    await runVerboseMode(options);
  } else {
    await runDefaultMode(options);
  }
}

/**
 * Register the `repog embed` command.
 * Generates embeddings for repository content.
 */
export function register(program: Command): void {
  program
    .command('embed')
    .description('Generate embeddings for repository content')
    .option('--include-file-tree', 'Include file tree chunks in embeddings', false)
    .option('-v, --verbose', 'Show detailed progress', false)
    .option('--batch-size <number>', 'Batch size for embedding requests (default: 20, max: 100)', (value) => {
      const parsed = parseInt(value, 10);
      if (isNaN(parsed) || parsed < 1) {
        throw new Error('Batch size must be a positive integer');
      }
      return parsed;
    })
    .action(async (options: EmbedOptions) => {
      await runEmbed(options);
    });
}
