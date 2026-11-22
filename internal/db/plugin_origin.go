package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// GetPluginOrigin returns whether wordpress.org had a plugin slug when it
// was last checked, and when that was. found is false for a slug never
// checked.
func GetPluginOrigin(slug string) (onWPOrg bool, checkedAt time.Time, found bool, err error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return false, time.Time{}, false, fmt.Errorf("database not initialized")
	}
	err = dbConn.QueryRow(`SELECT on_wporg, checked_at FROM plugin_origins WHERE slug = ?`, slug).Scan(&onWPOrg, &checkedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, time.Time{}, false, nil
	}
	if err != nil {
		return false, time.Time{}, false, fmt.Errorf("failed to read the origin of plugin %s: %w", slug, err)
	}
	return onWPOrg, checkedAt, true, nil
}

// SavePluginOrigin records whether wordpress.org has a plugin slug.
func SavePluginOrigin(slug string, onWPOrg bool, checkedAt time.Time) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}
	if _, err := dbConn.Exec(`
	INSERT INTO plugin_origins (slug, on_wporg, checked_at) VALUES (?, ?, ?)
	ON CONFLICT(slug) DO UPDATE SET on_wporg = excluded.on_wporg, checked_at = excluded.checked_at`,
		slug, onWPOrg, checkedAt); err != nil {
		return fmt.Errorf("failed to save the origin of plugin %s: %w", slug, err)
	}
	return nil
}
