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
func Open(path string) (*sql.DB, error) {
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
	if err := RunMigrations(db); err != nil {
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
