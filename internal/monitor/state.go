package monitor

import (
	"log"
	"strings"
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

	ID                   string    `json:"id"`        // site UUID
	ServerID             string    `json:"server_id"` // server UUID
	Domain               string    `json:"domain"`
	IsDown               bool      `json:"is_down"`
	FailureCount         int       `json:"failure_count"`
	ConsecutiveSuccesses int       `json:"consecutive_successes"`
	CurrentMode          string    `json:"current_mode"`
	LastAlertTime        time.Time `json:"last_alert_time"`
	LastChecked          time.Time `json:"last_checked"`
	NextCheckAt          time.Time `json:"next_check_at"`

	// DownSince, PDTriggered and PDEscalated track the PagerDuty escalation
	// timer for the current outage. DownSince is set once, when the site
	// transitions into ModeAlert, and cleared on recovery.
	DownSince   time.Time `json:"down_since"`
	PDTriggered bool      `json:"pd_triggered"`
	PDEscalated bool      `json:"pd_escalated"`
}

// State represents the overall monitoring state for all sites.
type State struct {
	Mu    sync.RWMutex
	Sites map[string]*SiteStatus `json:"sites"`
}

// LoadState reads the monitor state from the database.
func LoadState() (*State, error) {
	records, err := db.GetAllMonitorStatusRecords()
	if err != nil {
		return nil, err
	}

	state := &State{
		Sites: make(map[string]*SiteStatus),
	}

	// Fetch monitor targets to populate IDs
	targets, _ := cache.GetMonitorTargets()
	siteMap := make(map[string]cache.MonitorTarget)
	for _, t := range targets {
		siteMap[strings.ToLower(t.Domain)] = t
	}

	for _, r := range records {
		status := &SiteStatus{
			Domain:               r.Domain,
			IsDown:               r.IsDown,
			FailureCount:         r.FailureCount,
			ConsecutiveSuccesses: r.ConsecutiveSuccesses,
			CurrentMode:          r.CurrentMode,
			LastAlertTime:        r.LastAlertTime,
			LastChecked:          r.LastChecked,
			NextCheckAt:          r.NextCheckAt,
			DownSince:            r.DownSince,
			PDTriggered:          r.PDTriggered,
			PDEscalated:          r.PDEscalated,
		}

		if t, ok := siteMap[strings.ToLower(r.Domain)]; ok {
			status.ID = t.SiteID
			status.ServerID = t.ServerID
		}

		// Normalize mode based on is_down status to ensure continuity after migration.
		// If a site is marked as down, it must be in Alert mode.
		if status.IsDown && status.CurrentMode != ModeAlert {
			status.CurrentMode = ModeAlert
		}

		state.Sites[r.Domain] = status
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
	// Lock the status to get a consistent snapshot of the data
	status.Mu.Lock()
	record := db.MonitorStatusRecord{
		Domain:               status.Domain,
		IsDown:               status.IsDown,
		FailureCount:         status.FailureCount,
		ConsecutiveSuccesses: status.ConsecutiveSuccesses,
		CurrentMode:          status.CurrentMode,
		LastAlertTime:        status.LastAlertTime,
		LastChecked:          status.LastChecked,
		NextCheckAt:          status.NextCheckAt,
		DownSince:            status.DownSince,
		PDTriggered:          status.PDTriggered,
		PDEscalated:          status.PDEscalated,
	}
	status.Mu.Unlock()

	// Ensure serialized writes to the database
	globalWriteMu.Lock()
	defer globalWriteMu.Unlock()

	return db.SaveMonitorStatusRecord(record)
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

	// Try to populate IDs from the monitor targets
	if targets, err := cache.GetMonitorTargets(); err == nil {
		for _, t := range targets {
			if strings.ToLower(t.Domain) == domain {
				status.ID = t.SiteID
				status.ServerID = t.ServerID
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

	if db.GetAPIDB() != nil {
		globalWriteMu.Lock()
		defer globalWriteMu.Unlock()
		_ = db.DeleteMonitorStatus(domain)
	}
}

// RecordHistory updates the history table with the current check result.
func RecordHistory(domain string, isUp bool, statusMsg string, errorCode int) {
	if db.GetAPIDB() == nil {
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

	if err := db.RecordMonitorHistory(domain, statusText, errorCode); err != nil {
		log.Printf("Warning: %v\n", err)
	}
}
