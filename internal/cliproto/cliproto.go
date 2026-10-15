// Package cliproto holds the wire types and headers shared by jman-api and
// the jman CLI. It has no dependencies on the database or the API server,
// so the CLI can import it without pulling either in.
package cliproto

import (
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// Response headers jman-api sends on every response, including 304s and
// errors, so a CLI can read them whatever the body format.
const (
	// MinCLIVersionHeader is the oldest CLI version jman-api serves. Absent
	// when there is no minimum.
	MinCLIVersionHeader = "X-Jman-Min-CLI-Version"
	// LatestCLIVersionHeader is the newest CLI release.
	LatestCLIVersionHeader = "X-Jman-Latest-CLI-Version"
	// RefreshedAtHeader is when servers and sites were last refreshed from
	// SpinupWP (RFC 3339), sent with the snapshot.
	RefreshedAtHeader = "X-Jman-Refreshed-At"
)

// TokenPrefix starts every CLI token.
const TokenPrefix = "jman_cli_"

// SnapshotSchemaVersion is the format version of Snapshot. Bump it when a
// change would break an older CLI's reading of the snapshot.
const SnapshotSchemaVersion = 1

// Snapshot is everything the jman CLI mirrors locally, served by
// GET /api/cli/snapshot. It is the same for every user.
type Snapshot struct {
	SchemaVersion int               `json:"schema_version"`
	Servers       []Server          `json:"servers"`
	Sites         []Site            `json:"sites"`
	Plugins       []models.WPPlugin `json:"plugins"`
	PluginInfo    []PluginInfo      `json:"plugin_info"`
}

// Server is a server in the snapshot.
type Server struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	Name      string `json:"name"`
	IsLogical bool   `json:"is_logical"`
	IPAddress string `json:"ip_address"`
	SSHPort   int    `json:"ssh_port"`
}

// Site is a site in the snapshot. A manually added site replaces a
// SpinupWP site with the same domain, so domains are unique.
type Site struct {
	ID             string                     `json:"id"`
	Provider       string                     `json:"provider"`
	Domain         string                     `json:"domain"`
	ServerID       string                     `json:"server_id,omitempty"`
	ServerName     string                     `json:"server_name"`
	Environment    models.SiteEnvironmentType `json:"environment,omitempty"`
	Status         string                     `json:"status"`
	IsWordpress    bool                       `json:"is_wordpress"`
	ConnectionType string                     `json:"connection_type"`
	SSHHost        string                     `json:"ssh_host"`
	SSHPort        int                        `json:"ssh_port"`
	SSHUser        string                     `json:"ssh_user"`
	// SSH is the WP-CLI --ssh target ("user@host" or "user@host:port").
	SSH         string              `json:"ssh,omitempty"`
	SitePath    string              `json:"site_path"`
	CanWPCLI    bool                `json:"can_wp_cli"`
	WPCore      *models.SiteCore    `json:"wp_core,omitempty"`
	UpdateLocks []models.UpdateLock `json:"update_locks,omitempty"`
}

// PluginInfo is a plugin's display name and latest known version.
type PluginInfo struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Latest string `json:"latest,omitempty"`
}

// Health is the body of GET /api/health.
type Health struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	// WebURL is the web UI's base URL, where the CLI login is approved.
	WebURL string `json:"webUrl"`
}

// TokenRequest is the body of POST /api/cli/token.
type TokenRequest struct {
	Code     string `json:"code"`
	Verifier string `json:"verifier"`
}

// TokenUser is the user a CLI token was issued to.
type TokenUser struct {
	Username       string `json:"username"`
	DisplayName    string `json:"displayName"`
	Level          string `json:"level"`
	RestrictedFrom string `json:"restrictedFrom,omitempty"`
}

// TokenResponse is the body of a successful POST /api/cli/token.
type TokenResponse struct {
	Token      string    `json:"token"`
	DeviceName string    `json:"device_name"`
	ExpiresAt  time.Time `json:"expires_at"`
	User       TokenUser `json:"user"`
}

// RefreshResult is the body of POST /api/refresh.
type RefreshResult struct {
	Servers     int       `json:"servers"`
	Sites       int       `json:"sites"`
	RefreshedAt time.Time `json:"refreshed_at"`
	// Skipped is true when the server reused a refresh that finished
	// moments ago instead of fetching again.
	Skipped bool `json:"skipped"`
}
