import Database from 'better-sqlite3';
import { migrate } from './migrate.js';

let dbInstance: Database.Database | null = null;
let currentDbPath: string | null = null;

/**
 * Get or create a database connection singleton.
 * Runs migrations on first connection.
 * Loads sqlite-vec extension automatically.
 *
 * @param dbPath - Path to the SQLite database file
 * @returns The better-sqlite3 database instance
 */
export function getDb(dbPath: string): Database.Database {
  // Return existing instance if same path
  if (dbInstance && currentDbPath === dbPath) {
    return dbInstance;
  }

  // Close existing connection if different path
  if (dbInstance && currentDbPath !== dbPath) {
    dbInstance.close();
    dbInstance = null;
    currentDbPath = null;
  }

  // Create new connection
  dbInstance = new Database(dbPath);
  currentDbPath = dbPath;

  // Enable foreign keys
  dbInstance.pragma('foreign_keys = ON');

  // Performance pragmas - must match initDb() settings
  dbInstance.pragma('journal_mode = WAL');
  dbInstance.pragma('synchronous = NORMAL');
  dbInstance.pragma('cache_size = -64000');
  dbInstance.pragma('temp_store = MEMORY');
  dbInstance.pragma('mmap_size = 268435456');

  // Enable WAL mode for better concurrent access and performance
  dbInstance.pragma('journal_mode = WAL');

  // Set synchronous to NORMAL for better write performance while maintaining durability
  // NORMAL is safe in WAL mode - data is still protected against corruption
  dbInstance.pragma('synchronous = NORMAL');

  // Increase cache size to 64MB for better read performance
  // Negative value specifies size in KB (64MB = 64 * 1024 = 65536 KB)
  dbInstance.pragma('cache_size = -64000');

  // Store temp tables in memory for faster operations
  dbInstance.pragma('temp_store = MEMORY');

  // Enable memory-mapped I/O for up to 256MB for faster reads
  // This allows SQLite to read data directly from OS page cache
  dbInstance.pragma('mmap_size = 268435456');

  // Run migrations (also loads sqlite-vec extension)
  migrate(dbInstance);

  return dbInstance;
}

/**
 * Close the database connection.
 */
export function closeDb(): void {
  if (dbInstance) {
    dbInstance.close();
    dbInstance = null;
    currentDbPath = null;
  }
}

export { migrate } from './migrate.js';
export * from './schema.js';
