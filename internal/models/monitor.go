package models

import (
	"encoding/json"
	"time"
)

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
	// LastStartedAt is when a heartbeat check's job last reported starting.
	LastStartedAt *time.Time `json:"last_started_at,omitempty"`
}

// Monitor check types.
const (
	MonitorCheckTypeHTTP      = "http"
	MonitorCheckTypeHeartbeat = "heartbeat" // an external job checks in
)

// Monitor check expectations: what a 2xx response body must look like.
const (
	MonitorExpectHTML = "html"
	MonitorExpectJSON = "json"
)

// Monitor check assertion operators.
const (
	MonitorAssertEquals = "equals"
	MonitorAssertExists = "exists"
)

// MonitorCheck is one thing the uptime monitor checks. A default check
// belongs to a site, which supplies its host; other checks carry their own.
type MonitorCheck struct {
	ID        string `json:"id"`
	SiteID    string `json:"site_id,omitempty"` // empty = not linked to a site
	IsDefault bool   `json:"is_default"`
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"`
	Host      string `json:"host,omitempty"` // empty on default checks: the site's domain
	Path      string `json:"path"`
	Expect    string `json:"expect"`
	// AssertPath, AssertOp and AssertValue are one optional assertion on a
	// JSON body. AssertValue is JSON, so true and "true" differ.
	AssertPath  string          `json:"assert_path,omitempty"`
	AssertOp    string          `json:"assert_op,omitempty"`
	AssertValue json.RawMessage `json:"assert_value,omitempty"`
	// CacheBypass overrides the global monitorCacheBypass setting; nil
	// follows it.
	CacheBypass *bool `json:"cache_bypass,omitempty"`
	// IntervalSeconds, GraceSeconds and FailThreshold configure a
	// heartbeat check: it is down when no check-in finished within
	// interval + grace of the last one, or after FailThreshold failed
	// check-ins in a row.
	IntervalSeconds int  `json:"interval_seconds,omitempty"`
	GraceSeconds    int  `json:"grace_seconds,omitempty"`
	FailThreshold   int  `json:"fail_threshold,omitempty"`
	Enabled         bool `json:"enabled"`
	// PausedByRuleID is the rule that started this default check paused;
	// cleared when the check is resumed or the rule deleted.
	PausedByRuleID string     `json:"paused_by_rule_id,omitempty"`
	OrphanedAt     *time.Time `json:"orphaned_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	CreatedBy      string     `json:"created_by,omitempty"`
	UpdatedAt      time.Time  `json:"updated_at"`
	UpdatedBy      string     `json:"updated_by,omitempty"`
}

// Monitor check rule match types.
const (
	MonitorRuleMatchServer = "server" // Value is a server UUID
	MonitorRuleMatchHost   = "host"   // Value is a glob on the site's domain
)

// MonitorRuleActionStartPaused is the action of a rule that creates
// matching sites' default checks paused.
const MonitorRuleActionStartPaused = "start_paused"

// MonitorCheckRule decides how new default checks are created: a site
// matching it gets its check with Action applied. It never changes
// existing checks.
type MonitorCheckRule struct {
	ID        string    `json:"id"`
	MatchType string    `json:"match_type"`
	Value     string    `json:"value"`
	Action    string    `json:"action"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by,omitempty"`
}

// MonitorCheckSecret describes a secret header sent with a check's
// requests. Its value is never returned by the API.
type MonitorCheckSecret struct {
	CheckID   string    `json:"-"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by,omitempty"`
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
