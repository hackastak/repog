package db

import (
	"database/sql"
	"strconv"
)

// GetMeta retrieves a metadata value from the repog_meta table
func GetMeta(db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRow("SELECT value FROM repog_meta WHERE key = ?", key).Scan(&value)
	return value, err
}

// SetMeta stores a metadata value in the repog_meta table
func SetMeta(db *sql.DB, key, value string) error {
	_, err := db.Exec(`
		INSERT INTO repog_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

// GetEmbeddingDimensions retrieves stored embedding dimensions, returns 0 if not set
func GetEmbeddingDimensions(db *sql.DB) (int, error) {
	val, err := GetMeta(db, "embedding_dimensions")
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(val)
}

// SetEmbeddingDimensions stores the embedding dimensions
func SetEmbeddingDimensions(db *sql.DB, dimensions int) error {
	return SetMeta(db, "embedding_dimensions", strconv.Itoa(dimensions))
}
