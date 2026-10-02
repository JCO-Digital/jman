package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/updatejobs"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/verb"
	"github.com/JCO-Digital/jman/internal/vuln"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// pluginSlugRegex matches valid WordPress plugin slugs. The first character must be
// alphanumeric, which rejects flag-like values such as --all or -p outright.
var pluginSlugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_\-/]*$`)

// PluginsHandler returns the list of cached WordPress plugins.
func PluginsHandler(w http.ResponseWriter, r *http.Request) {
	plugins, err := db.GetAllSitePlugins()
	if err != nil {
		verb.LogPrintf(verb.Normal, "PluginsHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Sort by site ID then by name for deterministic output.
	sort.Slice(plugins, func(i, j int) bool {
		if plugins[i].SiteID != plugins[j].SiteID {
			return plugins[i].SiteID < plugins[j].SiteID
		}
		return plugins[i].Name < plugins[j].Name
	})

	WriteJSON(w, http.StatusOK, plugins)
}

// PluginInfoHandler returns the list of cached WordPress plugin information.
func PluginInfoHandler(w http.ResponseWriter, r *http.Request) {
	plugins, err := db.GetAllPluginInfo()
	if err != nil {
		verb.LogPrintf(verb.Normal, "PluginInfoHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if len(plugins) == 0 {
		WriteError(w, http.StatusNotFound, "No plugin info found in database. Run 'jman fetch info' to fetch data.")
		return
	}

	// Sort by slug for deterministic output.
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Slug < plugins[j].Slug
	})

	WriteJSON(w, http.StatusOK, plugins)
}

// apiServer is the /api/servers representation of a server: the inventory
// record, with the SpinupWP cache details embedded for SpinupWP servers. The
// embedded pointer is nil for other servers, so those fields are omitted.
type apiServer struct {
	*models.Server
	ID               string `json:"id"`
	Provider         string `json:"provider"`
	ProviderServerID string `json:"provider_server_id,omitempty"`
	Name             string `json:"name"`
	IsLogical        bool   `json:"is_logical"`
	IPAddress        string `json:"ip_address"`
	SSHPort          int    `json:"ssh_port"`
}

// ServersHandler returns every server in the inventory, enriched with the
// SpinupWP cache details for SpinupWP servers. ?format=managed is accepted as
// an alias for the same list.
func ServersHandler(w http.ResponseWriter, r *http.Request) {
	managedServers, err := db.ListManagedServers()
	if err != nil {
		verb.LogPrintf(verb.Normal, "ServersHandler managed servers error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if r.URL.Query().Get("format") == "managed" {
		if managedServers == nil {
			managedServers = []models.ManagedServer{}
		}
		WriteJSON(w, http.StatusOK, managedServers)
		return
	}

	cached := []models.Server{}
	_ = cache.ReadJSONCache("servers", &cached, -1)
	cachedByUUID := make(map[string]*models.Server, len(cached))
	for i := range cached {
		cachedByUUID[utils.SpinupWPServerUUID(cached[i].ID)] = &cached[i]
	}

	servers := make([]apiServer, 0, len(managedServers)+len(cached))
	known := make(map[string]bool, len(managedServers))
	for _, ms := range managedServers {
		known[ms.ID] = true
		srv := apiServer{
			ID:               ms.ID,
			Provider:         ms.Provider,
			ProviderServerID: ms.ProviderServerID,
			Name:             ms.Name,
			IsLogical:        ms.IsLogical,
			IPAddress:        ms.IPAddress,
			SSHPort:          ms.SSHPort,
		}
		if ms.Provider == "spinupwp" {
			srv.Server = cachedByUUID[ms.ID]
		}
		servers = append(servers, srv)
	}
	// SpinupWP servers not yet synced into the inventory.
	for uuid, c := range cachedByUUID {
		if known[uuid] {
			continue
		}
		servers = append(servers, apiServer{
			Server:           c,
			ID:               uuid,
			Provider:         "spinupwp",
			ProviderServerID: strconv.Itoa(c.ID),
			Name:             c.Name,
			IPAddress:        c.IPAddress,
			SSHPort:          c.SSHPort,
		})
	}

	sort.Slice(servers, func(i, j int) bool {
		if servers[i].Name != servers[j].Name {
			return servers[i].Name < servers[j].Name
		}
		return servers[i].ID < servers[j].ID
	})

	WriteJSON(w, http.StatusOK, servers)
}

// apiSite is the /api/sites representation of a site: the inventory record
// (identity, connection and capabilities), with the SpinupWP cache details
// embedded for SpinupWP sites. The embedded pointer is nil for other sites,
// so SpinupWP-only fields are omitted for them.
type apiSite struct {
	*models.Site
	ID             string                     `json:"id"`
	Provider       string                     `json:"provider"`
	ProviderSiteID string                     `json:"provider_site_id,omitempty"`
	ServerID       *string                    `json:"server_id"`
	ServerName     string                     `json:"server_name"`
	Domain         string                     `json:"domain"`
	Environment    models.SiteEnvironmentType `json:"environment,omitempty"`
	Status         string                     `json:"status"`
	IsWordpress    bool                       `json:"is_wordpress"`
	PHPVersion     string                     `json:"php_version"`
	SiteUser       string                     `json:"site_user"`
	ConnectionType string                     `json:"connection_type"`
	SSHHost        string                     `json:"ssh_host"`
	SSHPort        int                        `json:"ssh_port"`
	// SSH is the target jman actually connects to for WP-CLI, in WP-CLI's
	// --ssh form ("user@host", or "user@host:port" for non-default ports).
	SSH           string `json:"ssh,omitempty"`
	SitePath      string `json:"site_path"`
	CanWPCLI      bool   `json:"can_wp_cli"`
	HasAgent      bool   `json:"has_agent"`
	HasMonitoring bool   `json:"has_monitoring"`

	DiskUsage  *models.SiteDiskUsage         `json:"disk_usage,omitempty"`
	WpFlags    *models.SiteWpFlags           `json:"wp_flags,omitempty"`
	LastUpdate *models.SiteUpdateLedgerEntry `json:"last_update,omitempty"`
	WPCore     *models.SiteCore              `json:"wp_core,omitempty"`
}

func newAPISite(ms models.ManagedSite) apiSite {
	site := apiSite{
		ID:             ms.ID,
		Provider:       ms.Provider,
		ProviderSiteID: ms.ProviderSiteID,
		ServerID:       ms.ServerID,
		ServerName:     ms.ServerName,
		Domain:         ms.Domain,
		Environment:    ms.Environment,
		Status:         ms.Status,
		IsWordpress:    ms.IsWordpress,
		PHPVersion:     ms.PHPVersion,
		SiteUser:       ms.SSHUser,
		ConnectionType: ms.ConnectionType,
		SSHHost:        ms.SSHHost,
		SSHPort:        ms.SSHPort,
		SitePath:       ms.SitePath,
		CanWPCLI:       ms.CanWPCLI,
		HasAgent:       ms.HasAgent,
		HasMonitoring:  ms.HasMonitoring,
	}
	if ms.SSHHost != "" {
		site.SSH = ms.ToCliSite().SSH
	}
	return site
}

// loadAPISites builds the /api/sites list: every inventory site (enriched
// with SpinupWP cache details for SpinupWP sites), plus SpinupWP sites not
// yet synced into the inventory, with environment, disk usage, WP flags,
// latest update and core version attached.
func loadAPISites() ([]apiSite, error) {
	managedSites, err := db.ListManagedSites()
	if err != nil {
		return nil, fmt.Errorf("managed sites: %w", err)
	}

	cachedSites := []models.Site{}
	_ = cache.ReadJSONCache("sites", &cachedSites, -1)
	cachedByUUID := make(map[string]*models.Site, len(cachedSites))
	for i := range cachedSites {
		cachedByUUID[utils.SpinupWPSiteUUID(cachedSites[i].ID)] = &cachedSites[i]
	}

	sites := make([]apiSite, 0, len(managedSites)+len(cachedSites))
	known := make(map[string]bool, len(managedSites))
	for _, ms := range managedSites {
		known[ms.ID] = true
		site := newAPISite(ms)
		if ms.Provider == "spinupwp" {
			site.Site = cachedByUUID[ms.ID]
		}
		sites = append(sites, site)
	}
	if len(cachedByUUID) > len(known) {
		serverNames, _ := cache.GetFastServerMap()
		for uuid, c := range cachedByUUID {
			if known[uuid] {
				continue
			}
			serverID := utils.SpinupWPServerUUID(c.ServerID)
			// Not synced yet: derive the target the way the SpinupWP sync
			// will (site user at the server's hostname).
			ssh := ""
			if host := serverNames[c.ServerID]; host != "" && c.SiteUser != "" {
				ssh = c.SiteUser + "@" + host
			}
			sites = append(sites, apiSite{
				Site:           c,
				ID:             uuid,
				Provider:       "spinupwp",
				ProviderSiteID: strconv.Itoa(c.ID),
				ServerID:       &serverID,
				ServerName:     serverNames[c.ServerID],
				Domain:         c.Domain,
				Status:         c.Status,
				IsWordpress:    c.IsWordpress,
				PHPVersion:     c.PHPVersion,
				SiteUser:       c.SiteUser,
				ConnectionType: "agent",
				SSHHost:        serverNames[c.ServerID],
				SSH:            ssh,
				SitePath:       "files",
				CanWPCLI:       true,
				HasAgent:       true,
				HasMonitoring:  true,
			})
		}
	}

	environments, err := db.GetAllSiteEnvironments()
	if err != nil {
		return nil, fmt.Errorf("environments: %w", err)
	}
	diskUsage, err := db.GetLatestSiteDiskUsage()
	if err != nil {
		return nil, fmt.Errorf("disk usage: %w", err)
	}
	wpFlags, err := db.GetAllSiteWpFlags()
	if err != nil {
		return nil, fmt.Errorf("wp flags: %w", err)
	}
	latestUpdates, err := db.GetLatestSiteUpdateLedgerEntries()
	if err != nil {
		return nil, fmt.Errorf("update ledger: %w", err)
	}
	coreVersions, err := db.GetAllSiteCore()
	if err != nil {
		return nil, fmt.Errorf("core versions: %w", err)
	}
	coreBySiteID := make(map[string]models.SiteCore, len(coreVersions))
	for _, c := range coreVersions {
		coreBySiteID[c.SiteID] = c
	}

	for i := range sites {
		id := sites[i].ID
		// An explicit classification (PATCH /environment, auto-classifier)
		// takes precedence over the inventory default.
		if env, ok := environments[id]; ok {
			sites[i].Environment = models.SiteEnvironmentType(env)
		}
		if usage, ok := diskUsage[id]; ok {
			sites[i].DiskUsage = &usage
		}
		if flags, ok := wpFlags[id]; ok {
			sites[i].WpFlags = &flags
		}
		if lastUp, ok := latestUpdates[id]; ok {
			sites[i].LastUpdate = &lastUp
		}
		if core, ok := coreBySiteID[id]; ok {
			sites[i].WPCore = &core
		}
	}

	sort.Slice(sites, func(i, j int) bool {
		if sites[i].Domain != sites[j].Domain {
			return sites[i].Domain < sites[j].Domain
		}
		return sites[i].ID < sites[j].ID
	})

	return sites, nil
}

// SitesHandler returns every site (see loadAPISites). If ?format=managed is
// provided, it returns the raw host-agnostic ManagedSite records instead,
// optionally filtered by ?provider=.
func SitesHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("format") == "managed" {
		providerFilter := r.URL.Query().Get("provider")
		var managedSites []models.ManagedSite
		var err error
		if providerFilter != "" {
			managedSites, err = db.ListManagedSites(providerFilter)
		} else {
			managedSites, err = db.ListManagedSites()
		}
		if err != nil {
			verb.LogPrintf(verb.Normal, "SitesHandler managed sites error: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		if managedSites == nil {
			managedSites = []models.ManagedSite{}
		}
		WriteJSON(w, http.StatusOK, managedSites)
		return
	}

	sites, err := loadAPISites()
	if err != nil {
		verb.LogPrintf(verb.Normal, "SitesHandler error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	WriteJSON(w, http.StatusOK, sites)
}

// VulnsHandler returns the filtered and enriched vulnerability data for managed plugins.
// If a "plugin" query parameter is provided, it returns vulnerabilities for that plugin.
// Otherwise, it returns all active vulnerabilities across all managed sites.
func VulnsHandler(w http.ResponseWriter, r *http.Request) {
	matcher, err := db.NewVulnIgnoreMatcher()
	if err != nil {
		verb.LogPrintf(verb.Normal, "Warning: failed to load ignore entries: %v\n", err)
	}

	pluginName := r.URL.Query().Get("plugin")
	if pluginName == "" {
		reports, err := vuln.ProcessVulnerabilities(matcher)
		if err != nil {
			verb.LogPrintf(verb.Normal, "VulnsHandler process error: %v", err)
			WriteError(w, http.StatusInternalServerError, "Failed to process vulnerabilities")
			return
		}

		WriteJSON(w, http.StatusOK, reports)
		return
	}

	// Sanitize the plugin name to prevent path traversal.
	pluginName = filepath.Base(pluginName)
	if pluginName == "." || pluginName == ".." || pluginName == "/" {
		WriteError(w, http.StatusBadRequest, "Invalid plugin name")
		return
	}

	// Get vulnerability data first to get metadata.
	vulnResponse, err := cache.GetCachedVulnerabilities(pluginName)
	if err != nil || vulnResponse == nil || vulnResponse.Data == nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Vulnerability data not found for plugin %q.", pluginName))
		return
	}

	pluginData, err := cache.GetCachedPluginData()
	if err != nil {
		verb.LogPrintf(verb.Normal, "VulnsHandler plugin data error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to get plugin data")
		return
	}

	var targetSites []models.PluginSite
	for _, p := range pluginData {
		if p.Name == pluginName {
			targetSites = p.Sites
			break
		}
	}

	report := vuln.GetVulnerabilityReportsForPlugin(pluginName, targetSites, matcher)
	if report == nil {
		// Return the basic cached data but with zero vulnerabilities if none passed filtering.
		response := *vulnResponse.Data
		response.Suppressed = matcher != nil && matcher.IsPluginIgnored(pluginName)
		response.Vulnerability = []models.Vulnerability{}
		WriteJSON(w, http.StatusOK, response)
		return
	}

	// Return a GroupedVulnReport style response for consistency.
	WriteJSON(w, http.StatusOK, report)
}

// CoreVulnsHandler returns the vulnerability data for the WordPress core versions
// installed across the managed sites, grouped by core version.
func CoreVulnsHandler(w http.ResponseWriter, r *http.Request) {
	matcher, err := db.NewVulnIgnoreMatcher()
	if err != nil {
		verb.LogPrintf(verb.Normal, "Warning: failed to load ignore entries: %v\n", err)
	}

	reports, err := vuln.ProcessCoreVulnerabilities(matcher)
	if err != nil {
		verb.LogPrintf(verb.Normal, "CoreVulnsHandler process error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to process core vulnerabilities")
		return
	}

	WriteJSON(w, http.StatusOK, reports)
}

// SitePluginUpdatesHandler returns the list of plugins with available updates for a site.
// It calls WP-CLI live so the result reflects the current state of the site.
func SitePluginUpdatesHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	site, err := getCliSite(siteID)
	if err != nil {
		WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	plugins, err := wpcli.GetPlugins(*site, false)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SitePluginUpdatesHandler error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to get plugins from site")
		return
	}

	// Persist the freshly-fetched plugin state for this site.
	// Delete first to remove plugins that are no longer installed; if that
	// fails, log and continue — SaveSitePlugin uses ON CONFLICT UPDATE so
	// existing rows are still refreshed and updated_at is kept current.
	if err := db.DeleteSitePlugins(site.ID); err != nil {
		verb.PrintErrorf(verb.Normal, "Warning: failed to clear plugin cache for site %s: %v\n", site.Name, err)
	}
	for _, p := range plugins {
		if err := db.SaveSitePlugin(p); err != nil {
			verb.PrintErrorf(verb.Normal, "Warning: failed to save plugin %s for site %s: %v\n", p.Name, site.Name, err)
		}
		if p.Status != "must-use" && p.Status != "dropin" {
			bestVer := p.Version
			if p.Update != "" {
				bestVer = p.Update
			}
			cache.UpdatePluginInfo(p.Name, "", bestVer)
		}
	}

	updates := []models.WPPlugin{}
	for _, p := range plugins {
		if p.Update != "" {
			updates = append(updates, p)
		}
	}

	WriteJSON(w, http.StatusOK, updates)
}

// SitePluginUpdateHandler updates a single plugin on a site and refreshes the plugin cache.
func SitePluginUpdateHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Plugin     string `json:"plugin"`
		SkipLedger bool   `json:"skip_ledger"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if body.Plugin == "" {
		WriteError(w, http.StatusBadRequest, "Plugin name is required")
		return
	}
	if !pluginSlugRegex.MatchString(body.Plugin) {
		WriteError(w, http.StatusBadRequest, "Invalid plugin slug: must start with a letter or digit and contain only [a-z0-9_-/]")
		return
	}

	site, err := getCliSite(siteID)
	if err != nil {
		WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	// Try to get current version from cache for fallback
	currentVersion := ""
	if plugins, err := db.GetSitePlugins(site.ID); err == nil {
		for _, p := range plugins {
			if p.Name == body.Plugin {
				currentVersion = p.Version
				break
			}
		}
	}

	results, err := wpcli.UpdatePlugin(*site, []string{body.Plugin})

	// A slow host can time out after the update itself went through; if
	// the follow-up check shows the new version installed and the site out
	// of maintenance mode, report it as the update it was.
	installedVersion := currentVersion
	var failure *wpcli.UpdateFailure
	if errors.As(err, &failure) {
		if v, ok := failure.Versions[body.Plugin]; ok {
			installedVersion = v
			if currentVersion != "" && v != currentVersion && !failure.MaintenanceMode {
				verb.LogPrintf(verb.Normal, "Plugin %s on %s reported %v but is now at %s; treating as updated", body.Plugin, site.Name, failure.Err, v)
				results = []wpcli.UpdateResult{{Name: body.Plugin, OldVersion: currentVersion, NewVersion: v, Status: "Updated"}}
				err = nil
			}
		}
	}

	var response wpcli.UpdateResult
	response.Name = body.Plugin

	if err != nil {
		response.Status = "failed"
		response.OldVersion = currentVersion
		response.NewVersion = installedVersion
		response.Error = err.Error()

		// Save to site update ledger
		ledgerData := map[string]interface{}{
			"plugin":      body.Plugin,
			"old_version": currentVersion,
			"new_version": installedVersion,
			"error":       err.Error(),
		}
		ledgerJSON, _ := json.Marshal(ledgerData)
		username := "system"
		claims := GetAuthClaims(r.Context())
		if claims != nil {
			username = claims.Username
		}
		if !body.SkipLedger {
			_ = db.SaveSiteUpdateLedgerEntry(&models.SiteUpdateLedgerEntry{
				SiteID:     siteID,
				UpdateType: "plugin",
				Status:     "failed",
				DataJSON:   string(ledgerJSON),
				UpdatedBy:  username,
			})
		}

		WriteJSON(w, http.StatusInternalServerError, response)
		return
	}

	if len(results) == 0 {
		response.Status = "Up to date"
		// If we think it's up to date but versions don't match, or if we didn't have
		// a version, we should try to get the real current version.
		actual, err := wpcli.GetPlugins(*site, true)
		if err == nil {
			for _, p := range actual {
				if p.Name == body.Plugin {
					currentVersion = p.Version
					break
				}
			}
		}
		response.OldVersion = currentVersion
		response.NewVersion = currentVersion

		// Save to site update ledger
		ledgerData := map[string]interface{}{
			"plugin":      body.Plugin,
			"old_version": currentVersion,
			"new_version": currentVersion,
		}
		ledgerJSON, _ := json.Marshal(ledgerData)
		username := "system"
		claims := GetAuthClaims(r.Context())
		if claims != nil {
			username = claims.Username
		}
		if !body.SkipLedger {
			_ = db.SaveSiteUpdateLedgerEntry(&models.SiteUpdateLedgerEntry{
				SiteID:     siteID,
				UpdateType: "plugin",
				Status:     "partial",
				DataJSON:   string(ledgerJSON),
				UpdatedBy:  username,
			})
		}

		WriteJSON(w, http.StatusOK, response)
		return
	}

	response = results[0]

	// Normalize status to "failed" if it's not "Updated" and ensure versions are same on failure.
	if response.Status != "Updated" {
		response.Status = "failed"
		if response.OldVersion == "" {
			response.OldVersion = currentVersion
		}
		response.NewVersion = response.OldVersion
		response.Error = "Plugin update failed"

		// Save to site update ledger
		ledgerData := map[string]interface{}{
			"plugin":      body.Plugin,
			"old_version": response.OldVersion,
			"new_version": response.NewVersion,
			"error":       response.Error,
		}
		ledgerJSON, _ := json.Marshal(ledgerData)
		username := "system"
		claims := GetAuthClaims(r.Context())
		if claims != nil {
			username = claims.Username
		}
		if !body.SkipLedger {
			_ = db.SaveSiteUpdateLedgerEntry(&models.SiteUpdateLedgerEntry{
				SiteID:     siteID,
				UpdateType: "plugin",
				Status:     "failed",
				DataJSON:   string(ledgerJSON),
				UpdatedBy:  username,
			})
		}

		WriteJSON(w, http.StatusInternalServerError, response)
		return
	}

	// Save successful update to site update ledger
	ledgerData := map[string]interface{}{
		"plugin":      body.Plugin,
		"old_version": response.OldVersion,
		"new_version": response.NewVersion,
	}
	ledgerJSON, _ := json.Marshal(ledgerData)
	username := "system"
	claims := GetAuthClaims(r.Context())
	if claims != nil {
		username = claims.Username
	}
	if !body.SkipLedger {
		_ = db.SaveSiteUpdateLedgerEntry(&models.SiteUpdateLedgerEntry{
			SiteID:     siteID,
			UpdateType: "plugin",
			Status:     "partial",
			DataJSON:   string(ledgerJSON),
			UpdatedBy:  username,
		})
	}

	// Refresh the full plugin list for this site so all versions and
	// update_available flags reflect the post-update state.
	go func() {
		if err := cache.UpdateSitePluginCache(*site); err != nil {
			verb.PrintErrorf(verb.Normal, "Failed to refresh plugin cache for site %s after update: %v\n", site.Name, err)
		}
	}()

	WriteJSON(w, http.StatusOK, response)
}

// SiteCoreCheckHandler returns the installed WordPress core version and any
// available minor/major update for a site. It calls WP-CLI live so the
// result reflects the current state of the site.
func SiteCoreCheckHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	site, err := getCliSite(siteID)
	if err != nil {
		WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	core, err := cache.RefreshSiteCore(*site)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SiteCoreCheckHandler error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to check core version")
		return
	}

	WriteJSON(w, http.StatusOK, core)
}

// coreUpdateTargetRegex restricts the update target to the two supported values.
var coreUpdateTargetRegex = regexp.MustCompile(`^(minor|major)$`)

// coreJobMu serializes the duplicate check and enqueue of core update jobs,
// so two concurrent requests can't both queue one for the same site.
var coreJobMu sync.Mutex

// SiteCoreUpdateHandler queues a background update of WordPress core on a
// site to the latest minor or major version and returns the job (202). The
// job's result, including the refreshed core state, is available from
// GET /api/update-jobs/{id}.
func SiteCoreUpdateHandler(w http.ResponseWriter, r *http.Request) {
	siteID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if !coreUpdateTargetRegex.MatchString(body.Target) {
		WriteError(w, http.StatusBadRequest, `Target must be "minor" or "major"`)
		return
	}

	coreJobMu.Lock()
	defer coreJobMu.Unlock()
	active, err := db.HasActiveCoreUpdateJob(siteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SiteCoreUpdateHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if active {
		WriteError(w, http.StatusConflict, "A WordPress core update is already queued or running for this site")
		return
	}

	if _, err := getCliSite(siteID); err != nil {
		WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	job := models.UpdateJob{
		Kind:      models.UpdateJobKindCore,
		SiteID:    siteID,
		Target:    body.Target,
		CreatedBy: getUsername(r),
	}
	if err := updatejobs.Enqueue(&job); err != nil {
		verb.LogPrintf(verb.Normal, "SiteCoreUpdateHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to queue core update")
		return
	}
	WriteJSON(w, http.StatusAccepted, job)
}

// getCliSite returns the WP-CLI-reachable site with the given UUID.
func getCliSite(siteID string) (*models.CliSite, error) {
	sites, err := cache.GetFastSiteList()
	if err != nil {
		return nil, fmt.Errorf("failed to get site list: %w", err)
	}
	for _, s := range sites {
		if s.ID == siteID {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("site %s not found or not reachable via WP-CLI", siteID)
}

// SiteUpdateLedgerHandler returns the update history ledger for a site.
func SiteUpdateLedgerHandler(w http.ResponseWriter, r *http.Request) {
	siteUUID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	entries, err := db.GetSiteUpdateLedger(siteUUID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "SiteUpdateLedgerHandler error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	WriteJSON(w, http.StatusOK, entries)
}

// CreateSiteUpdateLedgerHandler manually adds an entry to the site update ledger.
func CreateSiteUpdateLedgerHandler(w http.ResponseWriter, r *http.Request) {
	siteUUID, err := resolveSiteUUID(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req struct {
		UpdateType string `json:"update_type"`
		Status     string `json:"status"`
		DataJSON   string `json:"data_json"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.UpdateType == "" || req.Status == "" {
		WriteError(w, http.StatusBadRequest, "update_type and status are required")
		return
	}

	username := "system"
	claims := GetAuthClaims(r.Context())
	if claims != nil {
		username = claims.Username
	}

	entry := models.SiteUpdateLedgerEntry{
		SiteID:     siteUUID,
		UpdateType: req.UpdateType,
		Status:     req.Status,
		DataJSON:   req.DataJSON,
		UpdatedBy:  username,
	}

	if err := db.SaveSiteUpdateLedgerEntry(&entry); err != nil {
		verb.LogPrintf(verb.Normal, "CreateSiteUpdateLedgerHandler error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to save entry")
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]string{"status": "success"})
}
