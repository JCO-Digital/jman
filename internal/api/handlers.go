package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verbosity"
)

// RegisterHandlers registers all API routes to the provided mux.
func RegisterHandlers(mux *http.ServeMux, version string) {
	mux.HandleFunc("GET /api/health", HealthHandler(version))
	mux.HandleFunc("GET /api/plugins", PluginsHandler)
	mux.HandleFunc("GET /api/servers", ServersHandler)
	mux.HandleFunc("GET /api/sites", SitesHandler)
	mux.HandleFunc("GET /api/vulns", VulnsHandler)
}

// HealthHandler returns a simple health check response.
func HealthHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","version":%q}`, version)
	}
}

// PluginsHandler returns the list of cached WordPress plugins.
func PluginsHandler(w http.ResponseWriter, r *http.Request) {
	var plugins []models.WPPlugin
	if err := cache.ReadJSONCache("plugins", &plugins); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Cache missing or expired: %v. Run 'jman fetch plugins' to fetch data."}`, err), http.StatusNotFound)
		return
	}
	WriteJSON(w, plugins)
}

// ServersHandler returns the list of cached servers.
func ServersHandler(w http.ResponseWriter, r *http.Request) {
	var servers []models.Server
	if err := cache.ReadJSONCache("servers", &servers); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Cache missing or expired: %v. Run 'jman fetch servers' to fetch data."}`, err), http.StatusNotFound)
		return
	}
	WriteJSON(w, servers)
}

// SitesHandler returns the list of cached sites.
func SitesHandler(w http.ResponseWriter, r *http.Request) {
	var sites []models.Site
	if err := cache.ReadJSONCache("sites", &sites); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Cache missing or expired: %v. Run 'jman fetch sites' to fetch data."}`, err), http.StatusNotFound)
		return
	}
	WriteJSON(w, sites)
}

// VulnsHandler returns the cached vulnerability data for a specific plugin.
func VulnsHandler(w http.ResponseWriter, r *http.Request) {
	plugin := r.URL.Query().Get("plugin")
	if plugin == "" {
		http.Error(w, `{"error":"Missing required query parameter: plugin"}`, http.StatusBadRequest)
		return
	}

	var vulnData models.VulnResponse
	filename := fmt.Sprintf("vulnerabilities/%s", plugin)
	if err := cache.ReadJSONCache(filename, &vulnData); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"Cache missing or expired for plugin %q: %v. Run 'jman vuln %s' to fetch data."}`, plugin, err, plugin), http.StatusNotFound)
		return
	}
	WriteJSON(w, vulnData)
}

// WriteJSON is a helper to encode data as JSON to the response writer.
func WriteJSON(w http.ResponseWriter, data any) {
	if err := json.NewEncoder(w).Encode(data); err != nil {
		verbosity.LogPrintf(verbosity.Normal, "Error encoding JSON: %v", err)
	}
}
