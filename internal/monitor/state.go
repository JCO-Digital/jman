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

const (
	ModeNormal        = "normal"
	ModeInvestigation = "investigation"
	ModeAlert         = "alert"
)

const monitorStateFile = "monitor_state"

var migrationOnce sync.Once

// SiteStatus tracks the monitoring state for an individual site.
type SiteStatus struct {
	Domain               string    `json:"domain"`
	IsDown               bool      `json:"is_down"`
	FailureCount         int       `json:"failure_count"`
	ConsecutiveSuccesses int       `json:"consecutive_successes"`
	CurrentMode          string    `json:"current_mode"`
	LastAlertTime        time.Time `json:"last_alert_time"`
	LastChecked          time.Time `json:"last_checked"`
	NextCheckAt          time.Time `json:"next_check_at"`
}

// State represents the overall monitoring state for all sites.
type State struct {
	Mu    sync.RWMutex
	Sites map[string]*SiteStatus `json:"sites"`
}

// LoadState reads the monitor state from the database.
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

	rows, err := database.Query("SELECT domain, is_down, failure_count, consecutive_successes, current_mode, last_alert_time, last_checked, next_check_at FROM monitor_status")
	if err != nil {
		return nil, fmt.Errorf("failed to load monitor status: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var domain string
		var lastAlertTime, lastChecked, nextCheckAt sql.NullTime
		status := &SiteStatus{}
		err := rows.Scan(
			&domain,
			&status.IsDown,
			&status.FailureCount,
			&status.ConsecutiveSuccesses,
			&status.CurrentMode,
			&lastAlertTime,
			&lastChecked,
			&nextCheckAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan monitor status: %w", err)
		}
		status.Domain = domain
		if lastAlertTime.Valid {
			status.LastAlertTime = lastAlertTime.Time
		}
		if lastChecked.Valid {
			status.LastChecked = lastChecked.Time
		}
		if nextCheckAt.Valid {
			status.NextCheckAt = nextCheckAt.Time
		}
		state.Sites[domain] = status
	}

	return state, nil
}

// SaveState writes the entire current monitor state to the database.
// Note: In a long-running daemon, it is usually better to use SaveSiteStatus for incremental updates.
func (s *State) SaveState() error {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	for _, status := range s.Sites {
		if err := SaveSiteStatus(status); err != nil {
			return err
		}
	}
	return nil
}

// SaveSiteStatus updates or inserts the status for a single site in the database.
func SaveSiteStatus(status *SiteStatus) error {
	database := db.GetDB()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `
		INSERT INTO monitor_status (
			domain, is_down, failure_count, consecutive_successes,
			current_mode, last_alert_time, last_checked, next_check_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET
			is_down = excluded.is_down,
			failure_count = excluded.failure_count,
			consecutive_successes = excluded.consecutive_successes,
			current_mode = excluded.current_mode,
			last_alert_time = excluded.last_alert_time,
			last_checked = excluded.last_checked,
			next_check_at = excluded.next_check_at
	`

	var lastAlertTime interface{}
	if !status.LastAlertTime.IsZero() {
		lastAlertTime = status.LastAlertTime
	}

	_, err := database.Exec(query,
		status.Domain,
		status.IsDown,
		status.FailureCount,
		status.ConsecutiveSuccesses,
		status.CurrentMode,
		lastAlertTime,
		status.LastChecked,
		status.NextCheckAt,
	)

	return err
}

// GetStatus returns the status for a given domain, creating it if it doesn't exist.
func (s *State) GetStatus(domain string) *SiteStatus {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if status, ok := s.Sites[domain]; ok {
		return status
	}
	status := &SiteStatus{
		Domain:      domain,
		CurrentMode: ModeNormal,
		NextCheckAt: time.Now(),
	}
	s.Sites[domain] = status
	return status
}

// RemoveStatus deletes the status for a given domain from both the state map and database.
func (s *State) RemoveStatus(domain string) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	delete(s.Sites, domain)

	database := db.GetDB()
	if database != nil {
		_, _ = database.Exec("DELETE FROM monitor_status WHERE domain = ?", domain)
	}
}

// RecordHistory updates the history table with the current check result.
func RecordHistory(domain string, isUp bool, statusMsg string, errorCode int) {
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

// migrateMonitorState migrates from the old JSON state file if it exists.
func migrateMonitorState(database *sql.DB) {
	var state struct {
		Sites map[string]struct {
			FailureCount  int       `json:"failure_count"`
			LastAlertTime time.Time `json:"last_alert_time"`
			IsDown        bool      `json:"is_down"`
		} `json:"sites"`
	}
	err := cache.ReadJSONData(monitorStateFile, &state)
	if err != nil {
		return
	}

	log.Printf("Migrating monitor state to database...\n")

	for domain, oldStatus := range state.Sites {
		status := &SiteStatus{
			Domain:        domain,
			IsDown:        oldStatus.IsDown,
			FailureCount:  oldStatus.FailureCount,
			LastAlertTime: oldStatus.LastAlertTime,
			CurrentMode:   ModeNormal,
			NextCheckAt:   time.Now(),
		}
		if status.IsDown {
			status.CurrentMode = ModeAlert
		}

		err := SaveSiteStatus(status)
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
