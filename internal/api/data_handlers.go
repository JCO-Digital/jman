package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"sort"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

// PluginsHandler returns the list of cached WordPress plugins.
func PluginsHandler(w http.ResponseWriter, r *http.Request) {
	var plugins []models.WPPlugin
	if err := cache.ReadJSONCache("plugins", &plugins, cache.DefaultTTL); err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Cache missing or expired: %v. Run 'jman fetch plugins' to fetch data.", err))
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
	var servers []models.Server
	if err := cache.ReadJSONCache("servers", &servers, cache.DefaultTTL); err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Cache missing or expired: %v. Run 'jman fetch servers' to fetch data.", err))
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
	var sites []models.Site
	if err := cache.ReadJSONCache("sites", &sites, cache.DefaultTTL); err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Cache missing or expired: %v. Run 'jman fetch sites' to fetch data.", err))
		return
	}

	// Sort by ID for deterministic output.
	sort.Slice(sites, func(i, j int) bool {
		return sites[i].ID < sites[j].ID
	})

	WriteJSON(w, http.StatusOK, sites)
}

// VulnsHandler returns the cached vulnerability data for a specific plugin.
func VulnsHandler(w http.ResponseWriter, r *http.Request) {
	plugin := r.URL.Query().Get("plugin")
	if plugin == "" {
		WriteError(w, http.StatusBadRequest, "Missing required query parameter: plugin")
		return
	}

	// Sanitize the plugin name to prevent path traversal.
	// We only want the base filename.
	plugin = filepath.Base(plugin)
	if plugin == "." || plugin == ".." || plugin == "/" {
		WriteError(w, http.StatusBadRequest, "Invalid plugin name")
		return
	}

	var vulnData models.VulnResponse
	filename := fmt.Sprintf("vulnerabilities/%s", plugin)
	if err := cache.ReadJSONCache(filename, &vulnData, cache.DefaultTTL); err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("Cache missing or expired for plugin %q: %v. Run 'jman vuln %s' to fetch data.", plugin, err, plugin))
		return
	}
	WriteJSON(w, http.StatusOK, vulnData)
}
