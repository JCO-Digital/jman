package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// resolveSiteUUID parses an ID path parameter that may be either a 36-character UUID
// or a legacy SpinupWP integer ID, and returns the canonical UUID string.
func resolveSiteUUID(rawID string) (string, error) {
	rawID = strings.TrimSpace(rawID)
	if utils.IsValidUUID(rawID) {
		return rawID, nil
	}
	if num, err := strconv.Atoi(rawID); err == nil && num > 0 {
		return utils.SpinupWPSiteUUID(num), nil
	}
	return "", fmt.Errorf("invalid site ID: %s", rawID)
}

// CreateSiteRequest defines the JSON payload for creating an external/manual site.
type CreateSiteRequest struct {
	Domain         string                     `json:"domain"`
	ServerID       *string                    `json:"server_id"`
	Provider       string                     `json:"provider"`
	Environment    models.SiteEnvironmentType `json:"environment"`
	IsWordpress    *bool                      `json:"is_wordpress"`
	PHPVersion     string                     `json:"php_version"`
	ConnectionType string                     `json:"connection_type"`
	SSHHost        string                     `json:"ssh_host"`
	SSHPort        int                        `json:"ssh_port"`
	SSHUser        string                     `json:"ssh_user"`
	SitePath       string                     `json:"site_path"`
	CanWPCLI       *bool                      `json:"can_wp_cli"`
	HasAgent       *bool                      `json:"has_agent"`
	HasMonitoring  *bool                      `json:"has_monitoring"`
	Status         string                     `json:"status"`
}

// CreateSiteHandler creates a new external or manual site in inventory.db.
func CreateSiteHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateSiteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.Domain = strings.TrimSpace(req.Domain)
	if req.Domain == "" {
		WriteError(w, http.StatusBadRequest, "Domain is required")
		return
	}

	// Check if site with this domain already exists
	existing, err := db.GetManagedSiteByDomain(req.Domain)
	if err != nil {
		verb.LogPrintf(verb.Normal, "CreateSiteHandler lookup error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if existing != nil {
		WriteError(w, http.StatusConflict, fmt.Sprintf("Site with domain %q already exists", req.Domain))
		return
	}

	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = "manual"
	}

	connType := strings.TrimSpace(req.ConnectionType)
	if connType == "" {
		connType = "ssh"
	}

	sshPort := req.SSHPort
	if sshPort <= 0 {
		sshPort = 22
	}

	sitePath := strings.TrimSpace(req.SitePath)
	if sitePath == "" {
		sitePath = "files"
	}

	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "active"
	}

	env := req.Environment
	if env == "" {
		env = models.SiteEnvironmentProduction
	}

	isWordpress := true
	if req.IsWordpress != nil {
		isWordpress = *req.IsWordpress
	}

	canWPCLI := true
	if req.CanWPCLI != nil {
		canWPCLI = *req.CanWPCLI
	}

	hasAgent := false
	if req.HasAgent != nil {
		hasAgent = *req.HasAgent
	}

	hasMonitoring := true
	if req.HasMonitoring != nil {
		hasMonitoring = *req.HasMonitoring
	}

	site := models.ManagedSite{
		ID:             utils.NewV7UUID(),
		ServerID:       req.ServerID,
		Provider:       provider,
		Domain:         req.Domain,
		Environment:    env,
		IsWordpress:    isWordpress,
		PHPVersion:     strings.TrimSpace(req.PHPVersion),
		ConnectionType: connType,
		SSHHost:        strings.TrimSpace(req.SSHHost),
		SSHPort:        sshPort,
		SSHUser:        strings.TrimSpace(req.SSHUser),
		SitePath:       sitePath,
		CanWPCLI:       canWPCLI,
		HasAgent:       hasAgent,
		HasMonitoring:  hasMonitoring,
		Status:         status,
	}

	if err := db.SaveManagedSite(site); err != nil {
		verb.LogPrintf(verb.Normal, "CreateSiteHandler save error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to save site")
		return
	}

	// Fetch created site to return fully populated model
	created, err := db.GetManagedSite(site.ID)
	if err != nil || created == nil {
		WriteJSON(w, http.StatusCreated, site)
		return
	}

	WriteJSON(w, http.StatusCreated, created)
}

// GetSiteHandler returns a single site by UUID or legacy SpinupWP integer ID.
func GetSiteHandler(w http.ResponseWriter, r *http.Request) {
	rawID := r.PathValue("id")
	siteUUID, err := resolveSiteUUID(rawID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	site, err := db.GetManagedSite(siteUUID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "GetSiteHandler error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if site == nil {
		// Fallback check by provider_site_id if rawID was numeric
		if _, numErr := strconv.Atoi(rawID); numErr == nil {
			site, _ = db.GetManagedSiteByProviderID("spinupwp", rawID)
		}
		if site == nil {
			WriteError(w, http.StatusNotFound, "Site not found")
			return
		}
	}

	WriteJSON(w, http.StatusOK, site)
}

// UpdateSiteHandler updates an existing site's configuration.
func UpdateSiteHandler(w http.ResponseWriter, r *http.Request) {
	rawID := r.PathValue("id")
	siteUUID, err := resolveSiteUUID(rawID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	site, err := db.GetManagedSite(siteUUID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "UpdateSiteHandler lookup error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if site == nil {
		WriteError(w, http.StatusNotFound, "Site not found")
		return
	}

	var req CreateSiteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Domain != "" {
		site.Domain = strings.TrimSpace(req.Domain)
	}
	if req.ServerID != nil {
		site.ServerID = req.ServerID
	}
	if req.Environment != "" {
		site.Environment = req.Environment
	}
	if req.IsWordpress != nil {
		site.IsWordpress = *req.IsWordpress
	}
	if req.PHPVersion != "" {
		site.PHPVersion = strings.TrimSpace(req.PHPVersion)
	}
	if req.ConnectionType != "" {
		site.ConnectionType = strings.TrimSpace(req.ConnectionType)
	}
	if req.SSHHost != "" {
		site.SSHHost = strings.TrimSpace(req.SSHHost)
	}
	if req.SSHPort > 0 {
		site.SSHPort = req.SSHPort
	}
	if req.SSHUser != "" {
		site.SSHUser = strings.TrimSpace(req.SSHUser)
	}
	if req.SitePath != "" {
		site.SitePath = strings.TrimSpace(req.SitePath)
	}
	if req.CanWPCLI != nil {
		site.CanWPCLI = *req.CanWPCLI
	}
	if req.HasAgent != nil {
		site.HasAgent = *req.HasAgent
	}
	if req.HasMonitoring != nil {
		site.HasMonitoring = *req.HasMonitoring
	}
	if req.Status != "" {
		site.Status = strings.TrimSpace(req.Status)
	}

	if err := db.SaveManagedSite(*site); err != nil {
		verb.LogPrintf(verb.Normal, "UpdateSiteHandler save error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to update site")
		return
	}

	updated, err := db.GetManagedSite(site.ID)
	if err != nil || updated == nil {
		WriteJSON(w, http.StatusOK, site)
		return
	}

	WriteJSON(w, http.StatusOK, updated)
}

// DeleteSiteHandler deletes a site from inventory.db.
func DeleteSiteHandler(w http.ResponseWriter, r *http.Request) {
	rawID := r.PathValue("id")
	siteUUID, err := resolveSiteUUID(rawID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	site, err := db.GetManagedSite(siteUUID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "DeleteSiteHandler lookup error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if site == nil {
		WriteError(w, http.StatusNotFound, "Site not found")
		return
	}

	if err := db.DeleteManagedSite(site.ID); err != nil {
		verb.LogPrintf(verb.Normal, "DeleteSiteHandler error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to delete site")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Site %s deleted successfully", site.Domain),
	})
}

// TestSiteConnectionHandler tests remote SSH and WP-CLI execution for a site.
func TestSiteConnectionHandler(w http.ResponseWriter, r *http.Request) {
	rawID := r.PathValue("id")
	siteUUID, err := resolveSiteUUID(rawID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	site, err := db.GetManagedSite(siteUUID)
	if err != nil || site == nil {
		WriteError(w, http.StatusNotFound, "Site not found")
		return
	}

	if !site.CanWPCLI {
		WriteError(w, http.StatusBadRequest, "WP-CLI is not enabled for this site")
		return
	}

	if site.SSHHost == "" {
		WriteError(w, http.StatusBadRequest, "SSH host is not configured for this site")
		return
	}

	sshSpec := fmt.Sprintf("%s@%s", site.SSHUser, site.SSHHost)
	if site.SSHPort > 0 && site.SSHPort != 22 {
		sshSpec = fmt.Sprintf("%s@%s:%d", site.SSHUser, site.SSHHost, site.SSHPort)
	}

	opts := wpcli.CliOptions{
		SSH:     sshSpec,
		Path:    site.SitePath,
		Timeout: 15 * time.Second,
	}

	res, runErr := wpcli.RunWP(opts, "core", "version")
	if runErr != nil {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"error":   runErr.Error(),
			"output":  strings.TrimSpace(res.Output),
			"stderr":  strings.TrimSpace(res.Error),
		})
		return
	}

	version := strings.TrimSpace(res.Output)
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"wp_version": version,
		"message":    fmt.Sprintf("Successfully connected to WordPress %s", version),
	})
}

// CreateServerRequest defines the JSON payload for creating a managed server.
type CreateServerRequest struct {
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	IsLogical bool   `json:"is_logical"`
	IPAddress string `json:"ip_address"`
	SSHPort   int    `json:"ssh_port"`
}

// CreateServerHandler creates a physical or logical server in inventory.db.
func CreateServerHandler(w http.ResponseWriter, r *http.Request) {
	var req CreateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		WriteError(w, http.StatusBadRequest, "Server name is required")
		return
	}

	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = "manual"
	}

	sshPort := req.SSHPort
	if sshPort <= 0 {
		sshPort = 22
	}

	server := models.ManagedServer{
		ID:        utils.NewV7UUID(),
		Name:      name,
		Provider:  provider,
		IsLogical: req.IsLogical,
		IPAddress: strings.TrimSpace(req.IPAddress),
		SSHPort:   sshPort,
	}

	if err := db.SaveManagedServer(server); err != nil {
		verb.LogPrintf(verb.Normal, "CreateServerHandler error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to save server")
		return
	}

	WriteJSON(w, http.StatusCreated, server)
}

// DeleteServerHandler deletes a server from inventory.db.
func DeleteServerHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	server, err := db.GetManagedServer(id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "DeleteServerHandler error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if server == nil {
		WriteError(w, http.StatusNotFound, "Server not found")
		return
	}

	if err := db.DeleteManagedServer(id); err != nil {
		verb.LogPrintf(verb.Normal, "DeleteServerHandler delete error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to delete server")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("Server %s deleted successfully", server.Name),
	})
}

// SyncSpinupWPHandler manually triggers a sync of SpinupWP servers and sites.
func SyncSpinupWPHandler(w http.ResponseWriter, r *http.Request) {
	if config.Cfg.TokenSpinup == "" {
		WriteError(w, http.StatusBadRequest, "SpinupWP API token is not configured")
		return
	}

	servers, sites, err := cache.RefreshServersAndSites(0)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SyncSpinupWPHandler error: %v", err)
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Sync failed: %v", err))
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"servers": len(servers),
		"sites":   len(sites),
		"message": fmt.Sprintf("Successfully synced %d servers and %d sites from SpinupWP", len(servers), len(sites)),
	})
}
