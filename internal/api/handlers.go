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

	// --- Company & Contact routes ---
	mux.Handle("GET /api/companies", auth(ListCompaniesHandler))
	mux.Handle("POST /api/companies", auth(CreateCompanyHandler))
	mux.Handle("GET /api/companies/{id}", auth(GetCompanyHandler))
	mux.Handle("PATCH /api/companies/{id}", auth(UpdateCompanyHandler))
	mux.Handle("DELETE /api/companies/{id}", auth(DeleteCompanyHandler))
	mux.Handle("GET /api/companies/{id}/contacts", auth(ListContactsHandler))
	mux.Handle("GET /api/companies/{id}/sites", auth(ListCompanySitesHandler))
	mux.Handle("POST /api/contacts", auth(CreateContactHandler))
	mux.Handle("PATCH /api/contacts/{id}", auth(UpdateContactHandler))
	mux.Handle("DELETE /api/contacts/{id}", auth(DeleteContactHandler))

	// --- Site linking routes ---
	mux.Handle("GET /api/sites/{id}/company", auth(GetSiteCompanyHandler))
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
