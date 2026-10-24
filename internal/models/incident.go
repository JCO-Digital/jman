package models

import "time"

// Incident status constants.
const (
	IncidentStatusOpen         = "open"
	IncidentStatusAcknowledged = "acknowledged"
	IncidentStatusResolved     = "resolved"
	IncidentStatusClosed       = "closed"
)

// Incident types. An uptime incident follows a monitor check's outage; a
// security incident groups a site's file scan findings and only a person
// closes it.
const (
	IncidentTypeUptime   = "uptime"
	IncidentTypeSecurity = "security"
)

// Incident represents an outage of a monitored site, or a security
// incident on a scanned one.
type Incident struct {
	ID             int64      `json:"id"`
	Type           string     `json:"type"`
	CheckID        string     `json:"check_id"`
	SiteID         string     `json:"site_id,omitempty"`
	Severity       string     `json:"severity,omitempty"`
	Domain         string     `json:"domain"`
	Status         string     `json:"status"` // open, acknowledged, resolved, closed
	ErrorMessage   string     `json:"error_message"`
	ErrorCode      int        `json:"error_code"`
	DownSince      time.Time  `json:"down_since"`
	AcknowledgedBy *string    `json:"acknowledged_by,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	ResolvedBy     *string    `json:"resolved_by,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	PDTriggered    bool       `json:"pd_triggered"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
