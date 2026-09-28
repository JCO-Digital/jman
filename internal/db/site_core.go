package db

import (
	"database/sql"
	"fmt"
	"strconv"

	"github.com/JCO-Digital/jman/internal/models"
)

// SaveSiteCore inserts or updates the installed WordPress core version for a
// site, along with the latest available minor/major update version, if any
// (empty string means no update of that kind is available).
// siteID can be an int or a string (UUID).
func SaveSiteCore(siteID any, version, minorUpdate, majorUpdate string) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `
	INSERT INTO site_core (site_id, version, minor_update, major_update, updated_at)
	VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(site_id) DO UPDATE SET
		version = excluded.version,
		minor_update = excluded.minor_update,
		major_update = excluded.major_update,
		updated_at = CURRENT_TIMESTAMP;
	`

	if _, err := db.Exec(query, siteID, version, minorUpdate, majorUpdate); err != nil {
		return fmt.Errorf("failed to save core version for site %v: %w", siteID, err)
	}

	return nil
}

// GetAllSiteCore retrieves the installed WordPress core version, and any
// available minor/major update, for every known site.
func GetAllSiteCore() ([]models.SiteCore, error) {
	db := GetInventoryDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT site_id, version, minor_update, major_update FROM site_core`)
	if err != nil {
		return nil, fmt.Errorf("failed to query site core versions: %w", err)
	}
	defer rows.Close()

	var versions []models.SiteCore
	for rows.Next() {
		var rawSiteID string
		var v models.SiteCore
		var minorUpdate, majorUpdate sql.NullString
		if err := rows.Scan(&rawSiteID, &v.Version, &minorUpdate, &majorUpdate); err != nil {
			return nil, fmt.Errorf("failed to scan site core version: %w", err)
		}
		if id, err := strconv.Atoi(rawSiteID); err == nil {
			v.SiteID = id
		}
		v.SiteUUID = rawSiteID
		v.MinorUpdate = minorUpdate.String
		v.MajorUpdate = majorUpdate.String
		versions = append(versions, v)
	}

	return versions, nil
}

// GetSiteCoreLastUpdates returns a map of site IDs to their last core-version fetch timestamp.
func GetSiteCoreLastUpdates() (map[int]string, error) {
	db := GetInventoryDB()
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
		var rawSiteID string
		var updatedAt sql.NullString
		if err := rows.Scan(&rawSiteID, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan site core update: %w", err)
		}
		if id, err := strconv.Atoi(rawSiteID); err == nil {
			updates[id] = updatedAt.String
		}
	}

	return updates, nil
}
