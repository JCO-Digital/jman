package db

import (
	"database/sql"
	"fmt"

	"github.com/JCO-Digital/jman/internal/models"
)

// SetSiteWpFlags inserts or updates the current WordPress config flags for a
// site (by UUID). A nil autoUpdateCore stores the core auto-update setting
// as unknown.
func SetSiteWpFlags(siteID string, isMultisite, disallowFileMods bool, autoUpdateCore *string) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `
	INSERT INTO site_wp_flags (site_id, is_multisite, disallow_file_mods, auto_update_core, updated_at)
	VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(site_id) DO UPDATE SET
		is_multisite = excluded.is_multisite,
		disallow_file_mods = excluded.disallow_file_mods,
		auto_update_core = excluded.auto_update_core,
		updated_at = CURRENT_TIMESTAMP;
	`

	var autoUpdate sql.NullString
	if autoUpdateCore != nil {
		autoUpdate = sql.NullString{String: *autoUpdateCore, Valid: true}
	}
	if _, err := dbConn.Exec(query, siteID, isMultisite, disallowFileMods, autoUpdate); err != nil {
		return fmt.Errorf("failed to set wp flags for site %s: %w", siteID, err)
	}
	return nil
}

// GetAllSiteWpFlags returns a map of site UUID to its current WordPress config flags.
func GetAllSiteWpFlags() (map[string]models.SiteWpFlags, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := dbConn.Query(`SELECT site_id, is_multisite, disallow_file_mods, auto_update_core, updated_at FROM site_wp_flags`)
	if err != nil {
		return nil, fmt.Errorf("failed to query site wp flags: %w", err)
	}
	defer rows.Close()

	result := make(map[string]models.SiteWpFlags)
	for rows.Next() {
		var siteID string
		var flags models.SiteWpFlags
		var autoUpdate sql.NullString
		if err := rows.Scan(&siteID, &flags.IsMultisite, &flags.DisallowFileMods, &autoUpdate, &flags.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan site wp flags: %w", err)
		}
		flags.AutoUpdateCore = autoUpdate.String
		result[siteID] = flags
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating site wp flags: %w", err)
	}

	return result, nil
}
