package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// GetReleaseChecksums returns the cached checksums of a WordPress or plugin
// release as JSON, when they were fetched, and whether they're cached at
// all. Empty data with found set means wordpress.org had none.
func GetReleaseChecksums(kind, name, version string) (string, time.Time, bool, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return "", time.Time{}, false, fmt.Errorf("database not initialized")
	}
	var data string
	var fetchedAt time.Time
	err := dbConn.QueryRow(`SELECT data, fetched_at FROM release_checksums WHERE kind = ? AND name = ? AND version = ?`, kind, name, version).Scan(&data, &fetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, fmt.Errorf("failed to read %s checksums for %s %s: %w", kind, name, version, err)
	}
	return data, fetchedAt, true, nil
}

// SaveReleaseChecksums caches the checksums of a release as JSON; empty
// data records that wordpress.org had none.
func SaveReleaseChecksums(kind, name, version, data string, fetchedAt time.Time) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := dbConn.Exec(`
	INSERT INTO release_checksums (kind, name, version, data, fetched_at) VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(kind, name, version) DO UPDATE SET data = excluded.data, fetched_at = excluded.fetched_at`, kind, name, version, data, fetchedAt)
	if err != nil {
		return fmt.Errorf("failed to cache %s checksums for %s %s: %w", kind, name, version, err)
	}
	return nil
}
