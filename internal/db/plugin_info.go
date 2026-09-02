package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// SavePluginInfo inserts or updates a plugin's metadata in the database.
func SavePluginInfo(info models.PluginInfo) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `
	INSERT INTO plugin_info (
		slug, name, version, author, author_profile, requires, tested, last_updated, homepage, fetched_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(slug) DO UPDATE SET
		name = excluded.name,
		version = excluded.version,
		author = excluded.author,
		author_profile = excluded.author_profile,
		requires = excluded.requires,
		tested = excluded.tested,
		last_updated = excluded.last_updated,
		homepage = excluded.homepage,
		fetched_at = CURRENT_TIMESTAMP;
	`

	_, err := db.Exec(query,
		info.Slug,
		info.Name,
		info.Version,
		info.Author,
		info.AuthorProfile,
		info.Requires,
		info.Tested,
		info.LastUpdated,
		info.Homepage,
	)

	if err != nil {
		return fmt.Errorf("failed to save plugin info for %s: %w", info.Slug, err)
	}

	return nil
}

// GetPluginInfo retrieves a plugin's metadata and its last fetch timestamp from the database.
func GetPluginInfo(slug string) (*models.PluginInfo, time.Time, error) {
	db := GetInventoryDB()
	if db == nil {
		return nil, time.Time{}, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT
		slug, name, version, author, author_profile, requires, tested, last_updated, homepage, fetched_at
	FROM plugin_info
	WHERE slug = ?
	`

	var info models.PluginInfo
	var fetchedAt time.Time

	err := db.QueryRow(query, slug).Scan(
		&info.Slug,
		&info.Name,
		&info.Version,
		&info.Author,
		&info.AuthorProfile,
		&info.Requires,
		&info.Tested,
		&info.LastUpdated,
		&info.Homepage,
		&fetchedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, time.Time{}, nil
		}
		return nil, time.Time{}, fmt.Errorf("failed to get plugin info for %s: %w", slug, err)
	}

	return &info, fetchedAt, nil
}

// DeletePluginInfo removes a plugin's metadata from the database.
func DeletePluginInfo(slug string) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `DELETE FROM plugin_info WHERE slug = ?`
	_, err := db.Exec(query, slug)
	if err != nil {
		return fmt.Errorf("failed to delete plugin info for %s: %w", slug, err)
	}

	return nil
}

// GetAllPluginSlugs returns a list of all plugin slugs currently in the database.
func GetAllPluginSlugs() ([]string, error) {
	db := GetInventoryDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT slug FROM plugin_info`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query plugin slugs: %w", err)
	}
	defer rows.Close()

	slugs := []string{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, fmt.Errorf("failed to scan slug: %w", err)
		}
		slugs = append(slugs, slug)
	}

	return slugs, nil
}

// GetAllPluginInfo returns a list of all plugin metadata stored in the database.
func GetAllPluginInfo() ([]models.PluginInfo, error) {
	db := GetInventoryDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT
		slug, name, version, author, author_profile, requires, tested, last_updated, homepage
	FROM plugin_info
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all plugin info: %w", err)
	}
	defer rows.Close()

	plugins := []models.PluginInfo{}
	for rows.Next() {
		var info models.PluginInfo
		err := rows.Scan(
			&info.Slug,
			&info.Name,
			&info.Version,
			&info.Author,
			&info.AuthorProfile,
			&info.Requires,
			&info.Tested,
			&info.LastUpdated,
			&info.Homepage,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan plugin info: %w", err)
		}
		plugins = append(plugins, info)
	}

	return plugins, nil
}
