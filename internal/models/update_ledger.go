package models

import "time"

// SiteUpdateLedgerEntry represents an entry in the update ledger for a specific site.
type SiteUpdateLedgerEntry struct {
	ID         int       `json:"id"`
	SiteID     int       `json:"site_id"`
	UpdateType string    `json:"update_type"` // "core", "plugin", "theme"
	Status     string    `json:"status"`      // "full", "partial", "failed"
	DataJSON   string    `json:"data_json,omitempty"`
	UpdatedBy  string    `json:"updated_by"`
	UpdatedAt  time.Time `json:"updated_at"`
}
