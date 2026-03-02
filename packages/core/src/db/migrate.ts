import type Database from 'better-sqlite3';
import * as sqliteVec from 'sqlite-vec';
import { ALL_SCHEMA_STATEMENTS, VECTOR_SCHEMA_STATEMENTS } from './schema.js';

/**
 * Run database schema migrations.
 * Creates all tables and indexes if they don't exist.
 *
 * @param db - The better-sqlite3 database instance
 */
export function migrate(db: Database.Database): void {
  // Load sqlite-vec extension first
  sqliteVec.load(db);

  // Run all standard schema statements in a transaction
  db.transaction(() => {
    for (const statement of ALL_SCHEMA_STATEMENTS) {
      db.exec(statement);
    }

    // Run vector schema statements (sqlite-vec extension is now loaded)
    for (const statement of VECTOR_SCHEMA_STATEMENTS) {
      db.exec(statement);
    }
  })();

  // Add embedded_hash column if it doesn't exist (for existing databases)
  try {
    db.exec('ALTER TABLE repos ADD COLUMN embedded_hash TEXT');
  } catch {
    // Column already exists, ignore
  }
}
