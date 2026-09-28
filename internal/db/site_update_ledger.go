package db

import (
	"database/sql"
	"fmt"
	"strconv"

	"github.com/JCO-Digital/jman/internal/models"
)

// scanUpdateLedgerRow safely scans a site_update_ledger row where site_id can be int or UUID string.
func scanUpdateLedgerRow(scanner interface{ Scan(dest ...any) error }) (*models.SiteUpdateLedgerEntry, error) {
	var e models.SiteUpdateLedgerEntry
	var rawSiteID string
	var dataJSON sql.NullString
	err := scanner.Scan(
		&e.ID,
		&rawSiteID,
		&e.UpdateType,
		&e.Status,
		&dataJSON,
		&e.UpdatedBy,
		&e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if id, err := strconv.Atoi(rawSiteID); err == nil {
		e.SiteID = id
	} else {
		e.SiteID = rawSiteID
	}
	e.SiteUUID = rawSiteID
	if dataJSON.Valid {
		e.DataJSON = dataJSON.String
	}
	return &e, nil
}

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

	siteIdent := entry.SiteID
	if entry.SiteUUID != "" {
		siteIdent = entry.SiteUUID
	}

	_, err := db.Exec(query,
		siteIdent,
		entry.UpdateType,
		entry.Status,
		entry.DataJSON,
		entry.UpdatedBy,
	)

	if err != nil {
		return fmt.Errorf("failed to save site update ledger entry for site %v: %w", siteIdent, err)
	}

	return nil
}

// GetSiteUpdateLedger retrieves all update ledger entries for a specific site, sorted by newest first.
// siteID can be an int or a string (UUID).
func GetSiteUpdateLedger(siteID any) ([]models.SiteUpdateLedgerEntry, error) {
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
		return nil, fmt.Errorf("failed to query update ledger for site %v: %w", siteID, err)
	}
	defer rows.Close()

	entries := []models.SiteUpdateLedgerEntry{}
	for rows.Next() {
		e, err := scanUpdateLedgerRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site update ledger entry: %w", err)
		}
		entries = append(entries, *e)
	}

	return entries, nil
}

// GetLatestSiteUpdateLedgerEntry retrieves the most recent update ledger entry for a specific site.
// siteID can be an int or a string (UUID).
func GetLatestSiteUpdateLedgerEntry(siteID any) (*models.SiteUpdateLedgerEntry, error) {
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

	e, err := scanUpdateLedgerRow(db.QueryRow(query, siteID))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan latest site update ledger entry: %w", err)
	}

	return e, nil
}

// GetLatestSiteUpdateLedgerEntries retrieves the most recent update ledger entry for all sites,
// returned as a map of site_id (string UUID or numeric string) -> entry.
func GetLatestSiteUpdateLedgerEntries() (map[string]models.SiteUpdateLedgerEntry, error) {
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

	entries := make(map[string]models.SiteUpdateLedgerEntry)
	for rows.Next() {
		e, err := scanUpdateLedgerRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site update ledger entry: %w", err)
		}
		entries[e.SiteUUID] = *e
		if id, ok := e.SiteID.(int); ok {
			entries[strconv.Itoa(id)] = *e
		}
	}

	return entries, nil
}
