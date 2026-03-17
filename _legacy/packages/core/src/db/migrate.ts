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

  // Migration: If chunk_embeddings has the old 'chunk_id' column name, drop it
  // This is required because sqlite-vec's vec0 prefers 'rowid' as the primary key name
  try {
    const tableInfo = db.prepare("PRAGMA table_info('chunk_embeddings')").all() as Array<{ name: string }>;
    if (tableInfo.length > 0 && tableInfo.some(col => col.name === 'chunk_id')) {
      db.exec('DROP TABLE chunk_embeddings');
    }
  } catch {
    // Table might not exist yet, ignore
  }

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
