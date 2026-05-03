package api

import (
	"net/http"

	"github.com/JCO-Digital/jman/internal/config"
)

// RegisterHandlers registers all API routes to the provided mux.
// Authentication is mandatory: all data endpoints are protected by JWT middleware,
// while health and login endpoints remain public.
func RegisterHandlers(mux *http.ServeMux, version string, usersCfg config.UsersConfig) {
	limiter := NewLoginRateLimiter()

	// --- Public routes (no auth required) ---
	mux.HandleFunc("GET /api/health", HealthHandler(version))
	mux.HandleFunc("POST /api/auth/login", LoginHandler(&usersCfg, limiter))

	// --- Protected routes (JWT required) ---
	auth := func(h http.HandlerFunc) http.Handler {
		return AuthMiddleware(&usersCfg, h)
	}

	mux.Handle("POST /api/auth/refresh", auth(RefreshHandler(&usersCfg)))
	mux.Handle("GET /api/plugins", auth(PluginsHandler))
	mux.Handle("GET /api/plugininfo", auth(PluginInfoHandler))
	mux.Handle("GET /api/servers", auth(ServersHandler))
	mux.Handle("GET /api/sites", auth(SitesHandler))
	mux.Handle("GET /api/vulns", auth(VulnsHandler))

	// --- Monitoring routes ---
	mux.Handle("GET /api/monitor/history", auth(MonitorHistoryHandler))
	mux.Handle("GET /api/monitor/status", auth(MonitorStatusHandler))
	mux.Handle("GET /api/monitor/ignored", auth(IgnoredSitesHandler))
	mux.Handle("POST /api/monitor/ignored", auth(IgnoredSitesHandler))
	mux.Handle("DELETE /api/monitor/ignored/{domain}", auth(UnignoreSiteHandler))

	// --- Organization & Contact routes ---
	mux.Handle("GET /api/organizations", auth(ListOrganizationsHandler))
	mux.Handle("POST /api/organizations", auth(CreateOrganizationHandler))
	mux.Handle("GET /api/organizations/{id}", auth(GetOrganizationHandler))
	mux.Handle("PATCH /api/organizations/{id}", auth(UpdateOrganizationHandler))
	mux.Handle("DELETE /api/organizations/{id}", auth(DeleteOrganizationHandler))
	mux.Handle("GET /api/organizations/{id}/contacts", auth(ListContactsHandler))
	mux.Handle("GET /api/organizations/{id}/sites", auth(ListOrganizationSitesHandler))
	mux.Handle("POST /api/contacts", auth(CreateContactHandler))
	mux.Handle("PATCH /api/contacts/{id}", auth(UpdateContactHandler))
	mux.Handle("DELETE /api/contacts/{id}", auth(DeleteContactHandler))

	// --- Site linking routes ---
	mux.Handle("GET /api/sites/{id}/organization", auth(GetSiteOrganizationHandler))
	mux.Handle("POST /api/sites/{id}/link", auth(LinkSiteHandler))
	mux.Handle("DELETE /api/sites/{id}/link", auth(UnlinkSiteHandler))

	// --- Note routes ---
	mux.Handle("GET /api/notes", auth(ListNotesHandler))
	mux.Handle("POST /api/notes", auth(CreateNoteHandler))
	mux.Handle("PATCH /api/notes/{id}", auth(UpdateNoteHandler))
	mux.Handle("DELETE /api/notes/{id}", auth(DeleteNoteHandler))
}

// HealthHandler returns a simple health check response.
func HealthHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": version,
		})
	}
}
