package monitor

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
)

const monitorStateFile = "monitor_state"

var migrationOnce sync.Once

// SiteStatus tracks the monitoring state for an individual site.
type SiteStatus struct {
	FailureCount  int       `json:"failure_count"`
	LastAlertTime time.Time `json:"last_alert_time"`
	IsDown        bool      `json:"is_down"`
	Domain        string    `json:"-"`
}

// State represents the overall monitoring state for all sites.
type State struct {
	Sites map[string]*SiteStatus `json:"sites"`
}

// LoadState reads the monitor state from the database, migrating from JSON if necessary.
func LoadState() (*State, error) {
	database := db.GetDB()
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	migrationOnce.Do(func() {
		migrateMonitorState(database)
	})

	state := &State{
		Sites: make(map[string]*SiteStatus),
	}

	rows, err := database.Query("SELECT domain, failure_count, last_alert_time, is_down FROM monitor_status")
	if err != nil {
		return nil, fmt.Errorf("failed to load monitor status: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var domain string
		var lastAlertTime sql.NullTime
		status := &SiteStatus{}
		err := rows.Scan(&domain, &status.FailureCount, &lastAlertTime, &status.IsDown)
		if err != nil {
			return nil, fmt.Errorf("failed to scan monitor status: %w", err)
		}
		status.Domain = domain
		if lastAlertTime.Valid {
			status.LastAlertTime = lastAlertTime.Time
		}
		state.Sites[domain] = status
	}

	return state, nil
}

// SaveState writes the current monitor state to the database.
func (s *State) SaveState() error {
	database := db.GetDB()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}

	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Sync the map to the database by clearing and re-inserting
	_, err = tx.Exec("DELETE FROM monitor_status")
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO monitor_status (domain, failure_count, last_alert_time, is_down, last_checked) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for domain, status := range s.Sites {
		var lastAlertTime interface{}
		if !status.LastAlertTime.IsZero() {
			lastAlertTime = status.LastAlertTime
		}
		_, err = stmt.Exec(domain, status.FailureCount, lastAlertTime, status.IsDown)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetStatus returns the status for a given domain, creating it if it doesn't exist.
func (s *State) GetStatus(domain string) *SiteStatus {
	if status, ok := s.Sites[domain]; ok {
		return status
	}
	status := &SiteStatus{Domain: domain}
	s.Sites[domain] = status
	return status
}

// RemoveStatus deletes the status for a given domain.
func (s *State) RemoveStatus(domain string) {
	delete(s.Sites, domain)
}

// RecordHistory updates the history table with the current check result.
func (s *State) RecordHistory(domain string, isUp bool, statusMsg string, errorCode int) {
	database := db.GetDB()
	if database == nil {
		return
	}

	// Determine status string
	statusText := "UP"
	if !isUp {
		statusText = statusMsg
		if statusText == "" {
			statusText = "DOWN"
		}
	}

	// Check latest history record for this domain
	var lastID int
	var lastStatus string
	var lastCount int
	err := database.QueryRow("SELECT id, status, count FROM monitor_history WHERE domain = ? ORDER BY id DESC LIMIT 1", domain).Scan(&lastID, &lastStatus, &lastCount)

	if err == nil && lastStatus == statusText {
		// Same status, update existing record
		_, err = database.Exec("UPDATE monitor_history SET last_seen = CURRENT_TIMESTAMP, count = count + 1 WHERE id = ?", lastID)
		if err != nil {
			log.Printf("Warning: failed to update monitor history for %s: %v\n", domain, err)
		}
	} else {
		// New status or no previous record, insert new
		_, err = database.Exec("INSERT INTO monitor_history (domain, status, error_code) VALUES (?, ?, ?)", domain, statusText, errorCode)
		if err != nil {
			log.Printf("Warning: failed to insert monitor history for %s: %v\n", domain, err)
		}
	}
}

func migrateMonitorState(database *sql.DB) {
	var state struct {
		Sites map[string]*SiteStatus `json:"sites"`
	}
	err := cache.ReadJSONData(monitorStateFile, &state)
	if err != nil {
		return
	}

	log.Printf("Migrating monitor state to database...\n")

	for domain, status := range state.Sites {
		var lastAlertTime interface{}
		if !status.LastAlertTime.IsZero() {
			lastAlertTime = status.LastAlertTime
		}
		_, err := database.Exec(
			"INSERT OR REPLACE INTO monitor_status (domain, failure_count, last_alert_time, is_down) VALUES (?, ?, ?, ?)",
			domain, status.FailureCount, lastAlertTime, status.IsDown,
		)
		if err != nil {
			log.Printf("Warning: failed to migrate monitor status for %s: %v\n", domain, err)
		}
	}

	// Delete the old file after migration
	oldPath := cache.GetDataFilePath(monitorStateFile)
	if err := os.Remove(oldPath); err != nil {
		log.Printf("Warning: failed to remove old monitor state file: %v\n", err)
	}
}
