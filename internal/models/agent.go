package models

// AgentToken represents a per-server credential used by jman-agent to
// authenticate to jman-api. The plaintext secret is never stored — only its
// bcrypt hash — so TokenHash is deliberately omitted from this struct, which
// is used for API/CLI responses.
type AgentToken struct {
	ID           int     `json:"id"`
	ServerID     int     `json:"server_id"`
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
	IsMultisite      bool   `json:"is_multisite"`
	DisallowFileMods bool   `json:"disallow_file_mods"`
	UpdatedAt        string `json:"updated_at"`
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
	SiteID   int    `json:"site_id"`
	Domain   string `json:"domain"`
	SiteUser string `json:"site_user"`
}

// AgentManifest is the response body for GET /api/agent/manifest.
type AgentManifest struct {
	ServerID int                 `json:"server_id"`
	Sites    []AgentManifestSite `json:"sites"`
	// APIVersion is jman-api's own running version. Every binary in this
	// repo shares one version (one git tag per release), so if this is
	// newer than the agent's own version, a newer jman-agent release should
	// exist too — the agent uses this to trigger an immediate self-update
	// check instead of waiting for its periodic ticker.
	APIVersion string `json:"api_version"`
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

// AgentReportSite is a single site's worth of freshly collected data in a
// POST /api/agent/report request body.
type AgentReportSite struct {
	SiteID           int                  `json:"site_id"`
	DiskUsageBytes   *int64               `json:"disk_usage_bytes"`
	IsMultisite      *bool                `json:"is_multisite"`
	DisallowFileMods *bool                `json:"disallow_file_mods"`
	TrafficHourly    []TrafficHourlyEntry `json:"traffic_hourly,omitempty"`
	TrafficDaily     []TrafficDailyEntry  `json:"traffic_daily,omitempty"`
}

// AgentReport is the request body jman-agent POSTs on each collection cycle.
type AgentReport struct {
	CollectedAt  string            `json:"collected_at"`
	AgentVersion string            `json:"agent_version,omitempty"`
	Sites        []AgentReportSite `json:"sites"`
}
