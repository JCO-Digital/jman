package models

import "time"

// MonitorHistory represents a record in the monitor_history table.
type MonitorHistory struct {
	ID        int       `json:"id"`
	CheckID   string    `json:"check_id"`
	Domain    string    `json:"domain"`
	Status    string    `json:"status"`
	ErrorCode int       `json:"error_code"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Count     int       `json:"count"`
}

// MonitorStatus represents a record in the monitor_check_status table.
type MonitorStatus struct {
	CheckID              string     `json:"check_id"`
	Domain               string     `json:"domain"`
	IsDown               bool       `json:"is_down"`
	FailureCount         int        `json:"failure_count"`
	ConsecutiveSuccesses int        `json:"consecutive_successes"`
	CurrentMode          string     `json:"current_mode"`
	LastAlertTime        *time.Time `json:"last_alert_time,omitempty"`
	LastChecked          time.Time  `json:"last_checked"`
	NextCheckAt          time.Time  `json:"next_check_at"`
}

// Monitor check types.
const (
	MonitorCheckTypeHTTP = "http"
)

// Monitor check expectations: what a 2xx response body must look like.
const (
	MonitorExpectHTML = "html"
)

// MonitorCheck is one thing the uptime monitor checks. A default check
// belongs to a site, which supplies its host; other checks carry their own.
type MonitorCheck struct {
	ID         string     `json:"id"`
	SiteID     string     `json:"site_id,omitempty"` // empty = not linked to a site
	IsDefault  bool       `json:"is_default"`
	Type       string     `json:"type"`
	Name       string     `json:"name,omitempty"`
	Host       string     `json:"host,omitempty"` // empty on default checks: the site's domain
	Path       string     `json:"path"`
	Expect     string     `json:"expect"`
	Enabled    bool       `json:"enabled"`
	OrphanedAt *time.Time `json:"orphaned_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	CreatedBy  string     `json:"created_by,omitempty"`
	UpdatedAt  time.Time  `json:"updated_at"`
	UpdatedBy  string     `json:"updated_by,omitempty"`
}

// MonitorIgnoredHistory represents a record in the monitor_ignored_history table.
type MonitorIgnoredHistory struct {
	ID        int       `json:"id"`
	Domain    string    `json:"domain"`
	Action    string    `json:"action"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// IgnoredSite represents a record in the monitor_ignored_sites table.
// This is used for database interactions. Note that a similar struct exists
// in site.go but with string types for serialization from external APIs.
type IgnoredSite struct {
	Domain    string    `json:"domain"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}
