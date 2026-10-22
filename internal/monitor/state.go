package monitor

import (
	"log"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

const (
	// ModeWaiting is a new check's mode: it cannot alert until it has
	// been seen up (three successful probes in a row for an HTTP check,
	// a finished check-in for a heartbeat check), so a site that isn't
	// serving yet doesn't alert as soon as its check is added.
	ModeWaiting       = "waiting"
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

// CheckStatus tracks the monitoring state of one monitor check.
type CheckStatus struct {
	Mu       sync.Mutex `json:"-"`
	InFlight bool       `json:"-"`
	// Stopped is set when the check's outage was closed out (it was
	// paused, edited or orphaned) while this status was still referenced.
	Stopped bool `json:"-"`

	CheckID string `json:"check_id"`
	// Domain is the checked host, Probe what is requested and Label the
	// name used in alerts. All three come from the check's Target and are
	// refreshed on every scheduler tick.
	Domain string    `json:"domain"`
	Probe  HTTPProbe `json:"-"`
	Label  string    `json:"label"`
	// Type and Heartbeat come from the Target too.
	Type      string            `json:"type"`
	Heartbeat HeartbeatSettings `json:"-"`
	// LastStartedAt is when a heartbeat check's job last reported starting;
	// LastMessage what its last failed check-in said.
	LastStartedAt time.Time `json:"last_started_at"`
	LastMessage   string    `json:"-"`

	IsDown               bool      `json:"is_down"`
	FailureCount         int       `json:"failure_count"`
	ConsecutiveSuccesses int       `json:"consecutive_successes"`
	CurrentMode          string    `json:"current_mode"`
	LastAlertTime        time.Time `json:"last_alert_time"`
	LastChecked          time.Time `json:"last_checked"`
	NextCheckAt          time.Time `json:"next_check_at"`

	// DownSince, PDTriggered and PDEscalated track the PagerDuty escalation
	// timer for the current outage. DownSince is set once, when the check
	// transitions into ModeAlert, and cleared on recovery.
	DownSince   time.Time `json:"down_since"`
	PDTriggered bool      `json:"pd_triggered"`
	PDEscalated bool      `json:"pd_escalated"`
}

// probe returns what to check, falling back to the domain's home page
// for a status that has no target applied yet.
func (s *CheckStatus) probe() HTTPProbe {
	if s.Probe.URL != "" {
		return s.Probe
	}
	return HTTPProbe{URL: "https://" + s.Domain, Expect: models.MonitorExpectHTML}
}

// subject says what kind of thing the check watches, for alerts.
func (s *CheckStatus) subject() string {
	if s.Type == models.MonitorCheckTypeHeartbeat {
		return "Check-in"
	}
	return "Site"
}

// label returns the check's name for alerts, falling back to its domain.
func (s *CheckStatus) label() string {
	if s.Label != "" {
		return s.Label
	}
	return s.Domain
}

// State represents the overall monitoring state for all checks.
type State struct {
	Mu     sync.RWMutex
	Checks map[string]*CheckStatus `json:"checks"`
}

// LoadState reads the monitor state from the database.
func LoadState() (*State, error) {
	records, err := db.GetAllMonitorStatusRecords()
	if err != nil {
		return nil, err
	}

	state := &State{
		Checks: make(map[string]*CheckStatus),
	}

	for _, r := range records {
		status := &CheckStatus{
			CheckID:              r.CheckID,
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
			LastStartedAt:        r.LastStartedAt,
		}

		// Normalize mode based on is_down status to ensure continuity after migration.
		// If a check is marked as down, it must be in Alert mode.
		if status.IsDown && status.CurrentMode != ModeAlert {
			status.CurrentMode = ModeAlert
		}

		state.Checks[r.CheckID] = status
	}

	return state, nil
}

// SaveState writes the entire current monitor state to the database.
func (s *State) SaveState() error {
	s.Mu.RLock()
	defer s.Mu.RUnlock()

	for _, status := range s.Checks {
		if err := SaveCheckStatus(status); err != nil {
			return err
		}
	}
	return nil
}

// SaveCheckStatus updates or inserts the status for a single check in the database.
// It handles its own synchronization for both the CheckStatus object and the database.
func SaveCheckStatus(status *CheckStatus) error {
	// Lock the status to get a consistent snapshot of the data
	status.Mu.Lock()
	record := db.MonitorStatusRecord{
		CheckID:              status.CheckID,
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
		LastStartedAt:        status.LastStartedAt,
	}
	status.Mu.Unlock()

	// Ensure serialized writes to the database
	globalWriteMu.Lock()
	defer globalWriteMu.Unlock()

	return db.SaveMonitorStatusRecord(record)
}

// GetStatus returns the status for a target's check, creating it in
// ModeWaiting if it doesn't exist, and applies the target's current domain, probe and label.
func (s *State) GetStatus(t Target) *CheckStatus {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	status, ok := s.Checks[t.CheckID]
	if !ok {
		status = &CheckStatus{
			CheckID:     t.CheckID,
			CurrentMode: ModeWaiting,
			NextCheckAt: time.Now(),
		}
		s.Checks[t.CheckID] = status
	}

	status.Mu.Lock()
	status.Domain = t.Domain
	status.Probe = t.Probe
	status.Label = t.Label
	status.Type = t.Type
	status.Heartbeat = t.Heartbeat
	status.Mu.Unlock()

	return status
}

// RemoveStatus deletes the status for a check from both the state map and database.
func (s *State) RemoveStatus(checkID string) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	delete(s.Checks, checkID)

	if db.GetAPIDB() != nil {
		globalWriteMu.Lock()
		defer globalWriteMu.Unlock()
		_ = db.DeleteMonitorStatus(checkID)
	}
}

// RecordHistory updates the history table with the current check result.
func RecordHistory(checkID, domain string, isUp bool, statusMsg string, errorCode int) {
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

	if err := db.RecordMonitorHistory(checkID, domain, statusText, errorCode); err != nil {
		log.Printf("Warning: %v\n", err)
	}
}
