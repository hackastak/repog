package db

import (
	"database/sql"
)

// RunMigrations runs all schema migrations with the specified embedding dimensions.
// Each statement is idempotent (IF NOT EXISTS) so running migrations multiple times is safe.
// If the embedding dimensions change from what's stored, a migration is required.
func RunMigrations(db *sql.DB, embeddingDimensions int) error {
	// Create meta table first (needed to track dimensions)
	if _, err := db.Exec(CreateMetaTable); err != nil {
		return err
	}

	// Create standard tables
	standardTables := []string{
		CreateReposTable,
		CreateChunksTable,
		CreateSyncStateTable,
	}

	for _, migration := range standardTables {
		if _, err := db.Exec(migration); err != nil {
			return err
		}
	}

	// Handle embeddings table with dimension checking
	storedDims, err := GetEmbeddingDimensions(db)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if storedDims == 0 {
		// First time, create embeddings table with provided dimensions
		if _, err := db.Exec(CreateChunkEmbeddingsTableSQL(embeddingDimensions)); err != nil {
			return err
		}
		if err := SetEmbeddingDimensions(db, embeddingDimensions); err != nil {
			return err
		}
	} else if storedDims != embeddingDimensions {
		// Dimensions changed - this should only happen via explicit reconfiguration
		// The migration is handled by MigrateEmbeddingDimensions, not here
		// Just ensure the table exists with stored dimensions for now
		if _, err := db.Exec(CreateChunkEmbeddingsTableSQL(storedDims)); err != nil {
			return err
		}
	} else {
		// Dimensions match, ensure table exists
		if _, err := db.Exec(CreateChunkEmbeddingsTableSQL(embeddingDimensions)); err != nil {
			return err
		}
	}

	// Create indexes
	indexes := []string{
		CreateIndexReposFullName,
		CreateIndexReposLanguage,
		CreateIndexReposIsStarred,
		CreateIndexChunksRepoID,
		CreateIndexReposGitHubID,
		CreateIndexRepoPushedAtHash,
		CreateIndexRepoEmbeddedHash,
	}

	for _, index := range indexes {
		if _, err := db.Exec(index); err != nil {
			return err
		}
	}

	return nil
}
