package db

import (
	"database/sql"
	"fmt"
)

// SaveSiteAdminUser inserts or updates the cached administrator user ID for a site.
func SaveSiteAdminUser(siteID, userID int) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `
	INSERT INTO site_admin_user (site_id, user_id, updated_at)
	VALUES (?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(site_id) DO UPDATE SET
		user_id = excluded.user_id,
		updated_at = CURRENT_TIMESTAMP;
	`

	if _, err := db.Exec(query, siteID, userID); err != nil {
		return fmt.Errorf("failed to save admin user for site %d: %w", siteID, err)
	}

	return nil
}

// GetSiteAdminUser returns the cached administrator user ID for a site, its last-fetched
// timestamp, and whether a cached value exists at all.
func GetSiteAdminUser(siteID int) (userID int, updatedAt string, found bool, err error) {
	db := GetInventoryDB()
	if db == nil {
		return 0, "", false, fmt.Errorf("database not initialized")
	}

	var ua sql.NullString
	row := db.QueryRow(`SELECT user_id, updated_at FROM site_admin_user WHERE site_id = ?`, siteID)
	if scanErr := row.Scan(&userID, &ua); scanErr != nil {
		if scanErr == sql.ErrNoRows {
			return 0, "", false, nil
		}
		return 0, "", false, fmt.Errorf("failed to get admin user for site %d: %w", siteID, scanErr)
	}

	return userID, ua.String, true, nil
}
