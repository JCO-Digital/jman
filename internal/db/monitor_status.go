package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// GetSiteMode returns the current monitoring mode for a given domain from the database.
func GetSiteMode(domain string) (string, error) {
	db := GetAPIDB()
	if db == nil {
		return "", fmt.Errorf("database not initialized")
	}

	var mode string
	query := `SELECT current_mode FROM monitor_status WHERE domain = LOWER(?)`
	err := db.QueryRow(query, domain).Scan(&mode)
	if err != nil {
		if err == sql.ErrNoRows {
			return "normal", nil // Default mode if site has never been checked
		}
		return "", fmt.Errorf("failed to get site mode for %s: %w", domain, err)
	}
	return mode, nil
}

// IsSiteInAlertMode checks if a domain is currently in alert mode.
func IsSiteInAlertMode(domain string) (bool, error) {
	mode, err := GetSiteMode(domain)
	if err != nil {
		return false, err
	}
	return mode == "alert", nil
}

// MonitorStatusRecord is one monitor_status row as the monitor scheduler
// persists it. Zero LastAlertTime and DownSince are stored as NULL and
// read back as zero.
type MonitorStatusRecord struct {
	Domain               string
	IsDown               bool
	FailureCount         int
	ConsecutiveSuccesses int
	CurrentMode          string
	LastAlertTime        time.Time
	LastChecked          time.Time
	NextCheckAt          time.Time
	DownSince            time.Time
	PDTriggered          bool
	PDEscalated          bool
}

// GetAllMonitorStatusRecords returns every persisted monitor_status row.
func GetAllMonitorStatusRecords() ([]MonitorStatusRecord, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT domain, is_down, failure_count, consecutive_successes, current_mode,
		last_alert_time, last_checked, next_check_at, down_since, pd_triggered, pd_escalated
		FROM monitor_status`)
	if err != nil {
		return nil, fmt.Errorf("failed to load monitor status: %w", err)
	}
	defer rows.Close()

	var records []MonitorStatusRecord
	for rows.Next() {
		var r MonitorStatusRecord
		var lastAlertTime, lastChecked, nextCheckAt, downSince sql.NullTime
		if err := rows.Scan(
			&r.Domain, &r.IsDown, &r.FailureCount, &r.ConsecutiveSuccesses, &r.CurrentMode,
			&lastAlertTime, &lastChecked, &nextCheckAt, &downSince, &r.PDTriggered, &r.PDEscalated,
		); err != nil {
			return nil, fmt.Errorf("failed to scan monitor status: %w", err)
		}
		r.LastAlertTime = lastAlertTime.Time
		r.LastChecked = lastChecked.Time
		r.NextCheckAt = nextCheckAt.Time
		r.DownSince = downSince.Time
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate monitor status: %w", err)
	}
	return records, nil
}

// SaveMonitorStatusRecord inserts or updates the monitor_status row for
// r.Domain (lowercased).
func SaveMonitorStatusRecord(r MonitorStatusRecord) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	var lastAlertTime, downSince any
	if !r.LastAlertTime.IsZero() {
		lastAlertTime = r.LastAlertTime
	}
	if !r.DownSince.IsZero() {
		downSince = r.DownSince
	}

	_, err := db.Exec(`
		INSERT INTO monitor_status (
			domain, is_down, failure_count, consecutive_successes,
			current_mode, last_alert_time, last_checked, next_check_at,
			down_since, pd_triggered, pd_escalated
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET
			is_down = excluded.is_down,
			failure_count = excluded.failure_count,
			consecutive_successes = excluded.consecutive_successes,
			current_mode = excluded.current_mode,
			last_alert_time = excluded.last_alert_time,
			last_checked = excluded.last_checked,
			next_check_at = excluded.next_check_at,
			down_since = excluded.down_since,
			pd_triggered = excluded.pd_triggered,
			pd_escalated = excluded.pd_escalated
	`,
		strings.ToLower(r.Domain), r.IsDown, r.FailureCount, r.ConsecutiveSuccesses,
		r.CurrentMode, lastAlertTime, r.LastChecked, r.NextCheckAt,
		downSince, r.PDTriggered, r.PDEscalated,
	)
	return err
}

// DeleteMonitorStatus removes the monitor_status row for a domain.
func DeleteMonitorStatus(domain string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(`DELETE FROM monitor_status WHERE domain = ?`, strings.ToLower(domain))
	return err
}

// ResetMonitorStatus puts a domain's persisted status back to normal,
// clearing its down/PagerDuty state and scheduling an immediate check.
func ResetMonitorStatus(domain string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(`
		UPDATE monitor_status
		SET current_mode = 'normal', is_down = FALSE, failure_count = 0, consecutive_successes = 0,
		    down_since = NULL, pd_triggered = FALSE, pd_escalated = FALSE, next_check_at = CURRENT_TIMESTAMP
		WHERE domain = ?
	`, strings.ToLower(domain))
	return err
}
