package db

import (
	"database/sql"
	"fmt"

	"github.com/JCO-Digital/jman/internal/models"
)

// SaveSiteUpdateLedgerEntry inserts a new update ledger entry for a site.
func SaveSiteUpdateLedgerEntry(entry *models.SiteUpdateLedgerEntry) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `
	INSERT INTO site_update_ledger (
		site_id, update_type, status, data_json, updated_by, updated_at
	) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`

	_, err := db.Exec(query,
		entry.SiteID,
		entry.UpdateType,
		entry.Status,
		entry.DataJSON,
		entry.UpdatedBy,
	)

	if err != nil {
		return fmt.Errorf("failed to save site update ledger entry for site %d: %w", entry.SiteID, err)
	}

	return nil
}

// GetSiteUpdateLedger retrieves all update ledger entries for a specific site, sorted by newest first.
func GetSiteUpdateLedger(siteID int) ([]models.SiteUpdateLedgerEntry, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT id, site_id, update_type, status, data_json, updated_by, updated_at
	FROM site_update_ledger
	WHERE site_id = ?
	ORDER BY updated_at DESC
	`

	rows, err := db.Query(query, siteID)
	if err != nil {
		return nil, fmt.Errorf("failed to query update ledger for site %d: %w", siteID, err)
	}
	defer rows.Close()

	entries := []models.SiteUpdateLedgerEntry{}
	for rows.Next() {
		var e models.SiteUpdateLedgerEntry
		var dataJSON sql.NullString
		err := rows.Scan(
			&e.ID,
			&e.SiteID,
			&e.UpdateType,
			&e.Status,
			&dataJSON,
			&e.UpdatedBy,
			&e.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site update ledger entry: %w", err)
		}
		if dataJSON.Valid {
			e.DataJSON = dataJSON.String
		}
		entries = append(entries, e)
	}

	return entries, nil
}

// GetLatestSiteUpdateLedgerEntry retrieves the most recent update ledger entry for a specific site.
func GetLatestSiteUpdateLedgerEntry(siteID int) (*models.SiteUpdateLedgerEntry, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT id, site_id, update_type, status, data_json, updated_by, updated_at
	FROM site_update_ledger
	WHERE site_id = ?
	ORDER BY updated_at DESC
	LIMIT 1
	`

	var e models.SiteUpdateLedgerEntry
	var dataJSON sql.NullString
	err := db.QueryRow(query, siteID).Scan(
		&e.ID,
		&e.SiteID,
		&e.UpdateType,
		&e.Status,
		&dataJSON,
		&e.UpdatedBy,
		&e.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan latest site update ledger entry: %w", err)
	}
	if dataJSON.Valid {
		e.DataJSON = dataJSON.String
	}

	return &e, nil
}

// GetLatestSiteUpdateLedgerEntries retrieves the most recent update ledger entry for all sites,
// returned as a map of site_id -> entry.
func GetLatestSiteUpdateLedgerEntries() (map[int]models.SiteUpdateLedgerEntry, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT t1.id, t1.site_id, t1.update_type, t1.status, t1.data_json, t1.updated_by, t1.updated_at
	FROM site_update_ledger t1
	INNER JOIN (
		SELECT site_id, MAX(id) as max_id
		FROM site_update_ledger
		GROUP BY site_id
	) t2 ON t1.id = t2.max_id
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query latest site update ledger entries: %w", err)
	}
	defer rows.Close()

	entries := make(map[int]models.SiteUpdateLedgerEntry)
	for rows.Next() {
		var e models.SiteUpdateLedgerEntry
		var dataJSON sql.NullString
		err := rows.Scan(
			&e.ID,
			&e.SiteID,
			&e.UpdateType,
			&e.Status,
			&dataJSON,
			&e.UpdatedBy,
			&e.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site update ledger entry: %w", err)
		}
		if dataJSON.Valid {
			e.DataJSON = dataJSON.String
		}
		entries[e.SiteID] = e
	}

	return entries, nil
}
