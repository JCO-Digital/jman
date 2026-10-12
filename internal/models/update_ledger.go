package models

import "time"

// Update ledger statuses.
const (
	// LedgerFull means the site was brought up to date: every update
	// succeeded and no plugin updates are left.
	LedgerFull = "full"
	// LedgerVuln means every update succeeded and the job fixed the site's
	// vulnerable plugins (none with an update is left), but other plugin
	// updates remain.
	LedgerVuln = "vuln"
	// LedgerPartial means everything requested succeeded but updates
	// (including vulnerable ones) remain, or a plugin management action
	// worked for only some of its plugins.
	LedgerPartial = "partial"
	LedgerFailed  = "failed"

	// Plugin management statuses: every plugin of the job ended up in the
	// expected state.
	LedgerActivated   = "activated"
	LedgerDeactivated = "deactivated"
	// LedgerDeleted covers both deleting a plugin's files and uninstalling
	// it (the entry's data says which).
	LedgerDeleted   = "deleted"
	LedgerInstalled = "installed"

	// LedgerDetected is a change jman found on the site but didn't make
	// itself (e.g. an update in wp-admin or a WordPress auto-update).
	LedgerDetected = "detected"
)

// SiteUpdateLedgerEntry represents an entry in the update ledger for a specific site.
type SiteUpdateLedgerEntry struct {
	ID         int       `json:"id"`
	SiteID     string    `json:"site_id"`     // site UUID
	UpdateType string    `json:"update_type"` // "core", "plugin", "theme"
	Status     string    `json:"status"`      // one of the Ledger* statuses
	DataJSON   string    `json:"data_json,omitempty"`
	UpdatedBy  string    `json:"updated_by"`
	UpdatedAt  time.Time `json:"updated_at"`
}
