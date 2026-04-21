package db

import (
	"database/sql"
	"fmt"

	"github.com/JCO-Digital/jman/internal/models"
)

// GetMonitorHistory returns monitoring history for all sites for the specified number of hours.
func GetMonitorHistory(hours int) ([]models.MonitorHistory, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
		SELECT id, domain, status, error_code, first_seen, last_seen, count
		FROM monitor_history
		WHERE last_seen >= datetime('now', ?)
		ORDER BY first_seen DESC
	`
	interval := fmt.Sprintf("-%d hours", hours)
	rows, err := db.Query(query, interval)
	if err != nil {
		return nil, fmt.Errorf("failed to query monitor history: %w", err)
	}
	defer rows.Close()

	var history []models.MonitorHistory
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

	return history, nil
}

// GetMonitorStatus returns the current monitoring status for a specific domain.
func GetMonitorStatus(domain string) (*models.MonitorStatus, error) {
	db := GetDB()
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
	db := GetDB()
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

	var statuses []models.MonitorStatus
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

	return statuses, nil
}
