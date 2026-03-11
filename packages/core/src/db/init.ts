import Database from 'better-sqlite3';
import path from 'path';
import fs from 'fs';
import * as sqliteVec from 'sqlite-vec';
import { ALL_SCHEMA_STATEMENTS, VECTOR_SCHEMA_STATEMENTS } from './schema.js';

/**
 * Result of database initialization.
 */
export interface InitDbResult {
  success: boolean;
  dbPath: string;
  walEnabled: boolean;
  tablesCreated: string[];
  error?: string;
}

/**
 * Options for database initialization.
 */
export interface InitDbOptions {
  /**
   * Enable WAL mode for better concurrent access.
   * @default true
   */
  enableWal?: boolean;

  /**
   * Create parent directories if they don't exist.
   * @default true
   */
  createDirs?: boolean;
}

/**
 * Initialize the RepoG SQLite database.
 *
 * Creates all required tables and indexes if they don't exist.
 * Enables WAL mode by default for better performance and concurrent access.
 * This operation is idempotent - safe to call multiple times.
 *
 * @param dbPath - Path to the SQLite database file (use ':memory:' for in-memory)
 * @param options - Initialization options
 * @returns Result indicating success and what was created
 */
export function initDb(dbPath: string, options: InitDbOptions = {}): InitDbResult {
  const { enableWal = true, createDirs = true } = options;

  const tablesCreated: string[] = [];
  let walEnabled = false;

  try {
    // Create parent directories if needed (skip for :memory: databases)
    if (dbPath !== ':memory:' && createDirs) {
      const dir = path.dirname(dbPath);
      fs.mkdirSync(dir, { recursive: true });
    }

    // Open database connection
    const db = new Database(dbPath);

    try {
      // Load sqlite-vec extension (must be before any SQL execution)
      sqliteVec.load(db);

      // Enable foreign keys
      db.pragma('foreign_keys = ON');

      // Enable WAL mode if requested
      if (enableWal) {
        // WAL mode must be set first before other pragmas
        db.pragma('journal_mode = WAL');
        const mode = db.pragma('journal_mode', { simple: true }) as string;
        walEnabled = mode.toLowerCase() === 'wal';

        // Set synchronous to NORMAL for better write performance while maintaining durability
        // NORMAL is safe in WAL mode - data is still protected against corruption
        db.pragma('synchronous = NORMAL');

        // Increase cache size to 64MB for better read performance
        // Negative value specifies size in KB (64MB = 64 * 1024 = 65536 KB)
        db.pragma('cache_size = -64000');

        // Store temp tables in memory for faster operations
        db.pragma('temp_store = MEMORY');

        // Enable memory-mapped I/O for up to 256MB for faster reads
        // This allows SQLite to read data directly from OS page cache
        db.pragma('mmap_size = 268435456');
      }

      // Run all schema statements in a transaction
      db.transaction(() => {
        for (const statement of ALL_SCHEMA_STATEMENTS) {
          db.exec(statement);

          // Extract table name from CREATE TABLE statements
          const tableMatch = statement.match(/CREATE TABLE IF NOT EXISTS (\w+)/i);
          if (tableMatch) {
            tablesCreated.push(tableMatch[1]);
          }
        }

        // Run vector schema statements (sqlite-vec extension required)
        for (const statement of VECTOR_SCHEMA_STATEMENTS) {
          db.exec(statement);

          // Extract virtual table name from CREATE VIRTUAL TABLE statements
          const virtualTableMatch = statement.match(/CREATE VIRTUAL TABLE IF NOT EXISTS (\w+)/i);
          if (virtualTableMatch) {
            tablesCreated.push(virtualTableMatch[1]);
          }
        }
      })();

      return {
        success: true,
        dbPath,
        walEnabled,
        tablesCreated,
      };
    } finally {
      // Always close the connection
      db.close();
    }
  } catch (error) {
    return {
      success: false,
      dbPath,
      walEnabled,
      tablesCreated,
      error: error instanceof Error ? error.message : 'Unknown error initializing database',
    };
  }
}

/**
 * Check if WAL mode is enabled for a database.
 *
 * @param dbPath - Path to the SQLite database file
 * @returns True if WAL mode is enabled
 */
export function isWalEnabled(dbPath: string): boolean {
  try {
    const db = new Database(dbPath, { readonly: true });
    try {
      // Load sqlite-vec in case the database has virtual tables
      sqliteVec.load(db);
      const mode = db.pragma('journal_mode', { simple: true }) as string;
      return mode.toLowerCase() === 'wal';
    } finally {
      db.close();
    }
  } catch {
    return false;
  }
}

/**
 * Get a list of tables in the database.
 *
 * @param dbPath - Path to the SQLite database file
 * @returns Array of table names
 */
export function getTableList(dbPath: string): string[] {
  try {
    const db = new Database(dbPath, { readonly: true });
    try {
      // Load sqlite-vec in case the database has virtual tables
      sqliteVec.load(db);
      const result = db
        .prepare("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
        .all() as { name: string }[];
      return result.map((row) => row.name);
    } finally {
      db.close();
    }
  } catch {
    return [];
  }
}
