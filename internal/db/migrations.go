package db

import (
	"database/sql"
)

// migrations contains all schema migrations in order.
var migrations = []string{
	CreateReposTable,
	CreateChunksTable,
	CreateSyncStateTable,
	CreateChunkEmbeddingsTable,
	CreateIndexReposFullName,
	CreateIndexReposLanguage,
	CreateIndexReposIsStarred,
	CreateIndexChunksRepoID,
	CreateIndexReposGitHubID,
	CreateIndexRepoPushedAtHash,
	CreateIndexRepoEmbeddedHash,
}

// RunMigrations runs all schema migrations.
// Each statement is idempotent (IF NOT EXISTS) so running migrations
// multiple times is safe.
func RunMigrations(db *sql.DB) error {
	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			return err
		}
	}
	return nil
}
