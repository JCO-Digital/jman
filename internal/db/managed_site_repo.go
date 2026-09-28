package db

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/JCO-Digital/jman/internal/models"
)

// SaveManagedServer inserts or updates a managed server record in inventory.db.
func SaveManagedServer(server models.ManagedServer) error {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `
	INSERT INTO servers (id, provider, provider_server_id, name, is_logical, ip_address, ssh_port, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(id) DO UPDATE SET
		provider = excluded.provider,
		provider_server_id = excluded.provider_server_id,
		name = excluded.name,
		is_logical = excluded.is_logical,
		ip_address = excluded.ip_address,
		ssh_port = excluded.ssh_port,
		updated_at = CURRENT_TIMESTAMP
	`

	_, err := dbConn.Exec(query,
		server.ID,
		server.Provider,
		server.ProviderServerID,
		server.Name,
		server.IsLogical,
		server.IPAddress,
		server.SSHPort,
	)
	if err != nil {
		return fmt.Errorf("failed to save managed server: %w", err)
	}

	return nil
}

// GetManagedServer retrieves a server by its UUID.
func GetManagedServer(id string) (*models.ManagedServer, error) {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT id, provider, provider_server_id, name, is_logical, ip_address, ssh_port, created_at, updated_at
	FROM servers
	WHERE id = ?
	`

	var s models.ManagedServer
	var ip sql.NullString
	var provServerID sql.NullString
	err := dbConn.QueryRow(query, id).Scan(
		&s.ID,
		&s.Provider,
		&provServerID,
		&s.Name,
		&s.IsLogical,
		&ip,
		&s.SSHPort,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get managed server: %w", err)
	}

	s.IPAddress = ip.String
	s.ProviderServerID = provServerID.String
	return &s, nil
}

// GetManagedServerByProviderID finds a server by provider and provider's server ID.
func GetManagedServerByProviderID(provider, providerServerID string) (*models.ManagedServer, error) {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT id, provider, provider_server_id, name, is_logical, ip_address, ssh_port, created_at, updated_at
	FROM servers
	WHERE provider = ? AND provider_server_id = ?
	`

	var s models.ManagedServer
	var ip sql.NullString
	var provServerID sql.NullString
	err := dbConn.QueryRow(query, provider, providerServerID).Scan(
		&s.ID,
		&s.Provider,
		&provServerID,
		&s.Name,
		&s.IsLogical,
		&ip,
		&s.SSHPort,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get managed server by provider ID: %w", err)
	}

	s.IPAddress = ip.String
	s.ProviderServerID = provServerID.String
	return &s, nil
}

// ListManagedServers retrieves all managed servers from inventory.db.
func ListManagedServers() ([]models.ManagedServer, error) {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT id, provider, provider_server_id, name, is_logical, ip_address, ssh_port, created_at, updated_at
	FROM servers
	ORDER BY name ASC
	`

	rows, err := dbConn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list managed servers: %w", err)
	}
	defer rows.Close()

	var servers []models.ManagedServer
	for rows.Next() {
		var s models.ManagedServer
		var ip sql.NullString
		var provServerID sql.NullString
		if err := rows.Scan(
			&s.ID,
			&s.Provider,
			&provServerID,
			&s.Name,
			&s.IsLogical,
			&ip,
			&s.SSHPort,
			&s.CreatedAt,
			&s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan managed server: %w", err)
		}
		s.IPAddress = ip.String
		s.ProviderServerID = provServerID.String
		servers = append(servers, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating managed servers: %w", err)
	}

	return servers, nil
}

// DeleteManagedServer removes a server by its UUID.
func DeleteManagedServer(id string) error {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `DELETE FROM servers WHERE id = ?`
	if _, err := dbConn.Exec(query, id); err != nil {
		return fmt.Errorf("failed to delete managed server: %w", err)
	}

	return nil
}

// CountManagedSitesForServer returns how many sites reference the given server UUID.
func CountManagedSitesForServer(serverID string) (int, error) {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	var count int
	if err := dbConn.QueryRow(`SELECT COUNT(*) FROM sites WHERE server_id = ?`, serverID).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count sites for server: %w", err)
	}

	return count, nil
}

// SaveManagedSite inserts or updates a managed site record in inventory.db.
func SaveManagedSite(site models.ManagedSite) error {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `
	INSERT INTO sites (
		id, server_id, provider, provider_site_id, domain, environment, is_wordpress, php_version,
		connection_type, ssh_host, ssh_port, ssh_user, site_path,
		can_wp_cli, has_agent, has_monitoring, status, updated_at
	)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(id) DO UPDATE SET
		server_id = excluded.server_id,
		provider = excluded.provider,
		provider_site_id = excluded.provider_site_id,
		domain = excluded.domain,
		environment = excluded.environment,
		is_wordpress = excluded.is_wordpress,
		php_version = excluded.php_version,
		connection_type = excluded.connection_type,
		ssh_host = excluded.ssh_host,
		ssh_port = excluded.ssh_port,
		ssh_user = excluded.ssh_user,
		site_path = excluded.site_path,
		can_wp_cli = excluded.can_wp_cli,
		has_agent = excluded.has_agent,
		has_monitoring = excluded.has_monitoring,
		status = excluded.status,
		updated_at = CURRENT_TIMESTAMP
	`

	_, err := dbConn.Exec(query,
		site.ID,
		site.ServerID,
		site.Provider,
		site.ProviderSiteID,
		site.Domain,
		string(site.Environment),
		site.IsWordpress,
		site.PHPVersion,
		site.ConnectionType,
		site.SSHHost,
		site.SSHPort,
		site.SSHUser,
		site.SitePath,
		site.CanWPCLI,
		site.HasAgent,
		site.HasMonitoring,
		site.Status,
	)
	if err != nil {
		return fmt.Errorf("failed to save managed site: %w", err)
	}

	return nil
}

// GetManagedSite retrieves a site by its UUID, including server name if joined.
func GetManagedSite(id string) (*models.ManagedSite, error) {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT s.id, s.server_id, s.provider, s.provider_site_id, s.domain, s.environment, s.is_wordpress, s.php_version,
	       s.connection_type, s.ssh_host, s.ssh_port, s.ssh_user, s.site_path,
	       s.can_wp_cli, s.has_agent, s.has_monitoring, s.status, s.created_at, s.updated_at,
	       COALESCE(srv.name, '')
	FROM sites s
	LEFT JOIN servers srv ON s.server_id = srv.id
	WHERE s.id = ?
	`

	var site models.ManagedSite
	var serverID sql.NullString
	var provSiteID sql.NullString
	var phpVer sql.NullString
	var serverName string

	err := dbConn.QueryRow(query, id).Scan(
		&site.ID,
		&serverID,
		&site.Provider,
		&provSiteID,
		&site.Domain,
		&site.Environment,
		&site.IsWordpress,
		&phpVer,
		&site.ConnectionType,
		&site.SSHHost,
		&site.SSHPort,
		&site.SSHUser,
		&site.SitePath,
		&site.CanWPCLI,
		&site.HasAgent,
		&site.HasMonitoring,
		&site.Status,
		&site.CreatedAt,
		&site.UpdatedAt,
		&serverName,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get managed site: %w", err)
	}

	if serverID.Valid && serverID.String != "" {
		sid := serverID.String
		site.ServerID = &sid
	}
	site.ProviderSiteID = provSiteID.String
	site.PHPVersion = phpVer.String
	site.ServerName = serverName

	return &site, nil
}

// GetManagedSiteByDomain retrieves a site by its domain (case-insensitive).
func GetManagedSiteByDomain(domain string) (*models.ManagedSite, error) {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT s.id, s.server_id, s.provider, s.provider_site_id, s.domain, s.environment, s.is_wordpress, s.php_version,
	       s.connection_type, s.ssh_host, s.ssh_port, s.ssh_user, s.site_path,
	       s.can_wp_cli, s.has_agent, s.has_monitoring, s.status, s.created_at, s.updated_at,
	       COALESCE(srv.name, '')
	FROM sites s
	LEFT JOIN servers srv ON s.server_id = srv.id
	WHERE LOWER(s.domain) = LOWER(?)
	`

	var site models.ManagedSite
	var serverID sql.NullString
	var provSiteID sql.NullString
	var phpVer sql.NullString
	var serverName string

	err := dbConn.QueryRow(query, domain).Scan(
		&site.ID,
		&serverID,
		&site.Provider,
		&provSiteID,
		&site.Domain,
		&site.Environment,
		&site.IsWordpress,
		&phpVer,
		&site.ConnectionType,
		&site.SSHHost,
		&site.SSHPort,
		&site.SSHUser,
		&site.SitePath,
		&site.CanWPCLI,
		&site.HasAgent,
		&site.HasMonitoring,
		&site.Status,
		&site.CreatedAt,
		&site.UpdatedAt,
		&serverName,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get managed site by domain: %w", err)
	}

	if serverID.Valid && serverID.String != "" {
		sid := serverID.String
		site.ServerID = &sid
	}
	site.ProviderSiteID = provSiteID.String
	site.PHPVersion = phpVer.String
	site.ServerName = serverName

	return &site, nil
}

// GetManagedSiteByProviderID retrieves a site by provider and provider's site ID.
func GetManagedSiteByProviderID(provider, providerSiteID string) (*models.ManagedSite, error) {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT s.id, s.server_id, s.provider, s.provider_site_id, s.domain, s.environment, s.is_wordpress, s.php_version,
	       s.connection_type, s.ssh_host, s.ssh_port, s.ssh_user, s.site_path,
	       s.can_wp_cli, s.has_agent, s.has_monitoring, s.status, s.created_at, s.updated_at,
	       COALESCE(srv.name, '')
	FROM sites s
	LEFT JOIN servers srv ON s.server_id = srv.id
	WHERE s.provider = ? AND s.provider_site_id = ?
	`

	var site models.ManagedSite
	var serverID sql.NullString
	var provSiteID sql.NullString
	var phpVer sql.NullString
	var serverName string

	err := dbConn.QueryRow(query, provider, providerSiteID).Scan(
		&site.ID,
		&serverID,
		&site.Provider,
		&provSiteID,
		&site.Domain,
		&site.Environment,
		&site.IsWordpress,
		&phpVer,
		&site.ConnectionType,
		&site.SSHHost,
		&site.SSHPort,
		&site.SSHUser,
		&site.SitePath,
		&site.CanWPCLI,
		&site.HasAgent,
		&site.HasMonitoring,
		&site.Status,
		&site.CreatedAt,
		&site.UpdatedAt,
		&serverName,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get managed site by provider ID: %w", err)
	}

	if serverID.Valid && serverID.String != "" {
		sid := serverID.String
		site.ServerID = &sid
	}
	site.ProviderSiteID = provSiteID.String
	site.PHPVersion = phpVer.String
	site.ServerName = serverName

	return &site, nil
}

// ListManagedSites retrieves all managed sites, optionally filtered by provider.
func ListManagedSites(providerFilter ...string) ([]models.ManagedSite, error) {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT s.id, s.server_id, s.provider, s.provider_site_id, s.domain, s.environment, s.is_wordpress, s.php_version,
	       s.connection_type, s.ssh_host, s.ssh_port, s.ssh_user, s.site_path,
	       s.can_wp_cli, s.has_agent, s.has_monitoring, s.status, s.created_at, s.updated_at,
	       COALESCE(srv.name, '')
	FROM sites s
	LEFT JOIN servers srv ON s.server_id = srv.id
	`
	var args []interface{}
	if len(providerFilter) > 0 && strings.TrimSpace(providerFilter[0]) != "" {
		query += " WHERE s.provider = ?"
		args = append(args, providerFilter[0])
	}
	query += " ORDER BY s.domain ASC"

	rows, err := dbConn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list managed sites: %w", err)
	}
	defer rows.Close()

	var sites []models.ManagedSite
	for rows.Next() {
		var site models.ManagedSite
		var serverID sql.NullString
		var provSiteID sql.NullString
		var phpVer sql.NullString
		var serverName string

		if err := rows.Scan(
			&site.ID,
			&serverID,
			&site.Provider,
			&provSiteID,
			&site.Domain,
			&site.Environment,
			&site.IsWordpress,
			&phpVer,
			&site.ConnectionType,
			&site.SSHHost,
			&site.SSHPort,
			&site.SSHUser,
			&site.SitePath,
			&site.CanWPCLI,
			&site.HasAgent,
			&site.HasMonitoring,
			&site.Status,
			&site.CreatedAt,
			&site.UpdatedAt,
			&serverName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan managed site: %w", err)
		}

		if serverID.Valid && serverID.String != "" {
			sid := serverID.String
			site.ServerID = &sid
		}
		site.ProviderSiteID = provSiteID.String
		site.PHPVersion = phpVer.String
		site.ServerName = serverName
		sites = append(sites, site)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating managed sites: %w", err)
	}

	return sites, nil
}

// DeleteManagedSite removes a site record by its UUID.
func DeleteManagedSite(id string) error {
	dbConn := GetInventoryDB()
	if dbConn == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `DELETE FROM sites WHERE id = ?`
	if _, err := dbConn.Exec(query, id); err != nil {
		return fmt.Errorf("failed to delete managed site: %w", err)
	}

	return nil
}
