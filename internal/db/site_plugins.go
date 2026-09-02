package db

import (
	"database/sql"
	"fmt"

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
		return fmt.Errorf("failed to save site plugin %s for site %d: %w", plugin.Name, plugin.SiteID, err)
	}

	return nil
}

// GetSitePlugins retrieves all plugins installed on a specific site.
func GetSitePlugins(siteID int) ([]models.WPPlugin, error) {
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
		return nil, fmt.Errorf("failed to query plugins for site %d: %w", siteID, err)
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

	return plugins, nil
}

// DeleteSitePlugins removes all plugin records for a specific site.
// This is useful when performing a full refresh of a site's plugin list.
func DeleteSitePlugins(siteID int) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `DELETE FROM site_plugins WHERE site_id = ?`
	_, err := db.Exec(query, siteID)
	if err != nil {
		return fmt.Errorf("failed to delete plugins for site %d: %w", siteID, err)
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

	return plugins, nil
}

// GetSitesWithPlugin returns a list of site IDs where a specific plugin is installed.
func GetSitesWithPlugin(slug string) ([]int, error) {
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

	var siteIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan site id: %w", err)
		}
		siteIDs = append(siteIDs, id)
	}

	return siteIDs, nil
}

// GetSitePluginLastUpdates returns a map of site IDs to their last plugin update timestamp.
func GetSitePluginLastUpdates() (map[int]string, error) {
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

	updates := make(map[int]string)
	for rows.Next() {
		var siteID int
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
