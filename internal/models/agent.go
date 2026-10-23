package models

// AgentToken represents a per-server credential used by jman-agent to
// authenticate to jman-api. The plaintext secret is never stored — only its
// bcrypt hash — so TokenHash is deliberately omitted from this struct, which
// is used for API/CLI responses.
type AgentToken struct {
	ID           int     `json:"id"`
	ServerID     string  `json:"server_id"` // server UUID
	ServerName   string  `json:"server_name"`
	TokenPrefix  string  `json:"token_prefix"`
	Description  *string `json:"description"`
	Revoked      bool    `json:"revoked"`
	LastSeenAt   *string `json:"last_seen_at"`
	AgentVersion *string `json:"agent_version"`
	CreatedAt    string  `json:"created_at"`
	CreatedBy    string  `json:"created_by"`
	// StaleAlertSentAt is internal bookkeeping for the staleness alert in
	// internal/tasks/agent_health.go — deliberately excluded from API/CLI
	// responses via json:"-", same treatment as TokenHash above.
	StaleAlertSentAt *string `json:"-"`
}

// SiteDiskUsage is the most recently reported disk usage for a single site,
// as measured locally by jman-agent (distinct from the server-level
// DiskSpace field on Server, which comes from SpinupWP).
type SiteDiskUsage struct {
	BytesUsed  int64  `json:"bytes_used"`
	MeasuredAt string `json:"measured_at"`
}

// SiteWpFlags holds the current WordPress configuration flags jman-agent
// reads directly from wp-config.php on the server.
type SiteWpFlags struct {
	IsMultisite      bool `json:"is_multisite"`
	DisallowFileMods bool `json:"disallow_file_mods"`
	// AutoUpdateCore is WordPress's core auto-update setting: "true"
	// (major updates too), "minor", "false", "disabled" (all automatic
	// updates off), "default" (minor only), or "" if unknown (an older
	// agent that doesn't report it).
	AutoUpdateCore string `json:"auto_update_core"`
	UpdatedAt      string `json:"updated_at"`
}

// AgentManifestSite is a single site entry returned to jman-agent by the
// manifest endpoint, describing what it should collect data for locally.
// jman-api deliberately does not compute an absolute filesystem path here —
// it has no filesystem access to the managed server to verify one exists.
// jman-agent runs locally as root instead, and resolves the actual path
// itself (see agent.ResolveSitePath): primarily via SpinupWP's standard
// /sites/<domain>/files layout, falling back to a local OS user lookup on
// SiteUser for servers provisioned with a dedicated user per site.
// (SpinupWP's own "public_folder" field describes the web-server-exposed
// docroot, which can differ from the WordPress install location, so it's
// deliberately not used here.)
type AgentManifestSite struct {
	SiteID string `json:"site_id"` // site UUID
	// LegacySiteID is the SpinupWP integer ID (omitted for non-SpinupWP
	// sites). jman-agent only uses it to rename log-state files written by
	// pre-UUID agent releases, which keyed them by this integer.
	LegacySiteID int    `json:"legacy_site_id,omitempty"`
	Domain       string `json:"domain"`
	SiteUser     string `json:"site_user"`
	IsWordpress  bool   `json:"is_wordpress"`
	// WPDataHash is the WPDataHash of the plugins and core state jman-api
	// holds for the site ("" if none). The agent sends its collected
	// WordPress data in full only when its own hash differs.
	WPDataHash string `json:"wp_data_hash,omitempty"`
}

// AgentManifest is the response body for GET /api/agent/manifest.
type AgentManifest struct {
	ServerID string              `json:"server_id"` // server UUID
	Sites    []AgentManifestSite `json:"sites"`
	// APIVersion is jman-api's own running version. Every binary in this
	// repo shares one version (one git tag per release), so if this is
	// newer than the agent's own version, a newer jman-agent release should
	// exist too — the agent uses this to trigger an immediate self-update
	// check instead of waiting for its periodic ticker.
	APIVersion string `json:"api_version"`
	// WPDataIntervalMinutes is how often the agent collects each site's
	// WordPress data (plugins and core) with wp-cli. Zero means don't:
	// jman-api versions that predate agent collection don't send it.
	WPDataIntervalMinutes int `json:"wp_data_interval_minutes,omitempty"`
	// CollectUsers asks the agent to include each site's users in its
	// WordPress data. jman-api versions that predate it don't send it, and
	// would reject a hash that includes users.
	CollectUsers bool `json:"collect_users,omitempty"`
	// ScanIntervalMinutes is how often the agent scans each site's files
	// for signs of compromise. Zero means don't: jman-api versions that
	// predate scanning don't send it.
	ScanIntervalMinutes int `json:"scan_interval_minutes,omitempty"`
}

// TrafficTopEntry is a single ranked entry (a page path or a referrer) in a
// top-N list, truncated agent-side before sending.
type TrafficTopEntry struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// TrafficHourlyEntry is one fully-elapsed hour's worth of visitor traffic
// for a site, as aggregated locally by jman-agent from its access logs.
// jman-agent only ever sends a given hour once it's closed (the wall clock
// has moved past it), so jman-api can persist these with a plain
// replace-style upsert — no incremental merging required.
type TrafficHourlyEntry struct {
	Hour           string            `json:"hour"` // RFC3339, truncated to the hour, UTC
	RequestsTotal  int               `json:"requests_total"`
	RequestsHuman  int               `json:"requests_human"`
	RequestsBot    int               `json:"requests_bot"`
	UniqueVisitors int               `json:"unique_visitors"`
	TopPages       []TrafficTopEntry `json:"top_pages"`
	TopReferrers   []TrafficTopEntry `json:"top_referrers"`
	// StatusCodes is a raw connection count per HTTP status code (e.g.
	// "200", "404"), independent of TopPages — every request counts here,
	// including ones excluded from TopPages (non-200 or a WordPress system
	// path).
	StatusCodes []TrafficTopEntry `json:"status_codes"`
}

// TrafficDailyEntry is one full calendar day's worth of visitor traffic for
// a site, aggregated locally by jman-agent directly from its access logs.
// It's only used for backlog older than jman-api's site_traffic_hourly
// retention window (see internal/tasks/scheduler.go) — sending such a day
// as 24 individual TrafficHourlyEntry values just to have jman-api roll
// them up and immediately prune the hourly detail would waste report
// budget and bandwidth for no benefit. jman-api stores these directly in
// site_traffic_daily; there is deliberately no site_traffic_hourly data
// backing them.
type TrafficDailyEntry struct {
	Day            string            `json:"day"` // YYYY-MM-DD, UTC
	RequestsTotal  int               `json:"requests_total"`
	RequestsHuman  int               `json:"requests_human"`
	RequestsBot    int               `json:"requests_bot"`
	UniqueVisitors int               `json:"unique_visitors"`
	TopPages       []TrafficTopEntry `json:"top_pages"`
	TopReferrers   []TrafficTopEntry `json:"top_referrers"`
	StatusCodes    []TrafficTopEntry `json:"status_codes"`
}

// SiteTrafficPeriod is a single hourly or daily traffic data point returned
// by GET /api/sites/{id}/traffic.
type SiteTrafficPeriod struct {
	PeriodStart    string            `json:"period_start"`
	RequestsTotal  int               `json:"requests_total"`
	RequestsHuman  int               `json:"requests_human"`
	RequestsBot    int               `json:"requests_bot"`
	UniqueVisitors int               `json:"unique_visitors"`
	TopPages       []TrafficTopEntry `json:"top_pages"`
	TopReferrers   []TrafficTopEntry `json:"top_referrers"`
	StatusCodes    []TrafficTopEntry `json:"status_codes"`
}

// SiteTrafficDailyRow is a single site's daily traffic row, used by the
// cross-site traffic report — unlike SiteTrafficPeriod (a single site's own
// breakdown), this carries the site ID so report rows from multiple sites
// can be enriched with a site label after the query.
type SiteTrafficDailyRow struct {
	SiteID         string `json:"site_id"` // site UUID
	Day            string `json:"day"`
	RequestsTotal  int    `json:"requests_total"`
	RequestsHuman  int    `json:"requests_human"`
	RequestsBot    int    `json:"requests_bot"`
	UniqueVisitors int    `json:"unique_visitors"`
}

// AgentReportSite is a single site's worth of freshly collected data in a
// POST /api/agent/report request body.
type AgentReportSite struct {
	SiteID           string  `json:"site_id"` // site UUID
	DiskUsageBytes   *int64  `json:"disk_usage_bytes"`
	IsMultisite      *bool   `json:"is_multisite"`
	DisallowFileMods *bool   `json:"disallow_file_mods"`
	AutoUpdateCore   *string `json:"auto_update_core,omitempty"` // see SiteWpFlags.AutoUpdateCore
	// WPData is set on cycles where the agent collected the site's
	// WordPress data (about every WPDataIntervalMinutes).
	WPData        *AgentWPData         `json:"wp_data,omitempty"`
	TrafficHourly []TrafficHourlyEntry `json:"traffic_hourly,omitempty"`
	TrafficDaily  []TrafficDailyEntry  `json:"traffic_daily,omitempty"`
	// Scan is set on cycles where the agent scanned the site's files
	// (about every ScanIntervalMinutes).
	Scan *AgentScan `json:"scan,omitempty"`
}

// ScanFinding is one thing a file scan flagged. Findings describe changes:
// the agent reports a file once, when it appears or changes, not on every
// scan it is still there.
type ScanFinding struct {
	// Engine is what produced the finding: "builtin" for the agent's own
	// rules, "api" for ones jman-api raises itself.
	Engine   string `json:"engine"`
	Rule     string `json:"rule"`
	Severity string `json:"severity"` // low, medium, high or critical
	Path     string `json:"path"`     // relative to the site's directory
	SHA256   string `json:"sha256,omitempty"`
	// Snippet is the start of the file, for triage. It's left out for
	// files that hold secrets, such as wp-config.php.
	Snippet string `json:"snippet,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// AgentScan is one file scan of a site, as reported by jman-agent.
type AgentScan struct {
	ScannedAt      string `json:"scanned_at"` // RFC3339, UTC
	RuleSetVersion string `json:"rule_set_version"`
	// Fingerprint is a hash of the agent's baseline after this scan, and
	// PrevFingerprint of the one it compared against. jman-api checks the
	// latter against the fingerprint it last stored, to notice a baseline
	// that was reset or changed behind its back.
	Fingerprint     string `json:"fingerprint,omitempty"`
	PrevFingerprint string `json:"prev_fingerprint,omitempty"`
	// Rebaselined is set when the agent had no baseline for the site (the
	// first scan, or its state was lost) and recorded one without
	// reporting findings.
	Rebaselined bool `json:"rebaselined,omitempty"`
	// Entries is how many files the baseline holds, and Omitted how many
	// more the scan skipped because the baseline was full.
	Entries    int           `json:"entries"`
	Omitted    int           `json:"omitted,omitempty"`
	DurationMs int64         `json:"duration_ms"`
	Findings   []ScanFinding `json:"findings,omitempty"`
	// FindingsOmitted counts findings left out to keep the report small.
	FindingsOmitted int `json:"findings_omitted,omitempty"`
	// Error is set when the scan failed; nothing else is then meaningful.
	Error string `json:"error,omitempty"`
}

// SiteScan is the latest file scan jman-api holds for a site.
type SiteScan struct {
	SiteID         string  `json:"site_id"`
	ScannedAt      string  `json:"scanned_at"`
	RuleSetVersion string  `json:"rule_set_version"`
	Entries        int     `json:"entries"`
	Omitted        int     `json:"omitted"`
	DurationMs     int64   `json:"duration_ms"`
	Error          string  `json:"error,omitempty"`
	RebaselinedAt  *string `json:"rebaselined_at,omitempty"`
}

// StoredScanFinding is a scan finding as jman-api stores it. The same
// site, rule, path and hash is one finding, however often it's reported.
type StoredScanFinding struct {
	ID     int64  `json:"id"`
	SiteID string `json:"site_id"`
	ScanFinding
	RuleSetVersion string `json:"rule_set_version"`
	FirstSeenAt    string `json:"first_seen_at"`
	LastSeenAt     string `json:"last_seen_at"`
}

// AgentReport is the request body jman-agent POSTs on each collection cycle.
type AgentReport struct {
	CollectedAt  string            `json:"collected_at"`
	AgentVersion string            `json:"agent_version,omitempty"`
	Sites        []AgentReportSite `json:"sites"`
}
