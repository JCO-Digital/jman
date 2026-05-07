package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sort"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/vuln"
)

// PluginsHandler returns the list of cached WordPress plugins.
func PluginsHandler(w http.ResponseWriter, r *http.Request) {
	plugins, err := db.GetAllSitePlugins()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
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
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
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

// ServersHandler returns the list of cached servers.
func ServersHandler(w http.ResponseWriter, r *http.Request) {
	servers := []models.Server{}
	if err := cache.ReadJSONCache("servers", &servers, -1); err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Cache missing: %v", err))
		return
	}

	// Sort by ID for deterministic output.
	sort.Slice(servers, func(i, j int) bool {
		return servers[i].ID < servers[j].ID
	})

	WriteJSON(w, http.StatusOK, servers)
}

// SitesHandler returns the list of cached sites.
func SitesHandler(w http.ResponseWriter, r *http.Request) {
	sites := []models.Site{}
	if err := cache.ReadJSONCache("sites", &sites, -1); err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Cache missing: %v", err))
		return
	}

	// Sort by ID for deterministic output.
	sort.Slice(sites, func(i, j int) bool {
		return sites[i].ID < sites[j].ID
	})

	WriteJSON(w, http.StatusOK, sites)
}

// VulnsHandler returns the filtered and enriched vulnerability data for managed plugins.
// If a "plugin" query parameter is provided, it returns vulnerabilities for that plugin.
// Otherwise, it returns all active vulnerabilities across all managed sites.
func VulnsHandler(w http.ResponseWriter, r *http.Request) {
	pluginName := r.URL.Query().Get("plugin")
	if pluginName == "" {
		reports, err := vuln.ProcessVulnerabilities()
		if err != nil {
			WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to process vulnerabilities: %v", err))
			return
		}

		// Enrich each vulnerability with its affected sites for UI convenience.
		for i := range reports {
			reports[i].Vulnerability.Sites = reports[i].Sites
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
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get plugin data: %v", err))
		return
	}

	var targetSites []models.PluginSite
	for _, p := range pluginData {
		if p.Name == pluginName {
			targetSites = p.Sites
			break
		}
	}

	reports := vuln.GetVulnerabilityReportsForPlugin(pluginName, targetSites)

	// Prepare the response data based on the original structure but filtered.
	// We return a copy of VulnData with filtered and enriched vulnerabilities.
	response := *vulnResponse.Data
	response.Vulnerability = []models.Vulnerability{}

	for _, report := range reports {
		v := report.Vulnerability
		v.Sites = report.Sites
		response.Vulnerability = append(response.Vulnerability, v)
	}

	WriteJSON(w, http.StatusOK, response)
}
