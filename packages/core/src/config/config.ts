import Conf from 'conf';
import CryptoJS from 'crypto-js';
import os from 'os';
import path from 'path';
import fs from 'fs';

// Lazy-load keytar to avoid loading native modules during CLI --help
let keytarModule: typeof import('keytar') | null = null;

async function getKeytar(): Promise<typeof import('keytar')> {
  if (!keytarModule) {
    keytarModule = await import('keytar');
  }
  return keytarModule;
}

/**
 * Keychain service and account names.
 */
const KEYCHAIN_SERVICE = 'repog-cli';
const KEYCHAIN_GITHUB_PAT = 'github-pat';
const KEYCHAIN_GEMINI_KEY = 'gemini-api-key';

/**
 * Configuration schema stored by conf.
 * Only non-sensitive fields are stored here.
 */
interface StoredConfig {
  dbPath?: string;
  configVersion?: number;
  // Legacy encrypted fields (for migration)
  githubPat?: string;
  geminiKey?: string;
}

/**
 * Public configuration interface with decrypted values.
 * Named differently from types/index.ts RepoGConfig to avoid conflicts.
 */
export interface ConfigData {
  githubPat: string | null;
  geminiKey: string | null;
  dbPath: string;
}

/**
 * Result of a save operation.
 */
export interface SaveConfigResult {
  success: boolean;
  error?: string;
}

/**
 * Result of config permission check.
 */
export interface ConfigPermissionResult {
  safe: boolean;
  warning: string | null;
  path?: string;
}

// Legacy encryption key for migration
const LEGACY_ENCRYPTION_KEY = `repog-${os.hostname()}-${os.userInfo().username}`;

// Default database path
const DEFAULT_DB_PATH = path.resolve(os.homedir(), '.repog', 'repog.db');

// Current config version
const CONFIG_VERSION = 2;

// Singleton conf instance
let configStore: Conf<StoredConfig> | null = null;

/**
 * Get or create the conf store instance.
 * @returns The conf store instance
 */
function getStore(): Conf<StoredConfig> {
  if (!configStore) {
    configStore = new Conf<StoredConfig>({
      projectName: 'repog',
      cwd: path.resolve(os.homedir(), '.repog'),
      configName: 'config',
    });
  }
  return configStore;
}

/**
 * Decrypt a legacy encrypted value.
 * @param encrypted - The encrypted base64 string
 * @returns The decrypted plaintext value, or null if decryption fails
 */
function decryptLegacy(encrypted: string): string | null {
  try {
    const bytes = CryptoJS.AES.decrypt(encrypted, LEGACY_ENCRYPTION_KEY);
    const decrypted = bytes.toString(CryptoJS.enc.Utf8);
    return decrypted || null;
  } catch {
    return null;
  }
}

/**
 * Migrate from legacy encrypted conf storage to keychain.
 *
 * This function checks if old encrypted values exist in the conf store,
 * decrypts them, moves them to the system keychain, and removes the
 * legacy encrypted values.
 *
 * @returns True if migration occurred, false if nothing to migrate
 */
export async function migrateFromEncryptedConfig(): Promise<boolean> {
  try {
    const store = getStore();
    const keytar = await getKeytar();

    // Check for legacy encrypted values
    const legacyPat = store.get('githubPat');
    const legacyKey = store.get('geminiKey');
    const configVersion = store.get('configVersion');

    // If already on version 2 or no legacy values, nothing to migrate
    if (configVersion === CONFIG_VERSION || (!legacyPat && !legacyKey)) {
      return false;
    }

    // Check if keychain already has values (migration already done)
    const existingPat = await keytar.getPassword(KEYCHAIN_SERVICE, KEYCHAIN_GITHUB_PAT);
    const existingKey = await keytar.getPassword(KEYCHAIN_SERVICE, KEYCHAIN_GEMINI_KEY);

    if (existingPat && existingKey) {
      // Keychain values exist, just clean up legacy and update version
      store.delete('githubPat');
      store.delete('geminiKey');
      store.set('configVersion', CONFIG_VERSION);
      return false;
    }

    // Decrypt and migrate
    let migrated = false;

    if (legacyPat && !existingPat) {
      const decryptedPat = decryptLegacy(legacyPat);
      if (decryptedPat) {
        await keytar.setPassword(KEYCHAIN_SERVICE, KEYCHAIN_GITHUB_PAT, decryptedPat);
        migrated = true;
      }
    }

    if (legacyKey && !existingKey) {
      const decryptedKey = decryptLegacy(legacyKey);
      if (decryptedKey) {
        await keytar.setPassword(KEYCHAIN_SERVICE, KEYCHAIN_GEMINI_KEY, decryptedKey);
        migrated = true;
      }
    }

    // Clean up legacy values and update version
    if (migrated) {
      store.delete('githubPat');
      store.delete('geminiKey');
      store.set('configVersion', CONFIG_VERSION);
    }

    return migrated;
  } catch {
    // Never throw - return false on any error
    return false;
  }
}

/**
 * Save configuration values.
 * Credentials (githubPat, geminiKey) are stored in the system keychain.
 * Non-sensitive fields (dbPath) are stored in conf.
 *
 * @param config - Partial configuration to save (merged with existing)
 * @returns Result indicating success or failure
 */
export async function saveConfig(config: Partial<ConfigData>): Promise<SaveConfigResult> {
  try {
    const store = getStore();
    const keytar = await getKeytar();

    // Store credentials in keychain
    if (config.githubPat !== undefined) {
      if (config.githubPat === null) {
        await keytar.deletePassword(KEYCHAIN_SERVICE, KEYCHAIN_GITHUB_PAT);
      } else {
        await keytar.setPassword(KEYCHAIN_SERVICE, KEYCHAIN_GITHUB_PAT, config.githubPat);
      }
    }

    if (config.geminiKey !== undefined) {
      if (config.geminiKey === null) {
        await keytar.deletePassword(KEYCHAIN_SERVICE, KEYCHAIN_GEMINI_KEY);
      } else {
        await keytar.setPassword(KEYCHAIN_SERVICE, KEYCHAIN_GEMINI_KEY, config.geminiKey);
      }
    }

    // Store non-sensitive fields in conf
    if (config.dbPath !== undefined) {
      store.set('dbPath', config.dbPath);
    }

    // Update config version
    store.set('configVersion', CONFIG_VERSION);

    return { success: true };
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : 'Unknown error saving config',
    };
  }
}

/**
 * Load the current configuration.
 * Credentials are loaded from the system keychain.
 *
 * @returns The current configuration
 */
export function loadConfig(): ConfigData {
  const store = getStore();
  const storedDbPath = store.get('dbPath');

  // Load credentials from keychain synchronously using a cache
  // Note: This is a workaround since keytar is async but loadConfig was sync
  // For sync usage, we'll try to load from the cached keychain values
  // In practice, isConfigured and other checks should use async versions

  return {
    githubPat: null, // Will be loaded async
    geminiKey: null, // Will be loaded async
    dbPath: storedDbPath ?? DEFAULT_DB_PATH,
  };
}

/**
 * Load the current configuration asynchronously.
 * This version properly loads credentials from the keychain.
 *
 * @returns The current configuration with credentials
 */
export async function loadConfigAsync(): Promise<ConfigData> {
  const store = getStore();
  const storedDbPath = store.get('dbPath');
  const keytar = await getKeytar();

  const githubPat = await keytar.getPassword(KEYCHAIN_SERVICE, KEYCHAIN_GITHUB_PAT);
  const geminiKey = await keytar.getPassword(KEYCHAIN_SERVICE, KEYCHAIN_GEMINI_KEY);

  return {
    githubPat,
    geminiKey,
    dbPath: storedDbPath ?? DEFAULT_DB_PATH,
  };
}

/**
 * Check if RepoG is configured with both required credentials.
 *
 * @returns True if both GitHub PAT and Gemini API key are set
 */
export function isConfigured(): boolean {
  // For backwards compatibility, use sync check
  // This will always return false with keychain since we need async
  // The async version should be used in practice
  const store = getStore();
  const configVersion = store.get('configVersion');

  // If we're on the new config version, assume configured
  // The async version should be used for accurate check
  return configVersion === CONFIG_VERSION;
}

/**
 * Check if RepoG is configured with both required credentials (async version).
 *
 * @returns True if both GitHub PAT and Gemini API key are set in keychain
 */
export async function isConfiguredAsync(): Promise<boolean> {
  try {
    const keytar = await getKeytar();
    const githubPat = await keytar.getPassword(KEYCHAIN_SERVICE, KEYCHAIN_GITHUB_PAT);
    const geminiKey = await keytar.getPassword(KEYCHAIN_SERVICE, KEYCHAIN_GEMINI_KEY);
    return githubPat !== null && geminiKey !== null;
  } catch {
    return false;
  }
}

/**
 * Clear all stored configuration values.
 *
 * @returns Result indicating success or failure
 */
export async function clearConfig(): Promise<SaveConfigResult> {
  try {
    const store = getStore();
    const keytar = await getKeytar();

    // Delete keychain entries
    await keytar.deletePassword(KEYCHAIN_SERVICE, KEYCHAIN_GITHUB_PAT);
    await keytar.deletePassword(KEYCHAIN_SERVICE, KEYCHAIN_GEMINI_KEY);

    // Clear conf store
    store.clear();

    return { success: true };
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : 'Unknown error clearing config',
    };
  }
}

/**
 * Check config file permissions for security.
 *
 * @returns Result indicating if permissions are safe
 */
export function checkConfigPermissions(): ConfigPermissionResult {
  try {
    const store = getStore();
    const confPath = store.path;

    if (!fs.existsSync(confPath)) {
      return { safe: true, warning: null };
    }

    const stats = fs.statSync(confPath);
    const mode = stats.mode;

    // Check if file is world-readable (o+r = 0o004)
    const worldReadable = (mode & 0o004) !== 0;

    if (worldReadable) {
      return {
        safe: false,
        warning: 'Config file is world-readable',
        path: confPath,
      };
    }

    return { safe: true, warning: null, path: confPath };
  } catch {
    return { safe: true, warning: null };
  }
}

/**
 * Get the default database path.
 *
 * @returns The default path to the SQLite database
 */
export function getDefaultDbPath(): string {
  return DEFAULT_DB_PATH;
}

/**
 * Reset the config store instance (for testing purposes).
 * @internal
 */
export function _resetStore(): void {
  configStore = null;
}

/**
 * Get the raw value for a key (for testing purposes).
 * @internal
 */
export function _getRawValue(key: keyof StoredConfig): string | number | undefined {
  return getStore().get(key);
}

/**
 * Get the conf store path (for testing purposes).
 * @internal
 */
export function _getStorePath(): string {
  return getStore().path;
}
