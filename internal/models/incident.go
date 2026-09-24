package models

import "time"

// Incident status constants.
const (
	IncidentStatusOpen         = "open"
	IncidentStatusAcknowledged = "acknowledged"
	IncidentStatusResolved     = "resolved"
	IncidentStatusClosed       = "closed"
)

// Incident represents an outage incident for a monitored site.
type Incident struct {
	ID             int64      `json:"id"`
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
