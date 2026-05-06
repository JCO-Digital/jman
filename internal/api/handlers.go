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

	// --- Protected routes (JWT required with level check) ---
	basic := func(h http.HandlerFunc) http.Handler {
		return AuthMiddleware(&usersCfg, RequireLevel(config.LevelBasic)(h))
	}
	edit := func(h http.HandlerFunc) http.Handler {
		return AuthMiddleware(&usersCfg, RequireLevel(config.LevelEdit)(h))
	}
	execute := func(h http.HandlerFunc) http.Handler {
		return AuthMiddleware(&usersCfg, RequireLevel(config.LevelExecute)(h))
	}

	mux.Handle("POST /api/auth/refresh", basic(RefreshHandler(&usersCfg)))
	mux.Handle("GET /api/plugins", basic(PluginsHandler))
	mux.Handle("GET /api/plugininfo", basic(PluginInfoHandler))
	mux.Handle("GET /api/servers", basic(ServersHandler))
	mux.Handle("GET /api/sites", basic(SitesHandler))
	mux.Handle("GET /api/vulns", basic(VulnsHandler))

	// --- User Management (Admin) ---
	mux.Handle("GET /api/users", execute(AdminListUsersHandler(&usersCfg)))
	mux.Handle("POST /api/users", execute(CreateUserHandler(&usersCfg)))
	mux.Handle("PATCH /api/users/{username}", execute(UpdateUserHandler(&usersCfg)))
	mux.Handle("DELETE /api/users/{username}", execute(DeleteUserHandler(&usersCfg)))

	// --- User Self-Service ---
	mux.Handle("PATCH /api/user/profile", basic(UpdateProfileHandler(&usersCfg)))
	mux.Handle("POST /api/user/password", basic(ChangePasswordHandler(&usersCfg)))
	mux.Handle("POST /api/user/2fa/setup", basic(Setup2FAHandler))
	mux.Handle("POST /api/user/2fa/activate", basic(Activate2FAHandler(&usersCfg)))
	mux.Handle("POST /api/user/2fa/deactivate", basic(Deactivate2FAHandler(&usersCfg)))

	// --- Monitoring routes ---
	mux.Handle("GET /api/monitor/history", basic(MonitorHistoryHandler))
	mux.Handle("GET /api/monitor/status", basic(MonitorStatusHandler))
	mux.Handle("GET /api/monitor/ignored", basic(IgnoredSitesHandler))
	mux.Handle("POST /api/monitor/ignored", edit(IgnoredSitesHandler))
	mux.Handle("DELETE /api/monitor/ignored/{domain}", edit(UnignoreSiteHandler))

	// --- Organization & Contact routes ---
	mux.Handle("GET /api/organizations", basic(ListOrganizationsHandler))
	mux.Handle("POST /api/organizations", edit(CreateOrganizationHandler))
	mux.Handle("GET /api/organizations/{id}", basic(GetOrganizationHandler))
	mux.Handle("PATCH /api/organizations/{id}", edit(UpdateOrganizationHandler))
	mux.Handle("DELETE /api/organizations/{id}", edit(DeleteOrganizationHandler))
	mux.Handle("GET /api/organizations/{id}/contacts", basic(ListContactsHandler))
	mux.Handle("GET /api/organizations/{id}/sites", basic(ListOrganizationSitesHandler))
	mux.Handle("POST /api/contacts", edit(CreateContactHandler))
	mux.Handle("PATCH /api/contacts/{id}", edit(UpdateContactHandler))
	mux.Handle("DELETE /api/contacts/{id}", edit(DeleteContactHandler))

	// --- Asset routes ---
	mux.Handle("GET /api/assets", basic(ListAssetsHandler))
	mux.Handle("POST /api/assets", edit(CreateAssetHandler))
	mux.Handle("GET /api/assets/{id}", basic(GetAssetHandler))
	mux.Handle("PATCH /api/assets/{id}", edit(UpdateAssetHandler))
	mux.Handle("DELETE /api/assets/{id}", edit(DeleteAssetHandler))
	mux.Handle("GET /api/organization-assets", basic(ListAllOrganizationAssetsHandler))
	mux.Handle("GET /api/organizations/{id}/assets", basic(ListOrganizationAssetsHandler))
	mux.Handle("POST /api/organizations/{id}/assets", edit(CreateOrganizationAssetHandler))
	mux.Handle("GET /api/organization-assets/{id}", basic(GetOrganizationAssetHandler))
	mux.Handle("PATCH /api/organization-assets/{id}", edit(UpdateOrganizationAssetHandler))
	mux.Handle("DELETE /api/organization-assets/{id}", edit(DeleteOrganizationAssetHandler))
	mux.Handle("GET /api/organization-assets/{id}/payments", basic(ListAssetPaymentsHandler))
	mux.Handle("POST /api/organization-assets/{id}/payments", edit(CreateAssetPaymentHandler))
	mux.Handle("DELETE /api/asset-payments/{id}", edit(DeleteAssetPaymentHandler))

	// --- Site linking routes ---
	mux.Handle("GET /api/sites/{id}/organization", basic(GetSiteOrganizationHandler))
	mux.Handle("POST /api/sites/{id}/link", edit(LinkSiteHandler))
	mux.Handle("DELETE /api/sites/{id}/link", edit(UnlinkSiteHandler))

	// --- Note routes ---
	mux.Handle("GET /api/notes", basic(ListNotesHandler))
	mux.Handle("POST /api/notes", edit(CreateNoteHandler))
	mux.Handle("PATCH /api/notes/{id}", edit(UpdateNoteHandler))
	mux.Handle("DELETE /api/notes/{id}", edit(DeleteNoteHandler))
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
