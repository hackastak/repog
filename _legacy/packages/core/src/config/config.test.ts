import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';

// Create mock functions
const mockSetPassword = vi.fn().mockResolvedValue(undefined);
const mockGetPassword = vi.fn().mockResolvedValue(null);
const mockDeletePassword = vi.fn().mockResolvedValue(true);

// Mock keytar with both default export and named exports for dynamic import
vi.mock('keytar', () => ({
  default: {
    setPassword: mockSetPassword,
    getPassword: mockGetPassword,
    deletePassword: mockDeletePassword,
  },
  setPassword: mockSetPassword,
  getPassword: mockGetPassword,
  deletePassword: mockDeletePassword,
}));

import {
  saveConfig,
  loadConfig,
  loadConfigAsync,
  isConfigured,
  isConfiguredAsync,
  clearConfig,
  migrateFromEncryptedConfig,
  checkConfigPermissions,
  getDefaultDbPath,
  _resetStore,
  _getRawValue,
  _getStorePath,
} from './config.js';
import CryptoJS from 'crypto-js';
import Conf from 'conf';
import os from 'os';
import path from 'path';
import fs from 'fs';

// Use a unique test directory to avoid conflicts
const TEST_CONFIG_DIR = path.join(os.tmpdir(), `repog-test-${process.pid}-${Date.now()}`);

// Override conf's directory for testing
const originalHomedir = os.homedir;

describe('config', () => {
  beforeEach(() => {
    // Reset all mocks
    vi.clearAllMocks();

    // Reset the singleton store before each test
    _resetStore();

    // Create test directory
    fs.mkdirSync(TEST_CONFIG_DIR, { recursive: true });

    // Mock homedir to use test directory
    (os as { homedir: () => string }).homedir = () => TEST_CONFIG_DIR;
  });

  afterEach(() => {
    // Restore original homedir
    (os as { homedir: () => string }).homedir = originalHomedir;

    // Clean up test directory
    try {
      fs.rmSync(TEST_CONFIG_DIR, { recursive: true, force: true });
    } catch {
      // Ignore cleanup errors
    }

    _resetStore();
  });

  describe('getDefaultDbPath', () => {
    it('returns default db path containing .repog and repog.db', () => {
      const dbPath = getDefaultDbPath();

      expect(dbPath).toContain('.repog');
      expect(dbPath).toContain('repog.db');
    });
  });

  describe('_getStorePath', () => {
    it('returns the conf store path', () => {
      const storePath = _getStorePath();

      expect(storePath).toContain('.repog');
      expect(storePath).toContain('config.json');
    });
  });

  describe('saveConfig', () => {
    it('calls keytar.setPassword for GitHub PAT', async () => {
      const testPat = 'ghp_testPatToken123456789';

      await saveConfig({ githubPat: testPat });

      expect(mockSetPassword).toHaveBeenCalledWith(
        'repog-cli',
        'github-pat',
        testPat
      );
    });

    it('calls keytar.setPassword for Gemini API key', async () => {
      const testKey = 'AIzaSyTestGeminiKey123456';

      await saveConfig({ geminiKey: testKey });

      expect(mockSetPassword).toHaveBeenCalledWith(
        'repog-cli',
        'gemini-api-key',
        testKey
      );
    });

    it('stores both PAT and Gemini key in keychain', async () => {
      const testPat = 'ghp_testPatToken123456789';
      const testKey = 'AIzaSyTestGeminiKey123456';

      await saveConfig({ githubPat: testPat, geminiKey: testKey });

      expect(mockSetPassword).toHaveBeenCalledTimes(2);
      expect(mockSetPassword).toHaveBeenCalledWith(
        'repog-cli',
        'github-pat',
        testPat
      );
      expect(mockSetPassword).toHaveBeenCalledWith(
        'repog-cli',
        'gemini-api-key',
        testKey
      );
    });

    it('stores dbPath in conf (not keychain)', async () => {
      const testDbPath = '/custom/path/to/db.sqlite';

      await saveConfig({ dbPath: testDbPath });

      // dbPath should be in conf store, not keychain
      expect(mockSetPassword).not.toHaveBeenCalled();

      // Check conf store
      const stored = _getRawValue('dbPath');
      expect(stored).toBe(testDbPath);
    });

    it('calls keytar.deletePassword when setting githubPat to null', async () => {
      await saveConfig({ githubPat: null });

      expect(mockDeletePassword).toHaveBeenCalledWith(
        'repog-cli',
        'github-pat'
      );
    });

    it('calls keytar.deletePassword when setting geminiKey to null', async () => {
      await saveConfig({ geminiKey: null });

      expect(mockDeletePassword).toHaveBeenCalledWith(
        'repog-cli',
        'gemini-api-key'
      );
    });

    it('returns success: true on successful save', async () => {
      const result = await saveConfig({ githubPat: 'test_token' });

      expect(result.success).toBe(true);
      expect(result.error).toBeUndefined();
    });

    it('returns error when keytar throws an Error', async () => {
      mockSetPassword.mockRejectedValueOnce(new Error('Keychain access denied'));

      const result = await saveConfig({ githubPat: 'test_token' });

      expect(result.success).toBe(false);
      expect(result.error).toBe('Keychain access denied');
    });

    it('returns generic error when keytar throws non-Error', async () => {
      mockSetPassword.mockRejectedValueOnce('string error');

      const result = await saveConfig({ githubPat: 'test_token' });

      expect(result.success).toBe(false);
      expect(result.error).toBe('Unknown error saving config');
    });
  });

  describe('loadConfig', () => {
    it('returns dbPath from conf store', async () => {
      const testDbPath = '/custom/path/to/db.sqlite';

      // Save dbPath using saveConfig
      await saveConfig({ dbPath: testDbPath });

      _resetStore();

      const loaded = loadConfig();

      expect(loaded.dbPath).toBe(testDbPath);
    });

    it('returns default dbPath when not set', () => {
      const loaded = loadConfig();

      expect(loaded.dbPath).toContain('.repog');
      expect(loaded.dbPath).toContain('repog.db');
    });
  });

  describe('loadConfigAsync', () => {
    it('calls keytar.getPassword for both credentials', async () => {
      mockGetPassword
        .mockResolvedValueOnce('ghp_testPat')
        .mockResolvedValueOnce('AIzaSyTestKey');

      const loaded = await loadConfigAsync();

      expect(mockGetPassword).toHaveBeenCalledTimes(2);
      expect(mockGetPassword).toHaveBeenCalledWith('repog-cli', 'github-pat');
      expect(mockGetPassword).toHaveBeenCalledWith('repog-cli', 'gemini-api-key');
      expect(loaded.githubPat).toBe('ghp_testPat');
      expect(loaded.geminiKey).toBe('AIzaSyTestKey');
    });

    it('returns null for missing credentials', async () => {
      mockGetPassword.mockResolvedValue(null);

      const loaded = await loadConfigAsync();

      expect(loaded.githubPat).toBeNull();
      expect(loaded.geminiKey).toBeNull();
    });
  });

  describe('isConfigured', () => {
    it('returns false when configVersion is not set', () => {
      expect(isConfigured()).toBe(false);
    });

    it('returns true when configVersion is 2', async () => {
      // Save config to set version
      await saveConfig({ githubPat: 'test' });

      expect(isConfigured()).toBe(true);
    });
  });

  describe('isConfiguredAsync', () => {
    it('returns false when keychain returns null', async () => {
      mockGetPassword.mockResolvedValue(null);

      expect(await isConfiguredAsync()).toBe(false);
    });

    it('returns true when both credentials are in keychain', async () => {
      mockGetPassword
        .mockResolvedValueOnce('ghp_testPat')
        .mockResolvedValueOnce('AIzaSyTestKey');

      expect(await isConfiguredAsync()).toBe(true);
    });

    it('returns false when only one credential is set', async () => {
      mockGetPassword
        .mockResolvedValueOnce('ghp_testPat')
        .mockResolvedValueOnce(null);

      expect(await isConfiguredAsync()).toBe(false);
    });

    it('returns false when keytar throws an error', async () => {
      mockGetPassword.mockRejectedValueOnce(new Error('Keychain error'));

      expect(await isConfiguredAsync()).toBe(false);
    });
  });

  describe('clearConfig', () => {
    it('calls keytar.deletePassword for both entries', async () => {
      await clearConfig();

      expect(mockDeletePassword).toHaveBeenCalledWith(
        'repog-cli',
        'github-pat'
      );
      expect(mockDeletePassword).toHaveBeenCalledWith(
        'repog-cli',
        'gemini-api-key'
      );
    });

    it('clears the conf store', async () => {
      // First save something
      await saveConfig({ dbPath: '/custom/path' });

      // Clear
      const result = await clearConfig();

      expect(result.success).toBe(true);
    });

    it('returns error when keytar throws an Error', async () => {
      mockDeletePassword.mockRejectedValueOnce(new Error('Keychain locked'));

      const result = await clearConfig();

      expect(result.success).toBe(false);
      expect(result.error).toBe('Keychain locked');
    });

    it('returns generic error when keytar throws non-Error', async () => {
      mockDeletePassword.mockRejectedValueOnce({ code: 'UNKNOWN' });

      const result = await clearConfig();

      expect(result.success).toBe(false);
      expect(result.error).toBe('Unknown error clearing config');
    });
  });

  describe('migrateFromEncryptedConfig', () => {
    // Helper to create legacy encrypted value
    const LEGACY_ENCRYPTION_KEY = `repog-${os.hostname()}-${os.userInfo().username}`;

    function encryptLegacy(value: string): string {
      return CryptoJS.AES.encrypt(value, LEGACY_ENCRYPTION_KEY).toString();
    }

    function getTestStore(): Conf<{ githubPat?: string; geminiKey?: string; configVersion?: number; dbPath?: string }> {
      return new Conf({
        projectName: 'repog',
        cwd: path.join(TEST_CONFIG_DIR, '.repog'),
        configName: 'config',
      });
    }

    it('returns false when no legacy values exist', async () => {
      const result = await migrateFromEncryptedConfig();

      expect(result).toBe(false);
    });

    it('returns false when already on config version 2', async () => {
      // Save with new format to set version 2
      await saveConfig({ githubPat: 'test' });

      const result = await migrateFromEncryptedConfig();

      expect(result).toBe(false);
    });

    it('cleans up legacy values when keychain already has both values', async () => {
      // Set up legacy encrypted values
      const store = getTestStore();
      const encryptedPat = encryptLegacy('ghp_legacy_pat');
      const encryptedKey = encryptLegacy('AIzaSy_legacy_key');
      store.set('githubPat', encryptedPat);
      store.set('geminiKey', encryptedKey);

      // Reset store singleton to pick up changes
      _resetStore();

      // Mock keychain to return existing values
      mockGetPassword
        .mockResolvedValueOnce('existing_pat')
        .mockResolvedValueOnce('existing_key');

      const result = await migrateFromEncryptedConfig();

      // Should return false since keychain already has values
      expect(result).toBe(false);

      // Legacy values should be cleaned up
      _resetStore();
      const updatedStore = getTestStore();
      expect(updatedStore.get('githubPat')).toBeUndefined();
      expect(updatedStore.get('geminiKey')).toBeUndefined();
      expect(updatedStore.get('configVersion')).toBe(2);
    });

    it('migrates legacy PAT to keychain', async () => {
      // Set up legacy encrypted PAT only
      const store = getTestStore();
      const encryptedPat = encryptLegacy('ghp_legacy_pat_token');
      store.set('githubPat', encryptedPat);

      // Reset store singleton to pick up changes
      _resetStore();

      // Mock keychain to return no existing values
      mockGetPassword.mockResolvedValue(null);

      const result = await migrateFromEncryptedConfig();

      expect(result).toBe(true);
      expect(mockSetPassword).toHaveBeenCalledWith(
        'repog-cli',
        'github-pat',
        'ghp_legacy_pat_token'
      );
    });

    it('migrates legacy Gemini key to keychain', async () => {
      // Set up legacy encrypted Gemini key only
      const store = getTestStore();
      const encryptedKey = encryptLegacy('AIzaSy_legacy_gemini_key');
      store.set('geminiKey', encryptedKey);

      // Reset store singleton to pick up changes
      _resetStore();

      // Mock keychain to return no existing values
      mockGetPassword.mockResolvedValue(null);

      const result = await migrateFromEncryptedConfig();

      expect(result).toBe(true);
      expect(mockSetPassword).toHaveBeenCalledWith(
        'repog-cli',
        'gemini-api-key',
        'AIzaSy_legacy_gemini_key'
      );
    });

    it('migrates both legacy values to keychain', async () => {
      // Set up both legacy encrypted values
      const store = getTestStore();
      const encryptedPat = encryptLegacy('ghp_both_test_pat');
      const encryptedKey = encryptLegacy('AIzaSy_both_test_key');
      store.set('githubPat', encryptedPat);
      store.set('geminiKey', encryptedKey);

      // Reset store singleton to pick up changes
      _resetStore();

      // Mock keychain to return no existing values
      mockGetPassword.mockResolvedValue(null);

      const result = await migrateFromEncryptedConfig();

      expect(result).toBe(true);
      expect(mockSetPassword).toHaveBeenCalledWith(
        'repog-cli',
        'github-pat',
        'ghp_both_test_pat'
      );
      expect(mockSetPassword).toHaveBeenCalledWith(
        'repog-cli',
        'gemini-api-key',
        'AIzaSy_both_test_key'
      );

      // Verify legacy values are cleaned up
      _resetStore();
      const updatedStore = getTestStore();
      expect(updatedStore.get('githubPat')).toBeUndefined();
      expect(updatedStore.get('geminiKey')).toBeUndefined();
      expect(updatedStore.get('configVersion')).toBe(2);
    });

    it('skips migration for values that fail to decrypt', async () => {
      // Set up invalid encrypted values (not valid AES)
      const store = getTestStore();
      store.set('githubPat', 'not-valid-encrypted-data');
      store.set('geminiKey', 'also-not-valid-encrypted');

      // Reset store singleton to pick up changes
      _resetStore();

      // Mock keychain to return no existing values
      mockGetPassword.mockResolvedValue(null);

      const result = await migrateFromEncryptedConfig();

      // Should return false since nothing could be decrypted
      expect(result).toBe(false);
      expect(mockSetPassword).not.toHaveBeenCalled();
    });

    it('migrates only PAT when keychain already has Gemini key', async () => {
      // Set up both legacy encrypted values
      const store = getTestStore();
      const encryptedPat = encryptLegacy('ghp_partial_test_pat');
      const encryptedKey = encryptLegacy('AIzaSy_partial_test_key');
      store.set('githubPat', encryptedPat);
      store.set('geminiKey', encryptedKey);

      // Reset store singleton to pick up changes
      _resetStore();

      // Mock keychain: no PAT but has Gemini key
      mockGetPassword
        .mockResolvedValueOnce(null) // PAT
        .mockResolvedValueOnce('existing_gemini_key'); // Gemini

      const result = await migrateFromEncryptedConfig();

      expect(result).toBe(true);
      expect(mockSetPassword).toHaveBeenCalledWith(
        'repog-cli',
        'github-pat',
        'ghp_partial_test_pat'
      );
      // Should not set Gemini key since it already exists
      expect(mockSetPassword).not.toHaveBeenCalledWith(
        'repog-cli',
        'gemini-api-key',
        expect.any(String)
      );
    });

    it('returns false when keytar throws an error', async () => {
      // Set up legacy encrypted values
      const store = getTestStore();
      const encryptedPat = encryptLegacy('ghp_error_test');
      store.set('githubPat', encryptedPat);

      // Reset store singleton to pick up changes
      _resetStore();

      // Mock keytar.getPassword to throw
      mockGetPassword.mockRejectedValueOnce(new Error('Keychain unavailable'));

      const result = await migrateFromEncryptedConfig();

      // Should return false on error
      expect(result).toBe(false);
    });
  });

  describe('checkConfigPermissions', () => {
    it('returns safe: true when config file does not exist', () => {
      const result = checkConfigPermissions();

      expect(result.safe).toBe(true);
      expect(result.warning).toBeNull();
    });

    it('returns safe: true with path when file exists and is not world-readable', async () => {
      // Create a config file first
      await saveConfig({ dbPath: '/test' });

      const storePath = _getStorePath();

      // Ensure it's NOT world-readable (mode 0o600)
      fs.chmodSync(storePath, 0o600);

      const result = checkConfigPermissions();

      expect(result.safe).toBe(true);
      expect(result.warning).toBeNull();
      expect(result.path).toBeDefined();
      expect(result.path).toContain('.repog');
    });

    it('returns safe: false when file is world-readable', async () => {
      // Create a config file first
      await saveConfig({ dbPath: '/test' });

      const storePath = _getStorePath();

      // Make it world-readable (mode 0o644 includes o+r)
      fs.chmodSync(storePath, 0o644);

      const result = checkConfigPermissions();

      expect(result.safe).toBe(false);
      expect(result.warning).toBe('Config file is world-readable');
      expect(result.path).toBe(storePath);

      // Restore permissions
      fs.chmodSync(storePath, 0o600);
    });

    it('returns safe: true when fs.statSync throws an error', async () => {
      // Create a config file first
      await saveConfig({ dbPath: '/test' });

      // Mock fs.statSync to throw
      const originalStatSync = fs.statSync;
      (fs as { statSync: typeof fs.statSync }).statSync = () => {
        throw new Error('Permission denied');
      };

      const result = checkConfigPermissions();

      // Restore
      (fs as { statSync: typeof fs.statSync }).statSync = originalStatSync;

      expect(result.safe).toBe(true);
      expect(result.warning).toBeNull();
    });
  });
});
