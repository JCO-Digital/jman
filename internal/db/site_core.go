package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// SaveSiteCore inserts or updates the installed WordPress core version for a
// site (by UUID), along with the latest available minor/major update version,
// if any (empty string means no update of that kind is available).
func SaveSiteCore(siteID, version, minorUpdate, majorUpdate string) error {
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
		return fmt.Errorf("failed to save core version for site %s: %w", siteID, err)
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
		var v models.SiteCore
		var minorUpdate, majorUpdate sql.NullString
		if err := rows.Scan(&v.SiteID, &v.Version, &minorUpdate, &majorUpdate); err != nil {
			return nil, fmt.Errorf("failed to scan site core version: %w", err)
		}
		v.MinorUpdate = minorUpdate.String
		v.MajorUpdate = majorUpdate.String
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating site core versions: %w", err)
	}

	return versions, nil
}

// GetSiteCoreLastUpdates returns a map of site UUIDs to their last core-version fetch timestamp.
func GetSiteCoreLastUpdates() (map[string]string, error) {
	db := GetInventoryDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT site_id, updated_at FROM site_core`)
	if err != nil {
		return nil, fmt.Errorf("failed to query site core updates: %w", err)
	}
	defer rows.Close()

	updates := make(map[string]string)
	for rows.Next() {
		var siteID string
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

// GetSiteCore returns a site's cached core state and when it was written,
// or nil if there is none.
func GetSiteCore(siteID string) (*models.SiteCore, time.Time, error) {
	db := GetInventoryDB()
	if db == nil {
		return nil, time.Time{}, fmt.Errorf("database not initialized")
	}
	core := models.SiteCore{SiteID: siteID}
	var minorUpdate, majorUpdate, updatedAt sql.NullString
	err := db.QueryRow(`SELECT version, minor_update, major_update, updated_at FROM site_core WHERE site_id = ?`, siteID).
		Scan(&core.Version, &minorUpdate, &majorUpdate, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("failed to get core version for site %s: %w", siteID, err)
	}
	core.MinorUpdate = minorUpdate.String
	core.MajorUpdate = majorUpdate.String
	var observedAt time.Time
	if updatedAt.Valid {
		observedAt, _ = parseInventoryTimestamp(updatedAt.String)
	}
	return &core, observedAt, nil
}

// SaveSiteCoreObserved is SaveSiteCore with an explicit observation time
// (when the state was read from the site) instead of now.
func SaveSiteCoreObserved(siteID, version, minorUpdate, majorUpdate string, observedAt time.Time) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(`
	INSERT INTO site_core (site_id, version, minor_update, major_update, updated_at)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(site_id) DO UPDATE SET
		version = excluded.version,
		minor_update = excluded.minor_update,
		major_update = excluded.major_update,
		updated_at = excluded.updated_at`,
		siteID, version, minorUpdate, majorUpdate, inventoryTimestamp(observedAt))
	if err != nil {
		return fmt.Errorf("failed to save core version for site %s: %w", siteID, err)
	}
	return nil
}
