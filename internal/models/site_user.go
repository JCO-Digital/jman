package models

// MaxSiteUsers is how many users are kept for one site. Sites with more
// (beyond the excluded roles, see SiteUsers) keep the first MaxSiteUsers by
// ID and are marked Truncated, which keeps agent reports and the CLI
// snapshot bounded.
const MaxSiteUsers = 1000

// SiteUser is a WordPress user of a site.
type SiteUser struct {
	SiteID      string `json:"site_id,omitempty"` // site UUID
	ID          int    `json:"id"`
	Login       string `json:"login"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	// Roles is wp-cli's comma-separated role list, e.g. "editor,author".
	Roles      string `json:"roles"`
	Registered string `json:"registered"`
	// PostCount is how many posts of any type and status (revisions and
	// attachments included) the user authored: what deleting the user
	// would delete unless they're reassigned. Nil if unknown, e.g. from an
	// agent that doesn't count them yet.
	PostCount *int `json:"post_count,omitempty"`
}

// SiteUsers is a site's users, without the roles that tend to pile up by
// the thousand (subscribers, WooCommerce customers), which are only
// counted.
type SiteUsers struct {
	Users []SiteUser `json:"users"`
	// ExcludedCounts is the number of users by excluded role.
	ExcludedCounts map[string]int `json:"excluded_counts"`
	// Truncated says the site has more than MaxSiteUsers users to list.
	Truncated bool `json:"truncated,omitempty"`
}

// SiteUserSummary is what is known about a site's users besides the
// listed ones: the excluded roles' counts and when they were read.
type SiteUserSummary struct {
	SiteID         string         `json:"site_id"`
	ExcludedCounts map[string]int `json:"excluded_counts"`
	Truncated      bool           `json:"truncated,omitempty"`
	// UpdatedAt is when the users were read (RFC3339, UTC).
	UpdatedAt string `json:"updated_at"`
}
