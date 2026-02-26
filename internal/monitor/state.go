package monitor

import (
	"os"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
)

const monitorStateFile = "monitor_state"

// SiteStatus tracks the monitoring state for an individual site.
type SiteStatus struct {
	FailureCount  int       `json:"failure_count"`
	LastAlertTime time.Time `json:"last_alert_time"`
	IsDown        bool      `json:"is_down"`
}

// State represents the overall monitoring state for all sites.
type State struct {
	Sites map[string]*SiteStatus `json:"sites"`
}

// LoadState reads the monitor state from the data directory.
func LoadState() (*State, error) {
	var state State
	err := cache.ReadJSONData(monitorStateFile, &state)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{
				Sites: make(map[string]*SiteStatus),
			}, nil
		}
		return nil, err
	}
	if state.Sites == nil {
		state.Sites = make(map[string]*SiteStatus)
	}
	return &state, nil
}

// SaveState writes the current monitor state to the data directory.
func (s *State) SaveState() error {
	return cache.WriteJSONData(monitorStateFile, s)
}

// GetStatus returns the status for a given domain, creating it if it doesn't exist.
func (s *State) GetStatus(domain string) *SiteStatus {
	if status, ok := s.Sites[domain]; ok {
		return status
	}
	status := &SiteStatus{}
	s.Sites[domain] = status
	return status
}

// RemoveStatus deletes the status for a given domain.
func (s *State) RemoveStatus(domain string) {
	delete(s.Sites, domain)
}
