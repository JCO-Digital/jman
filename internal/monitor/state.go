package monitor

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

const (
	ModeNormal        = "normal"
	ModeInvestigation = "investigation"
	ModeAlert         = "alert"
)

const monitorStateFile = "monitor_state"

var (
	migrationOnce sync.Once
	// globalWriteMu ensures that only one database write operation happens at a time,
	// which is critical for SQLite stability in concurrent environments.
	globalWriteMu sync.Mutex
)

// SiteStatus tracks the monitoring state for an individual site.
type SiteStatus struct {
	Mu       sync.Mutex `json:"-"`
	InFlight bool       `json:"-"`

	ID                   int       `json:"id"`
	ServerID             int       `json:"server_id"`
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

	state := &State{
		Sites: make(map[string]*SiteStatus),
	}

	rows, err := database.Query("SELECT domain, is_down, failure_count, consecutive_successes, current_mode, last_alert_time, last_checked, next_check_at FROM monitor_status")
	if err != nil {
		return nil, fmt.Errorf("failed to load monitor status: %w", err)
	}
	defer rows.Close()

	// Fetch sites from cache to populate IDs
	cachedSites, _ := cache.GetCachedSites()
	siteMap := make(map[string]models.Site)
	for _, s := range cachedSites {
		siteMap[strings.ToLower(s.Domain)] = s
	}

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

		if s, ok := siteMap[strings.ToLower(domain)]; ok {
			status.ID = s.ID
			status.ServerID = s.ServerID
		}

		// Normalize mode based on is_down status to ensure continuity after migration.
		// If a site is marked as down, it must be in Alert mode.
		if status.IsDown && status.CurrentMode != ModeAlert {
			status.CurrentMode = ModeAlert
		}

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
// It handles its own synchronization for both the SiteStatus object and the database.
func SaveSiteStatus(status *SiteStatus) error {
	database := db.GetDB()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}

	// Lock the status to get a consistent snapshot of the data
	status.Mu.Lock()
	domain := strings.ToLower(status.Domain)
	isDown := status.IsDown
	failureCount := status.FailureCount
	consecutiveSuccesses := status.ConsecutiveSuccesses
	currentMode := status.CurrentMode
	lastAlertTimeVal := status.LastAlertTime
	lastChecked := status.LastChecked
	nextCheckAt := status.NextCheckAt
	status.Mu.Unlock()

	var lastAlertTime interface{}
	if !lastAlertTimeVal.IsZero() {
		lastAlertTime = lastAlertTimeVal
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

	// Ensure serialized writes to the database
	globalWriteMu.Lock()
	defer globalWriteMu.Unlock()

	_, err := database.Exec(query,
		domain,
		isDown,
		failureCount,
		consecutiveSuccesses,
		currentMode,
		lastAlertTime,
		lastChecked,
		nextCheckAt,
	)

	return err
}

// GetStatus returns the status for a given domain, creating it if it doesn't exist.
func (s *State) GetStatus(domain string) *SiteStatus {
	domain = strings.ToLower(domain)
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

	// Try to populate IDs from cache
	if cachedSites, err := cache.GetCachedSites(); err == nil {
		for _, cs := range cachedSites {
			if strings.ToLower(cs.Domain) == domain {
				status.ID = cs.ID
				status.ServerID = cs.ServerID
				break
			}
		}
	}

	s.Sites[domain] = status
	return status
}

// RemoveStatus deletes the status for a given domain from both the state map and database.
func (s *State) RemoveStatus(domain string) {
	domain = strings.ToLower(domain)
	s.Mu.Lock()
	defer s.Mu.Unlock()

	delete(s.Sites, domain)

	database := db.GetDB()
	if database != nil {
		globalWriteMu.Lock()
		defer globalWriteMu.Unlock()
		_, _ = database.Exec("DELETE FROM monitor_status WHERE domain = ?", domain)
	}
}

// RecordHistory updates the history table with the current check result.
func RecordHistory(domain string, isUp bool, statusMsg string, errorCode int) {
	domain = strings.ToLower(domain)
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

	globalWriteMu.Lock()
	defer globalWriteMu.Unlock()

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
