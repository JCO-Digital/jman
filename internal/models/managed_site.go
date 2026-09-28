package models

import "fmt"

// ManagedServer represents a host-agnostic server entity stored in inventory.db.
// It can be a physical host with root access or a logical placeholder (e.g. "WPEngine").
type ManagedServer struct {
	ID               string `json:"id"`                           // UUID
	Provider         string `json:"provider"`                     // 'spinupwp', 'manual', 'wpengine', etc.
	ProviderServerID string `json:"provider_server_id,omitempty"` // SpinupWP server ID if applicable
	Name             string `json:"name"`
	IsLogical        bool   `json:"is_logical"`
	IPAddress        string `json:"ip_address,omitempty"`
	SSHPort          int    `json:"ssh_port"`
	CreatedAt        string `json:"created_at,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

// ManagedSite represents a host-agnostic WordPress site entity stored in inventory.db.
type ManagedSite struct {
	ID             string              `json:"id"`                         // UUID
	ServerID       *string             `json:"server_id,omitempty"`        // UUID or nil
	ServerName     string              `json:"server_name,omitempty"`      // Denormalized name for display
	Provider       string              `json:"provider"`                   // 'spinupwp', 'manual', 'wpengine', etc.
	ProviderSiteID string              `json:"provider_site_id,omitempty"` // SpinupWP site ID if applicable
	Domain         string              `json:"domain"`
	Environment    SiteEnvironmentType `json:"environment"`
	IsWordpress    bool                `json:"is_wordpress"`
	PHPVersion     string              `json:"php_version,omitempty"`

	// Connection settings
	ConnectionType string `json:"connection_type"` // 'agent', 'ssh', 'none'
	SSHHost        string `json:"ssh_host"`
	SSHPort        int    `json:"ssh_port"`
	SSHUser        string `json:"ssh_user"`
	SitePath       string `json:"site_path"`

	// Capabilities
	CanWPCLI      bool `json:"can_wp_cli"`
	HasAgent      bool `json:"has_agent"`
	HasMonitoring bool `json:"has_monitoring"`

	Status    string `json:"status"` // 'active', 'paused', 'archived'
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`

	// Enriched fields for API responses
	DiskUsage  *SiteDiskUsage         `json:"disk_usage,omitempty"`
	WpFlags    *SiteWpFlags           `json:"wp_flags,omitempty"`
	LastUpdate *SiteUpdateLedgerEntry `json:"last_update,omitempty"`
	WPCore     *SiteCore              `json:"wp_core,omitempty"`
}

// ToCliSite converts a ManagedSite to a CliSite for wp-cli execution.
func (s ManagedSite) ToCliSite() CliSite {
	sshSpec := s.SSHHost
	if s.SSHUser != "" {
		sshSpec = s.SSHUser + "@" + s.SSHHost
	}
	if s.SSHPort > 0 && s.SSHPort != 22 {
		sshSpec = fmt.Sprintf("%s:%d", sshSpec, s.SSHPort)
	}

	return CliSite{
		UUID:       s.ID,
		Name:       s.Domain,
		ServerName: s.ServerName,
		SSH:        sshSpec,
		Path:       s.SitePath,
	}
}
