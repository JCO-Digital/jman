package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// SaveSitePlugin inserts or updates a plugin record for a specific site.
func SaveSitePlugin(plugin models.WPPlugin) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `
	INSERT INTO site_plugins (
		site_id, slug, status, version, update_available, auto_update, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(site_id, slug) DO UPDATE SET
		status = excluded.status,
		version = excluded.version,
		update_available = excluded.update_available,
		auto_update = excluded.auto_update,
		updated_at = CURRENT_TIMESTAMP;
	`

	_, err := db.Exec(query,
		plugin.SiteID,
		plugin.Name, // WPPlugin.Name is used as the slug
		plugin.Status,
		plugin.Version,
		plugin.Update,
		plugin.AutoUpdate,
	)

	if err != nil {
		return fmt.Errorf("failed to save site plugin %s for site %s: %w", plugin.Name, plugin.SiteID, err)
	}

	return nil
}

// GetSitePlugins retrieves all plugins installed on a specific site (by UUID).
func GetSitePlugins(siteID string) ([]models.WPPlugin, error) {
	db := GetInventoryDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT site_id, slug, status, version, update_available, auto_update
	FROM site_plugins
	WHERE site_id = ?
	`

	rows, err := db.Query(query, siteID)
	if err != nil {
		return nil, fmt.Errorf("failed to query plugins for site %s: %w", siteID, err)
	}
	defer rows.Close()

	var plugins []models.WPPlugin
	for rows.Next() {
		var p models.WPPlugin
		err := rows.Scan(
			&p.SiteID,
			&p.Name,
			&p.Status,
			&p.Version,
			&p.Update,
			&p.AutoUpdate,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site plugin: %w", err)
		}
		plugins = append(plugins, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating site plugins: %w", err)
	}

	return plugins, nil
}

// DeleteSitePlugins removes all plugin records for a specific site (by UUID).
func DeleteSitePlugins(siteID string) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `DELETE FROM site_plugins WHERE site_id = ?`
	_, err := db.Exec(query, siteID)
	if err != nil {
		return fmt.Errorf("failed to delete plugins for site %s: %w", siteID, err)
	}

	return nil
}

// GetAllSitePlugins retrieves every plugin instance across all sites.
func GetAllSitePlugins() ([]models.WPPlugin, error) {
	db := GetInventoryDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT site_id, slug, status, version, update_available, auto_update FROM site_plugins`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all site plugins: %w", err)
	}
	defer rows.Close()

	var plugins []models.WPPlugin
	for rows.Next() {
		var p models.WPPlugin
		err := rows.Scan(
			&p.SiteID,
			&p.Name,
			&p.Status,
			&p.Version,
			&p.Update,
			&p.AutoUpdate,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site plugin: %w", err)
		}
		plugins = append(plugins, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating all site plugins: %w", err)
	}

	return plugins, nil
}

// GetSitesWithPlugin returns the UUIDs of sites where a specific plugin is installed.
func GetSitesWithPlugin(slug string) ([]string, error) {
	db := GetInventoryDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT site_id FROM site_plugins WHERE slug = ? AND status NOT IN ('must-use', 'dropin')`
	rows, err := db.Query(query, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to query sites for plugin %s: %w", slug, err)
	}
	defer rows.Close()

	var siteIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan site id: %w", err)
		}
		siteIDs = append(siteIDs, id)
	}

	return siteIDs, nil
}

// GetSitePluginLastUpdates returns a map of site UUIDs to their last plugin update timestamp.
func GetSitePluginLastUpdates() (map[string]string, error) {
	db := GetInventoryDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT site_id, MAX(updated_at) FROM site_plugins GROUP BY site_id`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query plugin updates: %w", err)
	}
	defer rows.Close()

	updates := make(map[string]string)
	for rows.Next() {
		var siteID string
		var updatedAt sql.NullString
		if err := rows.Scan(&siteID, &updatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan plugin update: %w", err)
		}
		updates[siteID] = updatedAt.String
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating plugin updates: %w", err)
	}

	return updates, nil
}

// inventoryTimestamp formats t the way SQLite's CURRENT_TIMESTAMP does, so
// rows written with an explicit time sort and parse like the rest.
func inventoryTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}

// parseInventoryTimestamp parses a CURRENT_TIMESTAMP-style or RFC3339
// timestamp.
func parseInventoryTimestamp(value string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.ParseInLocation("2006-01-02 15:04:05", value, time.UTC)
}

// ReplaceSitePlugins replaces a site's plugin rows with plugins in one
// transaction, stamping them with observedAt (when the list was read from
// the site), so a reader never sees a half-replaced list.
func ReplaceSitePlugins(siteID string, plugins []models.WPPlugin, observedAt time.Time) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to replace plugins for site %s: %w", siteID, err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM site_plugins WHERE site_id = ?`, siteID); err != nil {
		return fmt.Errorf("failed to clear plugins for site %s: %w", siteID, err)
	}
	stamp := inventoryTimestamp(observedAt)
	for _, p := range plugins {
		if _, err := tx.Exec(
			`INSERT INTO site_plugins (site_id, slug, status, version, update_available, auto_update, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(site_id, slug) DO UPDATE SET
				status = excluded.status,
				version = excluded.version,
				update_available = excluded.update_available,
				auto_update = excluded.auto_update,
				updated_at = excluded.updated_at`,
			siteID, p.Name, p.Status, p.Version, p.Update, p.AutoUpdate, stamp,
		); err != nil {
			return fmt.Errorf("failed to save plugin %s for site %s: %w", p.Name, siteID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to replace plugins for site %s: %w", siteID, err)
	}
	return nil
}

// GetSitePluginsObservedAt returns when a site's cached plugin list was
// last written, and false if the site has no cached plugins.
func GetSitePluginsObservedAt(siteID string) (time.Time, bool, error) {
	db := GetInventoryDB()
	if db == nil {
		return time.Time{}, false, fmt.Errorf("database not initialized")
	}
	var updatedAt sql.NullString
	if err := db.QueryRow(`SELECT MAX(updated_at) FROM site_plugins WHERE site_id = ?`, siteID).Scan(&updatedAt); err != nil {
		return time.Time{}, false, fmt.Errorf("failed to read plugin cache time for site %s: %w", siteID, err)
	}
	if !updatedAt.Valid || updatedAt.String == "" {
		return time.Time{}, false, nil
	}
	t, err := parseInventoryTimestamp(updatedAt.String)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid plugin cache time %q for site %s: %w", updatedAt.String, siteID, err)
	}
	return t, true, nil
}
