import type { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';
import { initDb, loadConfigAsync, ingestRepos, NotConfiguredError } from '@repog/core';

/**
 * Register the `repog sync` command.
 * Syncs repositories from GitHub to the local database.
 */
export function register(program: Command): void {
  program
    .command('sync')
    .description('Sync repositories from GitHub to local database')
    .option('--owned', 'Sync owned repositories')
    .option('--starred', 'Sync starred repositories')
    .option('--full-tree', 'Always fetch file trees regardless of README length')
    .option('--verbose', 'Show detailed progress per repository')
    .action(async (options) => {
      const { owned, starred, fullTree, verbose } = options;

      // Require at least one of --owned or --starred
      if (!owned && !starred) {
        console.error(chalk.red('Error: Specify --owned, --starred, or both.'));
        process.exit(1);
      }

      try {
        // Load config to get dbPath
        const config = await loadConfigAsync();
        if (!config.githubPat) {
          console.error(chalk.red('Run `repog init` first.'));
          process.exit(1);
        }

        // Initialize database
        initDb(config.dbPath);

        // Start the ingestion
        const spinner = verbose ? null : ora('Syncing repositories...').start();
        let syncedCount = 0;
        let skippedCount = 0;
        let errorCount = 0;

        for await (const event of ingestRepos({
          includeOwned: !!owned,
          includeStarred: !!starred,
          fullTree: !!fullTree,
          verbose: !!verbose,
        })) {
          switch (event.type) {
            case 'repo':
              syncedCount++;
              if (verbose) {
                const statusIcon = chalk.green('✓');
                const statusText = event.status === 'new' ? 'new    ' : 'updated';
                console.log(`${statusIcon} ${statusText}  ${event.repo}`);
              } else if (spinner) {
                spinner.text = `Syncing repositories... (${syncedCount} synced, ${skippedCount} skipped)`;
              }
              break;

            case 'skip':
              skippedCount++;
              if (verbose) {
                console.log(`${chalk.yellow('~')} skipped  ${event.repo}  (${event.reason})`);
              } else if (spinner) {
                spinner.text = `Syncing repositories... (${syncedCount} synced, ${skippedCount} skipped)`;
              }
              break;

            case 'error':
              errorCount++;
              if (verbose) {
                console.log(`${chalk.red('✗')} error    ${event.repo}     (${event.error})`);
              }
              break;

            case 'done':
              if (spinner) {
                spinner.stop();
              }
              if (event.errors > 0) {
                console.log(
                  chalk.yellow(
                    `✓ Sync complete — ${event.total} repos synced, ${event.skipped} skipped, ${event.errors} errors`
                  )
                );
              } else {
                console.log(
                  chalk.green(
                    `✓ Sync complete — ${event.total} repos synced, ${event.skipped} skipped, ${event.errors} errors`
                  )
                );
              }
              break;
          }
        }

        // Exit with code 1 if there were errors
        if (errorCount > 0) {
          process.exit(1);
        }
      } catch (error) {
        if (error instanceof NotConfiguredError) {
          console.error(chalk.red('Run `repog init` first.'));
          process.exit(1);
        }
        throw error;
      }
    });
}
