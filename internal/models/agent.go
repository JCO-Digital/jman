package models

// AgentToken represents a per-server credential used by jman-agent to
// authenticate to jman-api. The plaintext secret is never stored — only its
// bcrypt hash — so TokenHash is deliberately omitted from this struct, which
// is used for API/CLI responses.
type AgentToken struct {
	ID          int     `json:"id"`
	ServerID    int     `json:"server_id"`
	ServerName  string  `json:"server_name"`
	TokenPrefix string  `json:"token_prefix"`
	Description *string `json:"description"`
	Revoked     bool    `json:"revoked"`
	LastSeenAt  *string `json:"last_seen_at"`
	CreatedAt   string  `json:"created_at"`
	CreatedBy   string  `json:"created_by"`
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
type AgentManifestSite struct {
	SiteID int    `json:"site_id"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

// AgentManifest is the response body for GET /api/agent/manifest.
type AgentManifest struct {
	ServerID int                 `json:"server_id"`
	Sites    []AgentManifestSite `json:"sites"`
}

// AgentReportSite is a single site's worth of freshly collected data in a
// POST /api/agent/report request body.
type AgentReportSite struct {
	SiteID           int   `json:"site_id"`
	DiskUsageBytes   *int64 `json:"disk_usage_bytes"`
	IsMultisite      *bool  `json:"is_multisite"`
	DisallowFileMods *bool  `json:"disallow_file_mods"`
}

// AgentReport is the request body jman-agent POSTs on each collection cycle.
type AgentReport struct {
	CollectedAt string            `json:"collected_at"`
	Sites       []AgentReportSite `json:"sites"`
}
