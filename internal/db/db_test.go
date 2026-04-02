package db

import (
	"path/filepath"
	"testing"
)

func TestOpenCreatesAllTables(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = Close(db) }()

	// Check that repos table exists
	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='repos'").Scan(&name)
	if err != nil {
		t.Errorf("repos table not created: %v", err)
	}

	// Check that chunks table exists
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='chunks'").Scan(&name)
	if err != nil {
		t.Errorf("chunks table not created: %v", err)
	}

	// Check that sync_state table exists
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='sync_state'").Scan(&name)
	if err != nil {
		t.Errorf("sync_state table not created: %v", err)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Open first time
	db1, err := Open(dbPath, 768)
	if err != nil {
		t.Fatalf("First Open failed: %v", err)
	}
	_ = Close(db1)

	// Open second time - should not error
	db2, err := Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Second Open failed: %v", err)
	}
	defer func() { _ = Close(db2) }()
}

func TestWALModeEnabled(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = Close(db) }()

	walEnabled, err := IsWALEnabled(db)
	if err != nil {
		t.Fatalf("IsWALEnabled failed: %v", err)
	}

	if !walEnabled {
		t.Error("WAL mode should be enabled")
	}
}

func TestVecVersionAvailable(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = Close(db) }()

	version, err := VecVersion(db)
	if err != nil {
		t.Fatalf("VecVersion failed: %v", err)
	}

	if version == "" {
		t.Error("sqlite-vec version should not be empty")
	}
}

func TestForeignKeyEnforcement(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath, 768)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = Close(db) }()

	// Try to insert a chunk with non-existent repo_id
	_, err = db.Exec(`INSERT INTO chunks (repo_id, chunk_type, content) VALUES (99999, 'metadata', 'test')`)
	if err == nil {
		t.Error("Foreign key constraint should have failed for non-existent repo_id")
	}
}
