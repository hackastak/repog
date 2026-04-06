// Package db handles database operations including schema management and migrations.
package db

import "fmt"

// Schema SQL statements

const CreateReposTable = `
CREATE TABLE IF NOT EXISTS repos (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    github_id       INTEGER UNIQUE NOT NULL,
    owner           TEXT NOT NULL,
    name            TEXT NOT NULL,
    full_name       TEXT UNIQUE NOT NULL,
    description     TEXT,
    language        TEXT,
    stars           INTEGER DEFAULT 0,
    forks           INTEGER DEFAULT 0,
    is_private      INTEGER DEFAULT 0,
    is_starred      INTEGER DEFAULT 0,
    is_owned        INTEGER DEFAULT 0,
    is_archived     INTEGER DEFAULT 0,
    is_fork         INTEGER DEFAULT 0,
    pushed_at       TEXT,
    pushed_at_hash  TEXT,
    embedded_hash   TEXT,
    topics          TEXT,
    html_url        TEXT,
    default_branch  TEXT,
    synced_at       TEXT,
    embedded_at     TEXT
)`

const CreateChunksTable = `
CREATE TABLE IF NOT EXISTS chunks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id     INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    chunk_type  TEXT NOT NULL CHECK(chunk_type IN ('metadata', 'readme', 'file_tree')),
    content     TEXT NOT NULL,
    created_at  TEXT DEFAULT (datetime('now')),
    UNIQUE(repo_id, chunk_type)
)`

const CreateSyncStateTable = `
CREATE TABLE IF NOT EXISTS sync_state (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id        INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    status         TEXT NOT NULL CHECK(status IN ('completed', 'failed')),
    last_synced_at TEXT,
    error_message  TEXT,
    UNIQUE(repo_id)
)`

// CreateMetaTable creates the metadata table for storing configuration
const CreateMetaTable = `
CREATE TABLE IF NOT EXISTS repog_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
)`

// CreateChunkEmbeddingsTableSQL generates the embeddings table with dynamic dimensions
func CreateChunkEmbeddingsTableSQL(dimensions int) string {
	return fmt.Sprintf(`
CREATE VIRTUAL TABLE IF NOT EXISTS chunk_embeddings USING vec0(
    chunk_id  INTEGER PRIMARY KEY,
    embedding FLOAT[%d]
)`, dimensions)
}

// Index definitions
const CreateIndexReposFullName = `CREATE INDEX IF NOT EXISTS idx_repos_full_name ON repos(full_name)`
const CreateIndexReposLanguage = `CREATE INDEX IF NOT EXISTS idx_repos_language ON repos(language)`
const CreateIndexReposIsStarred = `CREATE INDEX IF NOT EXISTS idx_repos_is_starred ON repos(is_starred)`
const CreateIndexChunksRepoID = `CREATE INDEX IF NOT EXISTS idx_chunks_repo_id ON chunks(repo_id)`
const CreateIndexReposGitHubID = `CREATE INDEX IF NOT EXISTS idx_repos_github_id ON repos(github_id)`
const CreateIndexRepoPushedAtHash = `CREATE INDEX IF NOT EXISTS idx_repos_pushed_at_hash ON repos(pushed_at_hash)`
const CreateIndexRepoEmbeddedHash = `CREATE INDEX IF NOT EXISTS idx_repos_embedded_hash ON repos(embedded_hash)`
