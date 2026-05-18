package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
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

// MonitorIgnoreMatcher provides efficient in-memory matching for monitor ignores.
type MonitorIgnoreMatcher struct {
	siteIgnores   map[int]bool
	serverIgnores map[int][]int // serverID -> negated site IDs
}

// NewMonitorIgnoreMatcher fetches all monitor ignore entries and returns a matcher.
func NewMonitorIgnoreMatcher() (*MonitorIgnoreMatcher, error) {
	entries, err := GetAllIgnoreEntries("")
	if err != nil {
		return nil, err
	}

	matcher := &MonitorIgnoreMatcher{
		siteIgnores:   make(map[int]bool),
		serverIgnores: make(map[int][]int),
	}

	for _, e := range entries {
		if !e.UseForMonitor {
			continue
		}

		targetID, err := strconv.Atoi(e.Target)
		if err != nil {
			continue
		}

		switch e.Type {
		case "site":
			matcher.siteIgnores[targetID] = true
		case "server":
			matcher.serverIgnores[targetID] = e.NegatedSiteIDs
		}
	}

	return matcher, nil
}

// IsIgnored checks if a site is ignored according to the matcher's data.
func (m *MonitorIgnoreMatcher) IsIgnored(siteID, serverID int) bool {
	if m.siteIgnores[siteID] {
		return true
	}

	if negatedIDs, ok := m.serverIgnores[serverID]; ok {
		isNegated := false
		for _, id := range negatedIDs {
			if id == siteID {
				isNegated = true
				break
			}
		}
		if !isNegated {
			return true
		}
	}

	return false
}

// VulnIgnoreMatcher provides efficient in-memory matching for vulnerability ignores.
type VulnIgnoreMatcher struct {
	siteIgnores          map[int]bool
	serverIgnores        map[int][]int // serverID -> negated site IDs
	pluginIgnores        map[string]bool
	vulnerabilityIgnores map[string]bool
}

// NewVulnIgnoreMatcher fetches all vulnerability ignore entries and returns a matcher.
func NewVulnIgnoreMatcher() (*VulnIgnoreMatcher, error) {
	entries, err := GetAllIgnoreEntries("")
	if err != nil {
		return nil, err
	}

	matcher := &VulnIgnoreMatcher{
		siteIgnores:          make(map[int]bool),
		serverIgnores:        make(map[int][]int),
		pluginIgnores:        make(map[string]bool),
		vulnerabilityIgnores: make(map[string]bool),
	}

	for _, e := range entries {
		if !e.UseForVuln {
			continue
		}

		switch e.Type {
		case "site":
			if id, err := strconv.Atoi(e.Target); err == nil {
				matcher.siteIgnores[id] = true
			}
		case "server":
			if id, err := strconv.Atoi(e.Target); err == nil {
				matcher.serverIgnores[id] = e.NegatedSiteIDs
			}
		case "plugin":
			matcher.pluginIgnores[e.Target] = true
		case "vulnerability":
			matcher.vulnerabilityIgnores[e.Target] = true
		}
	}

	return matcher, nil
}

// IsSiteIgnored checks if a site should be ignored for vulnerabilities.
func (m *VulnIgnoreMatcher) IsSiteIgnored(siteID, serverID int) bool {
	if m.siteIgnores[siteID] {
		return true
	}

	if negatedIDs, ok := m.serverIgnores[serverID]; ok {
		isNegated := false
		for _, id := range negatedIDs {
			if id == siteID {
				isNegated = true
				break
			}
		}
		if !isNegated {
			return true
		}
	}

	return false
}

// IsVulnerabilityIgnored checks if a specific vulnerability should be ignored.
func (m *VulnIgnoreMatcher) IsVulnerabilityIgnored(siteID, serverID int, pluginSlug, vulnUUID string) bool {
	if m.vulnerabilityIgnores[vulnUUID] {
		return true
	}

	if pluginSlug != "" && m.pluginIgnores[pluginSlug] {
		return true
	}

	return m.IsSiteIgnored(siteID, serverID)
}
