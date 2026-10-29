package models

import "time"

// Vulnerability exposure kinds: a plugin, or WordPress core.
const (
	VulnExposurePlugin = "plugin"
	VulnExposureCore   = "core"
)

// CoreExposureSlug is the slug of every core exposure.
const CoreExposureSlug = "wordpress"

// Fix states of a vulnerability exposure.
const (
	// VulnFixAvailable means every applicable vulnerability has a fixed
	// version (or, for core, the site has a core update).
	VulnFixAvailable = "fix_available"
	// VulnNoFix means some applicable vulnerability has no known fix.
	VulnNoFix = "no_fix"
)

// VulnExposure is one site running a vulnerable version of a plugin (or of
// WordPress core). It stays open while the site runs an affected version
// and is resolved when it no longer does.
type VulnExposure struct {
	ID      int64  `json:"id"`
	Kind    string `json:"kind"`
	Slug    string `json:"slug"`
	SiteID  string `json:"site_id"`
	Version string `json:"version"`
	// VulnUUIDs are the unsuppressed vulnerabilities that apply to Version,
	// sorted.
	VulnUUIDs []string `json:"vuln_uuids"`
	// MaxCvss is the highest CVSS score among them; nil when none is scored.
	MaxCvss  *float64 `json:"max_cvss"`
	FixState string   `json:"fix_state"`
	// FixedIn is the lowest version that fixes every applicable
	// vulnerability; empty when FixState is VulnNoFix.
	FixedIn string `json:"fixed_in,omitempty"`
	// PossibleFixVersion, on a no-fix exposure, is a newer available
	// version that is outside every applicable vulnerability's affected
	// range. It may or may not fix them.
	PossibleFixVersion string `json:"possible_fix_version,omitempty"`
	// FixAvailableSince is when the exposure last became fix-available;
	// the overdue clock runs from it. Nil while FixState is VulnNoFix.
	FixAvailableSince *time.Time `json:"fix_available_since"`
	FirstSeenAt       time.Time  `json:"first_seen_at"`
	LastSeenAt        time.Time  `json:"last_seen_at"`
	ResolvedAt        *time.Time `json:"resolved_at"`
}

// Vulnerability mute statuses.
const (
	VulnMuteActive = "active"
	// VulnMuteNeedsReview is a mute past its review date: it still hides
	// the exposure from the overdue count until someone confirms it again
	// or unmutes it.
	VulnMuteNeedsReview = "needs_review"
	VulnMuteLifted      = "lifted"
)

// VulnMuteReviewDays is how long a mute holds before it needs review.
const VulnMuteReviewDays = 90

// VulnMute accepts the risk of one plugin (or core) exposure on one site
// for a while: the exposure doesn't count as overdue. A mute is lifted
// when a new vulnerability applies, when a no-fix exposure gets a fix or a
// possibly fixing version, when the exposure is gone, or by hand.
type VulnMute struct {
	ID     int64  `json:"id"`
	Kind   string `json:"kind"`
	Slug   string `json:"slug"`
	SiteID string `json:"site_id"`
	Reason string `json:"reason"`
	Status string `json:"status"`
	// The exposure as it was when the mute was made or last confirmed.
	FixStateAtMute    string     `json:"fix_state_at_mute"`
	VulnUUIDsAtMute   []string   `json:"vuln_uuids_at_mute"`
	PossibleFixAtMute string     `json:"possible_fix_at_mute,omitempty"`
	ReviewAt          time.Time  `json:"review_at"`
	LiftedAt          *time.Time `json:"lifted_at"`
	LiftedReason      string     `json:"lifted_reason,omitempty"`
	AuditFields
	// Set by the API only.
	SiteName string `json:"site_name,omitempty"`
	Name     string `json:"name,omitempty"`
	Version  string `json:"version,omitempty"`
}

// Open reports whether the mute still applies (active or needing review).
func (m VulnMute) Open() bool {
	return m.Status == VulnMuteActive || m.Status == VulnMuteNeedsReview
}

// VulnExposureSite is an exposure as the API returns it, with the site's
// name, its overdue state and its open mute, if any. A muted exposure is
// never counted as overdue.
type VulnExposureSite struct {
	VulnExposure
	SiteName string     `json:"site_name"`
	DueAt    *time.Time `json:"due_at"`
	Overdue  bool       `json:"overdue"`
	Mute     *VulnMute  `json:"mute,omitempty"`
}

// VulnExposureVuln describes one vulnerability in an exposure group.
type VulnExposureVuln struct {
	UUID    string   `json:"uuid"`
	Name    string   `json:"name"`
	Cvss    *float64 `json:"cvss"`
	FixedIn string   `json:"fixed_in,omitempty"`
	Unfixed bool     `json:"unfixed"`
	Link    string   `json:"link,omitempty"`
}

// VulnExposureGroup is a vulnerable plugin, or a vulnerable WordPress core
// version, with the sites exposed to it.
type VulnExposureGroup struct {
	Kind string `json:"kind"`
	Slug string `json:"slug"`
	// Version is set for core groups, which are one per core version.
	Version string `json:"version,omitempty"`
	Name    string `json:"name"`
	// Closed, ClosedReason and ClosedDate say whether the plugin was closed
	// on wordpress.org.
	Closed       bool   `json:"closed"`
	ClosedReason string `json:"closed_reason,omitempty"`
	ClosedDate   string `json:"closed_date,omitempty"`
	// LastUpdated is when the plugin was last updated, per the feed.
	LastUpdated       *time.Time `json:"last_updated,omitempty"`
	MaxCvss           *float64   `json:"max_cvss"`
	FixAvailableCount int        `json:"fix_available_count"`
	NoFixCount        int        `json:"no_fix_count"`
	// OverdueCount counts unmuted overdue sites.
	OverdueCount    int                `json:"overdue_count"`
	MutedCount      int                `json:"muted_count"`
	Vulnerabilities []VulnExposureVuln `json:"vulnerabilities"`
	Sites           []VulnExposureSite `json:"sites"`
}

// VulnExposureSummary counts open exposures, for badges. Overdue and NoFix
// leave muted exposures out.
type VulnExposureSummary struct {
	Groups      int `json:"groups"`
	Exposures   int `json:"exposures"`
	Overdue     int `json:"overdue"`
	NoFix       int `json:"no_fix"`
	Muted       int `json:"muted"`
	NeedsReview int `json:"needs_review"`
}
