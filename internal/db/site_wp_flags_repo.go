package db

import (
	"fmt"
	"strconv"

	"github.com/JCO-Digital/jman/internal/models"
)

// SetSiteWpFlags inserts or updates the current WordPress config flags for a site.
// siteID can be an int or a string (UUID).
func SetSiteWpFlags(siteID any, isMultisite, disallowFileMods bool) error {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `
	INSERT INTO site_wp_flags (site_id, is_multisite, disallow_file_mods, updated_at)
	VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(site_id) DO UPDATE SET
		is_multisite = excluded.is_multisite,
		disallow_file_mods = excluded.disallow_file_mods,
		updated_at = CURRENT_TIMESTAMP;
	`

	if _, err := dbConn.Exec(query, siteID, isMultisite, disallowFileMods); err != nil {
		return fmt.Errorf("failed to set wp flags for site %v: %w", siteID, err)
	}
	return nil
}

// GetAllSiteWpFlags returns a map of numeric site ID to its current WordPress config flags.
func GetAllSiteWpFlags() (map[int]models.SiteWpFlags, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := dbConn.Query(`SELECT site_id, is_multisite, disallow_file_mods, updated_at FROM site_wp_flags`)
	if err != nil {
		return nil, fmt.Errorf("failed to query site wp flags: %w", err)
	}
	defer rows.Close()

	result := make(map[int]models.SiteWpFlags)
	for rows.Next() {
		var rawSiteID string
		var flags models.SiteWpFlags
		if err := rows.Scan(&rawSiteID, &flags.IsMultisite, &flags.DisallowFileMods, &flags.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan site wp flags: %w", err)
		}
		if id, err := strconv.Atoi(rawSiteID); err == nil {
			result[id] = flags
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating site wp flags: %w", err)
	}

	return result, nil
}

// GetAllSiteWpFlagsMap returns a map of site UUID/ID string to its current WordPress config flags.
func GetAllSiteWpFlagsMap() (map[string]models.SiteWpFlags, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := dbConn.Query(`SELECT site_id, is_multisite, disallow_file_mods, updated_at FROM site_wp_flags`)
	if err != nil {
		return nil, fmt.Errorf("failed to query site wp flags: %w", err)
	}
	defer rows.Close()

	result := make(map[string]models.SiteWpFlags)
	for rows.Next() {
		var siteID string
		var flags models.SiteWpFlags
		if err := rows.Scan(&siteID, &flags.IsMultisite, &flags.DisallowFileMods, &flags.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan site wp flags: %w", err)
		}
		result[siteID] = flags
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating site wp flags: %w", err)
	}

	return result, nil
}
