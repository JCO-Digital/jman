package db

import (
	"database/sql"
	"fmt"

	"github.com/JCO-Digital/jman/internal/models"
)

// SaveSiteCore inserts or updates the installed WordPress core version for a site.
func SaveSiteCore(siteID int, version string) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `
	INSERT INTO site_core (site_id, version, updated_at)
	VALUES (?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(site_id) DO UPDATE SET
		version = excluded.version,
		updated_at = CURRENT_TIMESTAMP;
	`

	if _, err := db.Exec(query, siteID, version); err != nil {
		return fmt.Errorf("failed to save core version for site %d: %w", siteID, err)
	}

	return nil
}

// GetAllSiteCore retrieves the installed WordPress core version for every known site.
func GetAllSiteCore() ([]models.SiteCore, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT site_id, version FROM site_core`)
	if err != nil {
		return nil, fmt.Errorf("failed to query site core versions: %w", err)
	}
	defer rows.Close()

	var versions []models.SiteCore
	for rows.Next() {
		var v models.SiteCore
		if err := rows.Scan(&v.SiteID, &v.Version); err != nil {
			return nil, fmt.Errorf("failed to scan site core version: %w", err)
		}
		versions = append(versions, v)
	}

	return versions, nil
}

// GetSiteCoreLastUpdates returns a map of site IDs to their last core-version fetch timestamp.
func GetSiteCoreLastUpdates() (map[int]string, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT site_id, updated_at FROM site_core`)
	if err != nil {
		return nil, fmt.Errorf("failed to query site core updates: %w", err)
	}
	defer rows.Close()

	updates := make(map[int]string)
	for rows.Next() {
		var siteID int
		var updatedAt sql.NullString
		if err := rows.Scan(&siteID, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan site core update: %w", err)
		}
		updates[siteID] = updatedAt.String
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating site core updates: %w", err)
	}

	return updates, nil
}
