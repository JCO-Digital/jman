package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// GetMonitorHistory returns monitoring history for all sites for the specified number of hours.
func GetMonitorHistory(hours int) ([]models.MonitorHistory, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
		SELECT id, domain, status, error_code, first_seen, last_seen, count
		FROM monitor_history
		WHERE last_seen >= ?
		ORDER BY first_seen DESC
	`
	rows, err := db.Query(query, time.Now().Add(-time.Duration(hours)*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("failed to query monitor history: %w", err)
	}
	defer rows.Close()

	history := []models.MonitorHistory{}
	for rows.Next() {
		var h models.MonitorHistory
		err := rows.Scan(
			&h.ID,
			&h.Domain,
			&h.Status,
			&h.ErrorCode,
			&h.FirstSeen,
			&h.LastSeen,
			&h.Count,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan monitor history: %w", err)
		}
		history = append(history, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return history, nil
}

// GetMonitorStatus returns the current monitoring status for a specific domain.
func GetMonitorStatus(domain string) (*models.MonitorStatus, error) {
	domain = strings.ToLower(domain)
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
		SELECT domain, is_down, failure_count, last_alert_time, last_checked
		FROM monitor_status
		WHERE domain = ?
	`
	var s models.MonitorStatus
	var lastAlertTime sql.NullTime

	err := db.QueryRow(query, domain).Scan(
		&s.Domain,
		&s.IsDown,
		&s.FailureCount,
		&lastAlertTime,
		&s.LastChecked,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get monitor status for %s: %w", domain, err)
	}

	if lastAlertTime.Valid {
		s.LastAlertTime = &lastAlertTime.Time
	}

	return &s, nil
}

// GetAllMonitorStatuses returns the current monitoring status for all sites.
func GetAllMonitorStatuses() ([]models.MonitorStatus, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
		SELECT domain, is_down, failure_count, last_alert_time, last_checked
		FROM monitor_status
		ORDER BY domain ASC
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query monitor statuses: %w", err)
	}
	defer rows.Close()

	statuses := []models.MonitorStatus{}
	for rows.Next() {
		var s models.MonitorStatus
		var lastAlertTime sql.NullTime
		err := rows.Scan(
			&s.Domain,
			&s.IsDown,
			&s.FailureCount,
			&lastAlertTime,
			&s.LastChecked,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan monitor status: %w", err)
		}
		if lastAlertTime.Valid {
			s.LastAlertTime = &lastAlertTime.Time
		}
		statuses = append(statuses, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return statuses, nil
}

// RecordMonitorHistory extends the latest monitor_history entry for a
// domain if its status is unchanged, or starts a new entry otherwise.
// The read and the write are not atomic; the monitor serializes calls.
func RecordMonitorHistory(domain, status string, errorCode int) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	domain = strings.ToLower(domain)

	var lastID int
	var lastStatus string
	err := db.QueryRow(`SELECT id, status FROM monitor_history WHERE domain = ? ORDER BY id DESC LIMIT 1`, domain).Scan(&lastID, &lastStatus)
	if err == nil && lastStatus == status {
		if _, err := db.Exec(`UPDATE monitor_history SET last_seen = CURRENT_TIMESTAMP, count = count + 1 WHERE id = ?`, lastID); err != nil {
			return fmt.Errorf("failed to update monitor history for %s: %w", domain, err)
		}
		return nil
	}
	if _, err := db.Exec(`INSERT INTO monitor_history (domain, status, error_code) VALUES (?, ?, ?)`, domain, status, errorCode); err != nil {
		return fmt.Errorf("failed to insert monitor history for %s: %w", domain, err)
	}
	return nil
}
