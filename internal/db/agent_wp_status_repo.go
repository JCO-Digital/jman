package db

import (
	"database/sql"
	"fmt"
	"time"
)

// SiteAgentWPStatus is jman-agent's WordPress data collection state for one
// site.
type SiteAgentWPStatus struct {
	SiteID string
	// CollectedAt is the last successful collection; zero if none yet.
	CollectedAt time.Time
	// Error is the latest failure, cleared by the next success.
	Error   string
	ErrorAt time.Time
}

// RecordSiteAgentWPCollection records a collection attempt the agent
// reported: a success (errMsg "") advances CollectedAt and clears the
// error; a failure keeps CollectedAt and stores the error.
func RecordSiteAgentWPCollection(siteID string, at time.Time, errMsg string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	var err error
	if errMsg == "" {
		_, err = db.Exec(`
		INSERT INTO site_agent_wp_status (site_id, collected_at, error, error_at, updated_at)
		VALUES (?, ?, NULL, NULL, ?)
		ON CONFLICT(site_id) DO UPDATE SET
			collected_at = excluded.collected_at,
			error = NULL,
			error_at = NULL,
			updated_at = excluded.updated_at`,
			siteID, at.UTC(), time.Now().UTC())
	} else {
		_, err = db.Exec(`
		INSERT INTO site_agent_wp_status (site_id, collected_at, error, error_at, updated_at)
		VALUES (?, NULL, ?, ?, ?)
		ON CONFLICT(site_id) DO UPDATE SET
			error = excluded.error,
			error_at = excluded.error_at,
			updated_at = excluded.updated_at`,
			siteID, errMsg, at.UTC(), time.Now().UTC())
	}
	if err != nil {
		return fmt.Errorf("failed to record agent WordPress collection for site %s: %w", siteID, err)
	}
	return nil
}

// ListSiteAgentWPStatus returns every site's agent collection state by
// site UUID.
func ListSiteAgentWPStatus() (map[string]SiteAgentWPStatus, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.Query(`SELECT site_id, collected_at, error, error_at FROM site_agent_wp_status`)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent WordPress collection status: %w", err)
	}
	defer rows.Close()

	result := map[string]SiteAgentWPStatus{}
	for rows.Next() {
		var s SiteAgentWPStatus
		var collectedAt, errorAt sql.NullTime
		var errMsg sql.NullString
		if err := rows.Scan(&s.SiteID, &collectedAt, &errMsg, &errorAt); err != nil {
			return nil, fmt.Errorf("failed to scan agent WordPress collection status: %w", err)
		}
		s.CollectedAt = collectedAt.Time
		s.Error = errMsg.String
		s.ErrorAt = errorAt.Time
		result[s.SiteID] = s
	}
	return result, rows.Err()
}

// AgentCollectedSites returns the sites whose WordPress data jman-agent has
// collected at least once. The periodic SSH refresh skips them. Outside
// jman-api (no API database, e.g. the CLI) there are none.
func AgentCollectedSites() map[string]bool {
	db := GetAPIDB()
	if db == nil {
		return map[string]bool{}
	}
	rows, err := db.Query(`SELECT site_id FROM site_agent_wp_status WHERE collected_at IS NOT NULL`)
	if err != nil {
		return map[string]bool{}
	}
	defer rows.Close()
	sites := map[string]bool{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			sites[id] = true
		}
	}
	return sites
}

// ListAgentStaleAlerts returns the open stale alerts of a kind (e.g.
// "site"): when each target was last alerted on, by target ID.
func ListAgentStaleAlerts(kind string) (map[string]time.Time, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.Query(`SELECT target_id, alerted_at FROM agent_stale_alerts WHERE kind = ?`, kind)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent stale alerts: %w", err)
	}
	defer rows.Close()
	open := map[string]time.Time{}
	for rows.Next() {
		var id string
		var at time.Time
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		open[id] = at
	}
	return open, rows.Err()
}

// MarkAgentStaleAlerted records that a stale alert was (re)sent now.
func MarkAgentStaleAlerted(kind, targetID string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(`INSERT INTO agent_stale_alerts (kind, target_id, alerted_at) VALUES (?, ?, ?)
		ON CONFLICT(kind, target_id) DO UPDATE SET alerted_at = excluded.alerted_at`,
		kind, targetID, time.Now().UTC())
	return err
}

// ClearAgentStaleAlert removes a stale alert once it has recovered.
func ClearAgentStaleAlert(kind, targetID string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(`DELETE FROM agent_stale_alerts WHERE kind = ? AND target_id = ?`, kind, targetID)
	return err
}
