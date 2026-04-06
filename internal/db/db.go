package db

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

func init() {
	// Register the sqlite-vec extension
	sqlite_vec.Auto()
}

// Open opens (or creates) the SQLite database at the given path, applies all
// pragmas, loads the sqlite-vec extension, and runs schema migrations.
// The caller is responsible for calling Close().
// embeddingDimensions specifies the vector size for embeddings (e.g., 768 for Gemini).
func Open(path string, embeddingDimensions int) (*sql.DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	// Open database with sqlite-vec support
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	// Set pragmas in order
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=-64000",
		"PRAGMA temp_store=MEMORY",
		"PRAGMA mmap_size=268435456",
		"PRAGMA foreign_keys=ON",
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	// Run migrations
	if err := RunMigrations(db, embeddingDimensions); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

// Close closes the database connection.
func Close(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return db.Close()
}

// IsWALEnabled checks if WAL mode is enabled for the database.
func IsWALEnabled(db *sql.DB) (bool, error) {
	var mode string
	err := db.QueryRow("PRAGMA journal_mode").Scan(&mode)
	if err != nil {
		return false, err
	}
	return mode == "wal", nil
}

// VecVersion returns the sqlite-vec version to verify it's loaded.
func VecVersion(db *sql.DB) (string, error) {
	var version string
	err := db.QueryRow("SELECT vec_version()").Scan(&version)
	if err != nil {
		return "", err
	}
	return version, nil
}

// MigrateEmbeddingDimensions drops and recreates the embeddings table with new dimensions.
// This is required when changing embedding models with different vector sizes.
// All existing embeddings will be lost and repos will need to be re-embedded.
func MigrateEmbeddingDimensions(db *sql.DB, newDimensions int) error {
	// Drop existing embeddings table
	if _, err := db.Exec("DROP TABLE IF EXISTS chunk_embeddings"); err != nil {
		return err
	}

	// Create with new dimensions
	if _, err := db.Exec(CreateChunkEmbeddingsTableSQL(newDimensions)); err != nil {
		return err
	}

	// Update stored dimensions
	if err := SetEmbeddingDimensions(db, newDimensions); err != nil {
		return err
	}

	// Clear embedded_hash on all repos to trigger re-embedding
	if _, err := db.Exec("UPDATE repos SET embedded_hash = NULL, embedded_at = NULL"); err != nil {
		return err
	}

	return nil
}
