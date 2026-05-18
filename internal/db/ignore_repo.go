package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// SaveIgnoreEntry saves or updates an ignore entry.
func SaveIgnoreEntry(entry *models.IgnoreEntry, username string) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	negatedJSON, err := json.Marshal(entry.NegatedSiteIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal negated site IDs: %w", err)
	}

	now := time.Now()
	if entry.ID == 0 {
		query := `
		INSERT INTO ignore_entries (type, target, reason, negated_site_ids, use_for_monitor, use_for_vuln, created_at, created_by, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		result, err := db.Exec(query, entry.Type, entry.Target, entry.Reason, string(negatedJSON), entry.UseForMonitor, entry.UseForVuln, now, username, now, username)
		if err != nil {
			return fmt.Errorf("failed to insert ignore entry: %w", err)
		}
		id, _ := result.LastInsertId()
		entry.ID = int(id)
		entry.CreatedAt = now
		entry.CreatedBy = username
		entry.UpdatedAt = now
		entry.UpdatedBy = username
	} else {
		query := `
		UPDATE ignore_entries SET type = ?, target = ?, reason = ?, negated_site_ids = ?, use_for_monitor = ?, use_for_vuln = ?, updated_at = ?, updated_by = ?
		WHERE id = ?
		`
		_, err := db.Exec(query, entry.Type, entry.Target, entry.Reason, string(negatedJSON), entry.UseForMonitor, entry.UseForVuln, now, username, entry.ID)
		if err != nil {
			return fmt.Errorf("failed to update ignore entry: %w", err)
		}
		entry.UpdatedAt = now
		entry.UpdatedBy = username
	}
	return nil
}

// GetIgnoreEntry fetches a single ignore entry by ID.
func GetIgnoreEntry(id int) (*models.IgnoreEntry, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, type, target, reason, negated_site_ids, use_for_monitor, use_for_vuln, created_at, created_by, updated_at, updated_by FROM ignore_entries WHERE id = ?`
	var e models.IgnoreEntry
	var negatedJSON string
	err := db.QueryRow(query, id).Scan(
		&e.ID, &e.Type, &e.Target, &e.Reason, &negatedJSON, &e.UseForMonitor, &e.UseForVuln, &e.CreatedAt, &e.CreatedBy, &e.UpdatedAt, &e.UpdatedBy,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get ignore entry: %w", err)
	}

	if err := json.Unmarshal([]byte(negatedJSON), &e.NegatedSiteIDs); err != nil {
		// If it's empty or invalid, just keep it empty
		e.NegatedSiteIDs = []int{}
	}

	return &e, nil
}

// GetAllIgnoreEntries returns all ignore entries, optionally filtered by type.
func GetAllIgnoreEntries(entryType string) ([]models.IgnoreEntry, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT id, type, target, reason, negated_site_ids, use_for_monitor, use_for_vuln, created_at, created_by, updated_at, updated_by FROM ignore_entries`
	var args []interface{}
	if entryType != "" {
		query += " WHERE type = ?"
		args = append(args, entryType)
	}
	query += " ORDER BY type ASC, target ASC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query ignore entries: %w", err)
	}
	defer rows.Close()

	entries := []models.IgnoreEntry{}
	for rows.Next() {
		var e models.IgnoreEntry
		var negatedJSON string
		if err := rows.Scan(&e.ID, &e.Type, &e.Target, &e.Reason, &negatedJSON, &e.UseForMonitor, &e.UseForVuln, &e.CreatedAt, &e.CreatedBy, &e.UpdatedAt, &e.UpdatedBy); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(negatedJSON), &e.NegatedSiteIDs); err != nil {
			e.NegatedSiteIDs = []int{}
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// DeleteIgnoreEntry removes an ignore entry.
func DeleteIgnoreEntry(id int) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM ignore_entries WHERE id = ?", id)
	return err
}

// IsSiteIgnoredForMonitor checks if a site should be ignored for monitoring.
func IsSiteIgnoredForMonitor(siteID, serverID int) (bool, error) {
	db := GetDB()
	if db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	// Check for site-specific ignore
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM ignore_entries WHERE type = 'site' AND target = ? AND use_for_monitor = 1)`
	err := db.QueryRow(query, fmt.Sprintf("%d", siteID)).Scan(&exists)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}

	// Check for server-wide ignore
	rows, err := db.Query(`SELECT negated_site_ids FROM ignore_entries WHERE type = 'server' AND target = ? AND use_for_monitor = 1`, fmt.Sprintf("%d", serverID))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var negatedJSON string
		if err := rows.Scan(&negatedJSON); err != nil {
			continue
		}
		var negatedIDs []int
		if err := json.Unmarshal([]byte(negatedJSON), &negatedIDs); err == nil {
			isNegated := false
			for _, id := range negatedIDs {
				if id == siteID {
					isNegated = true
					break
				}
			}
			if !isNegated {
				return true, nil // Server is ignored and this site is NOT negated
			}
		} else {
			// If JSON is invalid or empty, assume no negations
			return true, nil
		}
	}

	return false, nil
}

// IsSiteIgnoredForVuln checks if a site should be ignored for vulnerabilities.
func IsSiteIgnoredForVuln(siteID, serverID int) (bool, error) {
	db := GetDB()
	if db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	// Check for site-specific ignore
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM ignore_entries WHERE type = 'site' AND target = ? AND use_for_vuln = 1)`
	err := db.QueryRow(query, fmt.Sprintf("%d", siteID)).Scan(&exists)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}

	// Check for server-wide ignore
	rows, err := db.Query(`SELECT negated_site_ids FROM ignore_entries WHERE type = 'server' AND target = ? AND use_for_vuln = 1`, fmt.Sprintf("%d", serverID))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var negatedJSON string
		if err := rows.Scan(&negatedJSON); err != nil {
			continue
		}
		var negatedIDs []int
		if err := json.Unmarshal([]byte(negatedJSON), &negatedIDs); err == nil {
			isNegated := false
			for _, id := range negatedIDs {
				if id == siteID {
					isNegated = true
					break
				}
			}
			if !isNegated {
				return true, nil
			}
		} else {
			return true, nil
		}
	}

	return false, nil
}

// IsVulnerabilityIgnored checks if a vulnerability should be suppressed.
func IsVulnerabilityIgnored(siteID, serverID int, pluginSlug, vulnUUID string) (bool, error) {
	db := GetDB()
	if db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	// 1. Check specific vulnerability UUID
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM ignore_entries WHERE type = 'vulnerability' AND target = ? AND use_for_vuln = 1)`
	err := db.QueryRow(query, vulnUUID).Scan(&exists)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}

	// 2. Check plugin slug
	if pluginSlug != "" {
		query = `SELECT EXISTS(SELECT 1 FROM ignore_entries WHERE type = 'plugin' AND target = ? AND use_for_vuln = 1)`
		err = db.QueryRow(query, pluginSlug).Scan(&exists)
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}

	// 3. Check site ID
	query = `SELECT EXISTS(SELECT 1 FROM ignore_entries WHERE type = 'site' AND target = ? AND use_for_vuln = 1)`
	err = db.QueryRow(query, fmt.Sprintf("%d", siteID)).Scan(&exists)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}

	// 4. Check server ID with negations
	rows, err := db.Query(`SELECT negated_site_ids FROM ignore_entries WHERE type = 'server' AND target = ? AND use_for_vuln = 1`, fmt.Sprintf("%d", serverID))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var negatedJSON string
		if err := rows.Scan(&negatedJSON); err != nil {
			continue
		}
		var negatedIDs []int
		if err := json.Unmarshal([]byte(negatedJSON), &negatedIDs); err == nil {
			isNegated := false
			for _, id := range negatedIDs {
				if id == siteID {
					isNegated = true
					break
				}
			}
			if !isNegated {
				return true, nil
			}
		} else {
			return true, nil
		}
	}

	return false, nil
}
