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
	db := GetInventoryDB()
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
	db := GetInventoryDB()
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
		e.NegatedSiteIDs = []string{}
	}

	return &e, nil
}

// GetAllIgnoreEntries returns all ignore entries, optionally filtered by type.
func GetAllIgnoreEntries(entryType string) ([]models.IgnoreEntry, error) {
	db := GetInventoryDB()
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
			e.NegatedSiteIDs = []string{}
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// DeleteIgnoreEntry removes an ignore entry.
func DeleteIgnoreEntry(id int) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM ignore_entries WHERE id = ?", id)
	return err
}

// siteServerIgnores holds site- and server-scoped ignore rules keyed by
// site/server UUID, shared by the monitor and vulnerability matchers.
type siteServerIgnores struct {
	sites   map[string]bool
	servers map[string][]string // server UUID -> negated site UUIDs
}

func newSiteServerIgnores() siteServerIgnores {
	return siteServerIgnores{sites: make(map[string]bool), servers: make(map[string][]string)}
}

func (ig siteServerIgnores) add(e models.IgnoreEntry) {
	switch e.Type {
	case "site":
		ig.sites[e.Target] = true
	case "server":
		ig.servers[e.Target] = e.NegatedSiteIDs
	}
}

// isIgnored reports whether a site (or its server, unless the site is
// negated from that server-wide rule) is ignored. Empty IDs never match.
func (ig siteServerIgnores) isIgnored(siteID, serverID string) bool {
	if siteID != "" && ig.sites[siteID] {
		return true
	}
	if serverID == "" {
		return false
	}
	negatedIDs, ok := ig.servers[serverID]
	if !ok {
		return false
	}
	for _, id := range negatedIDs {
		if id == siteID {
			return false
		}
	}
	return true
}

// MonitorIgnoreMatcher provides efficient in-memory matching for monitor ignores.
type MonitorIgnoreMatcher struct {
	siteServerIgnores
}

// NewMonitorIgnoreMatcher fetches all monitor ignore entries and returns a matcher.
func NewMonitorIgnoreMatcher() (*MonitorIgnoreMatcher, error) {
	entries, err := GetAllIgnoreEntries("")
	if err != nil {
		return nil, err
	}

	matcher := &MonitorIgnoreMatcher{siteServerIgnores: newSiteServerIgnores()}
	for _, e := range entries {
		if e.UseForMonitor {
			matcher.add(e)
		}
	}

	return matcher, nil
}

// IsIgnored checks if a site (by site and server UUID) is ignored for monitoring.
func (m *MonitorIgnoreMatcher) IsIgnored(siteID, serverID string) bool {
	return m.isIgnored(siteID, serverID)
}

// VulnIgnoreMatcher provides efficient in-memory matching for vulnerability ignores.
type VulnIgnoreMatcher struct {
	siteServerIgnores
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
		siteServerIgnores:    newSiteServerIgnores(),
		pluginIgnores:        make(map[string]bool),
		vulnerabilityIgnores: make(map[string]bool),
	}

	for _, e := range entries {
		if !e.UseForVuln {
			continue
		}

		switch e.Type {
		case "site", "server":
			matcher.add(e)
		case "plugin":
			matcher.pluginIgnores[e.Target] = true
		case "vulnerability":
			matcher.vulnerabilityIgnores[e.Target] = true
		}
	}

	return matcher, nil
}

// IsSiteIgnored checks if a site (by site and server UUID) should be ignored for vulnerabilities.
func (m *VulnIgnoreMatcher) IsSiteIgnored(siteID, serverID string) bool {
	return m.isIgnored(siteID, serverID)
}

// IsVulnerabilityUUIDIgnored checks if a specific vulnerability UUID is ignored.
func (m *VulnIgnoreMatcher) IsVulnerabilityUUIDIgnored(vulnUUID string) bool {
	return m.vulnerabilityIgnores[vulnUUID]
}

// IsPluginIgnored checks if a plugin is ignored for vulnerabilities.
func (m *VulnIgnoreMatcher) IsPluginIgnored(pluginSlug string) bool {
	return m.pluginIgnores[pluginSlug]
}
