package models

import "time"

// IgnoreEntry represents a unified ignore rule for sites, servers, plugins, or vulnerabilities.
type IgnoreEntry struct {
	ID             int       `json:"id"`
	Type           string    `json:"type"`             // site, server, plugin, vulnerability
	Target         string    `json:"target"`           // SpinupWP ID, slug, or UUID
	Reason         string    `json:"reason"`           // Freetext explanation
	NegatedSiteIDs []int     `json:"negated_site_ids"` // Site IDs to exclude from a server-wide ignore
	UseForMonitor  bool      `json:"use_for_monitor"`  // Applies to uptime monitoring
	UseForVuln     bool      `json:"use_for_vuln"`     // Applies to vulnerability scanning
	CreatedAt      time.Time `json:"created_at"`
	CreatedBy      string    `json:"created_by"`
	UpdatedAt      time.Time `json:"updated_at"`
	UpdatedBy      string    `json:"updated_by"`
}
