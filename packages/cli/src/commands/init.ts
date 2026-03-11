import type { Command } from 'commander';
import chalk from 'chalk';
import ora from 'ora';
import { input, password, confirm } from '@inquirer/prompts';
import {
  saveConfig,
  loadConfig,
  isConfigured,
  initDb,
  validateGitHubToken,
  validateGeminiKey,
  getDefaultDbPath,
  migrateFromEncryptedConfig,
  checkConfigPermissions,
} from '@repog/core';

/**
 * Options passed from command line.
 */
interface InitOptions {
  githubToken?: string;
  geminiKey?: string;
  dbPath?: string;
  force?: boolean;
}

/**
 * Handle Ctrl+C (SIGINT) gracefully.
 * Prints a message and exits with code 0.
 */
function setupSignalHandlers(): void {
  const handler = (): void => {
    console.log('\nSetup cancelled.');
    process.exit(0);
  };

  process.on('SIGINT', handler);
  process.on('SIGTERM', handler);
}

/**
 * Mask a token/key for safe display.
 * Shows first 4 and last 4 characters with asterisks in between.
 */
function maskSecret(secret: string): string {
  if (secret.length <= 8) {
    return '****';
  }
  return `${secret.slice(0, 4)}${'*'.repeat(8)}${secret.slice(-4)}`;
}

/**
 * Run the init command logic.
 */
async function runInit(options: InitOptions): Promise<void> {
  setupSignalHandlers();

  const spinner = ora();

  console.log(chalk.bold('\nRepoG Setup\n'));

  // Attempt migration from legacy encrypted storage to keychain
  try {
    const migrated = await migrateFromEncryptedConfig();
    if (migrated) {
      console.log(chalk.green('  Migrated credentials from legacy storage to system keychain.'));
      console.log('');
    }
  } catch {
    // Migration failed silently - proceed with normal init
  }

  // Check if already configured
  if (isConfigured() && !options.force) {
    const config = loadConfig();
    console.log(chalk.yellow('RepoG is already configured.'));
    console.log(`  GitHub: ${config.githubPat ? chalk.green('configured') : chalk.red('not set')}`);
    console.log(`  Gemini: ${config.geminiKey ? chalk.green('configured') : chalk.red('not set')}`);
    console.log(`  Database: ${config.dbPath}`);
    console.log('');

    const overwrite = await confirm({
      message: 'Do you want to reconfigure?',
      default: false,
    });

    if (!overwrite) {
      console.log(chalk.gray('Setup cancelled.'));
      return;
    }
  }

  // Get GitHub token
  let githubToken = options.githubToken;
  if (!githubToken) {
    console.log(chalk.bold('RepoG requires a fine-grained GitHub Personal Access Token.\n'));
    console.log(chalk.dim('Create one at: https://github.com/settings/personal-access-tokens/new\n'));
    console.log(chalk.dim('Required settings:'));
    console.log(chalk.dim('  Resource owner:     Your GitHub account'));
    console.log(chalk.dim('  Repository access:  All repositories (or select specific repos)'));
    console.log(chalk.dim('  Permissions:'));
    console.log(chalk.dim('    Contents:         Read-only'));
    console.log(chalk.dim('    Metadata:         Read-only'));
    console.log('');

    githubToken = await password({
      message: 'GitHub Personal Access Token:',
      mask: '*',
      validate: (value) => {
        if (!value || value.trim() === '') {
          return 'Token is required';
        }
        return true;
      },
    });
  }

  // Validate GitHub token
  spinner.start('Validating GitHub token...');
  const githubResult = await validateGitHubToken(githubToken);

  if (!githubResult.valid) {
    spinner.fail(chalk.red(`GitHub token invalid: ${githubResult.error}`));
    process.exit(1);
  }

  // Handle token type messaging
  if (githubResult.tokenType === 'classic') {
    spinner.succeed(chalk.green(`GitHub token valid (${githubResult.login})`));
    console.log('');
    console.log(chalk.yellow('  Classic PAT detected. RepoG works best with a fine-grained PAT.'));
    console.log(chalk.yellow('  Fine-grained PATs provide read-only access and are more secure.'));
    console.log(chalk.yellow('  Consider regenerating your token at:'));
    console.log(chalk.yellow('  https://github.com/settings/personal-access-tokens/new'));
    console.log('');
  } else {
    spinner.succeed(chalk.green(`Fine-grained PAT validated - logged in as @${githubResult.login}`));
  }

  // Get Gemini API key
  let geminiKey = options.geminiKey;
  if (!geminiKey) {
    console.log('');
    console.log(chalk.dim('Get a Gemini API key at: https://aistudio.google.com/apikey\n'));

    geminiKey = await password({
      message: 'Gemini API Key:',
      mask: '*',
      validate: (value) => {
        if (!value || value.trim() === '') {
          return 'API key is required';
        }
        return true;
      },
    });
  }

  // Validate Gemini API key
  spinner.start('Validating Gemini API key...');
  const geminiResult = await validateGeminiKey(geminiKey);

  if (!geminiResult.valid) {
    spinner.fail(chalk.red(`Gemini API key invalid: ${geminiResult.error}`));
    process.exit(1);
  }

  spinner.succeed(chalk.green('Gemini API key valid'));

  // Get database path
  const defaultDbPath = getDefaultDbPath();
  let dbPath = options.dbPath;

  if (!dbPath) {
    console.log('');
    dbPath = await input({
      message: 'Database path:',
      default: defaultDbPath,
    });
  }

  // Initialize database
  spinner.start('Initializing database...');
  const dbResult = initDb(dbPath);

  if (!dbResult.success) {
    spinner.fail(chalk.red(`Database init failed: ${dbResult.error}`));
    process.exit(1);
  }

  spinner.succeed(chalk.green(`Database initialized (${dbResult.tablesCreated.length} tables)`));

  // Save configuration
  spinner.start('Saving configuration...');
  const saveResult = await saveConfig({
    githubPat: githubToken,
    geminiKey: geminiKey,
    dbPath: dbPath,
  });

  if (!saveResult.success) {
    spinner.fail(chalk.red(`Failed to save config: ${saveResult.error}`));
    process.exit(1);
  }

  spinner.succeed(chalk.green('Configuration saved'));

  // Check config file permissions
  const permCheck = checkConfigPermissions();
  if (!permCheck.safe && permCheck.warning) {
    console.log('');
    console.log(chalk.yellow(`  Config file has loose permissions: ${permCheck.path}`));
    console.log(chalk.yellow(`  Run: chmod 600 "${permCheck.path}"`));
  }

  // Summary
  console.log('');
  console.log(chalk.bold.green('Setup complete!'));
  console.log('');
  console.log('Configuration:');
  console.log(`  GitHub: ${chalk.cyan(maskSecret(githubToken))} (${githubResult.login})`);
  console.log(`  Gemini: ${chalk.cyan(maskSecret(geminiKey))}`);
  console.log(`  Database: ${chalk.cyan(dbPath)}`);
  console.log('');
  console.log('Next steps:');
  console.log(`  ${chalk.cyan('repog sync')}     - Sync your GitHub repositories`);
  console.log(`  ${chalk.cyan('repog status')}   - Check sync status`);
  console.log('');
}

/**
 * Register the `repog init` command.
 * Initializes RepoG with GitHub and Gemini credentials.
 */
export function register(program: Command): void {
  program
    .command('init')
    .description('Initialize RepoG with your GitHub and Gemini API credentials')
    .option('--github-token <token>', 'GitHub Personal Access Token')
    .option('--gemini-key <key>', 'Google Gemini API Key')
    .option('--db-path <path>', 'Custom database path')
    .option('-f, --force', 'Overwrite existing configuration')
    .action(async (options: InitOptions) => {
      try {
        await runInit(options);
      } catch (error) {
        // Handle ExitPromptError from inquirer (Ctrl+C)
        if (error instanceof Error && error.name === 'ExitPromptError') {
          console.log('\nSetup cancelled.');
          process.exit(0);
        }
        throw error;
      }
    });
}
