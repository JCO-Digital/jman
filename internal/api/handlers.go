package api

import (
	"net/http"

	"github.com/JCO-Digital/jman/internal/config"
)

// RegisterHandlers registers all API routes to the provided mux.
// Authentication is mandatory: all data endpoints are protected by JWT middleware,
// while health and login endpoints remain public.
func RegisterHandlers(mux *http.ServeMux, version string, usersCfg config.UsersConfig, behindProxy bool) {
	limiter := NewLoginRateLimiter(behindProxy)

	// --- Public routes (no auth required) ---
	mux.HandleFunc("GET /api/health", HealthHandler(version))
	mux.HandleFunc("POST /api/auth/login", LoginHandler(&usersCfg, limiter))
	mux.HandleFunc("POST /api/auth/refresh", RefreshHandler(&usersCfg))
	mux.HandleFunc("POST /api/auth/logout", LogoutHandler)
	mux.HandleFunc("POST /api/cli/token", CLITokenHandler(&usersCfg, limiter))
	// Heartbeat check-ins authenticate with their check's token. They get a
	// limiter of their own so invalid check-ins can't lock out logins.
	checkInLimiter := NewLoginRateLimiter(behindProxy)
	mux.HandleFunc("POST /api/checkin", CheckInHandler(checkInLimiter))
	mux.HandleFunc("POST /api/checkin/{signal}", CheckInHandler(checkInLimiter))

	// --- Protected routes (JWT required with level check) ---
	basic := func(h http.HandlerFunc) http.Handler {
		return AuthMiddleware(&usersCfg, RequireLevel(config.LevelBasic)(h))
	}
	edit := func(h http.HandlerFunc) http.Handler {
		return AuthMiddleware(&usersCfg, RequireLevel(config.LevelEdit)(h))
	}
	admin := func(h http.HandlerFunc) http.Handler {
		return AuthMiddleware(&usersCfg, RequireLevel(config.LevelAdmin)(h))
	}
	execute := func(h http.HandlerFunc) http.Handler {
		return AuthMiddleware(&usersCfg, RequireLevel(config.LevelExecute)(h))
	}
	// --- jman CLI ---
	mux.Handle("POST /api/cli/authorize", basic(CLIAuthorizeHandler))
	mux.Handle("GET /api/cli/tokens", basic(ListCLITokensHandler))
	mux.Handle("DELETE /api/cli/tokens/{id}", basic(DeleteCLITokenHandler))
	mux.Handle("DELETE /api/cli/token", basic(RevokeCurrentCLITokenHandler))
	mux.Handle("GET /api/cli/snapshot", basic(CLISnapshotHandler))
	mux.Handle("POST /api/refresh", execute(RefreshDataHandler))

	mux.Handle("GET /api/plugins", basic(PluginsHandler))
	mux.Handle("GET /api/plugininfo", basic(PluginInfoHandler))
	// WordPress users of the managed sites; /api/users is jman's own.
	mux.Handle("GET /api/site-users", basic(SiteUsersHandler))
	mux.Handle("GET /api/servers", basic(ServersHandler))
	mux.Handle("POST /api/servers", edit(CreateServerHandler))
	mux.Handle("DELETE /api/servers/{id}", edit(DeleteServerHandler))

	mux.Handle("GET /api/sites", basic(SitesHandler))
	mux.Handle("POST /api/sites", edit(CreateSiteHandler))
	mux.Handle("POST /api/sites/batch", edit(BatchCreateSitesHandler))
	mux.Handle("GET /api/sites/{id}", basic(GetSiteHandler))
	mux.Handle("PUT /api/sites/{id}", edit(UpdateSiteHandler))
	mux.Handle("DELETE /api/sites/{id}", edit(DeleteSiteHandler))
	mux.Handle("POST /api/sites/{id}/test-connection", execute(TestSiteConnectionHandler))
	mux.Handle("POST /api/providers/spinupwp/sync", edit(SyncSpinupWPHandler))

	mux.Handle("GET /api/vulns", basic(VulnsHandler))
	mux.Handle("GET /api/vulns/core", basic(CoreVulnsHandler))

	// --- Agent routes (X-Agent-Token auth, not JWT) ---
	mux.Handle("GET /api/agent/manifest", AgentAuthMiddleware(AgentManifestHandler))
	mux.Handle("POST /api/agent/report", AgentAuthMiddleware(AgentReportHandler))

	// --- Agent token management (JWT, admin level) ---
	mux.Handle("GET /api/agent-tokens", admin(ListAgentTokensHandler))
	mux.Handle("POST /api/agent-tokens", admin(CreateAgentTokenHandler))
	mux.Handle("DELETE /api/agent-tokens/{id}", admin(RevokeAgentTokenHandler))

	// --- User Management ---
	mux.Handle("GET /api/users", basic(ListUsersHandler(&usersCfg)))
	mux.Handle("POST /api/users", admin(CreateUserHandler(&usersCfg)))
	mux.Handle("PATCH /api/users/{username}", admin(UpdateUserHandler(&usersCfg)))
	mux.Handle("DELETE /api/users/{username}", admin(DeleteUserHandler(&usersCfg)))

	// --- User Self-Service ---
	mux.Handle("GET /api/user/profile", basic(GetProfileHandler(&usersCfg)))
	mux.Handle("PATCH /api/user/profile", basic(UpdateProfileHandler(&usersCfg)))
	mux.Handle("POST /api/user/password", basic(ChangePasswordHandler(&usersCfg, limiter)))
	mux.Handle("POST /api/user/2fa/setup", basic(Setup2FAHandler(&usersCfg)))
	mux.Handle("POST /api/user/2fa/activate", basic(Activate2FAHandler(&usersCfg, limiter)))
	mux.Handle("POST /api/user/2fa/deactivate", basic(Deactivate2FAHandler(&usersCfg, limiter)))

	// --- Monitoring routes ---
	mux.Handle("GET /api/monitor/history", basic(MonitorHistoryHandler))
	mux.Handle("GET /api/monitor/status", basic(MonitorStatusHandler))
	mux.Handle("GET /api/monitor/checks", basic(ListMonitorChecksHandler))
	mux.Handle("POST /api/monitor/checks", edit(CreateMonitorCheckHandler))
	mux.Handle("POST /api/monitor/checks/test", edit(TestMonitorCheckHandler))
	mux.Handle("GET /api/monitor/checks/{id}", basic(GetMonitorCheckHandler))
	mux.Handle("PATCH /api/monitor/checks/{id}", edit(UpdateMonitorCheckHandler))
	mux.Handle("DELETE /api/monitor/checks/{id}", edit(DeleteMonitorCheckHandler))
	mux.Handle("POST /api/monitor/checks/{id}/pause", edit(PauseMonitorCheckHandler))
	mux.Handle("POST /api/monitor/checks/{id}/resume", edit(ResumeMonitorCheckHandler))
	mux.Handle("POST /api/monitor/checks/{id}/reset", edit(ResetMonitorCheckHandler))
	mux.Handle("POST /api/monitor/checks/{id}/token", edit(IssueMonitorCheckTokenHandler))
	mux.Handle("PUT /api/monitor/checks/{id}/secrets/{header}", edit(SetMonitorCheckSecretHandler))
	mux.Handle("DELETE /api/monitor/checks/{id}/secrets/{header}", edit(DeleteMonitorCheckSecretHandler))

	// --- Incident routes ---
	mux.Handle("GET /api/incidents", basic(ListIncidentsHandler))
	mux.Handle("POST /api/incidents/{id}/acknowledge", basic(AcknowledgeIncidentHandler))
	mux.Handle("POST /api/incidents/{id}/close", basic(CloseIncidentHandler))
	mux.Handle("POST /api/incidents/{id}/pause", edit(PauseIncidentCheckHandler))

	// --- Ignore routes ---
	mux.Handle("GET /api/ignore", basic(ListIgnoreEntriesHandler))
	mux.Handle("POST /api/ignore", edit(CreateIgnoreEntryHandler))
	mux.Handle("PATCH /api/ignore/{id}", edit(UpdateIgnoreEntryHandler))
	mux.Handle("DELETE /api/ignore/{id}", edit(DeleteIgnoreEntryHandler))

	// --- Vulnerability backport routes ---
	mux.Handle("GET /api/vuln-backports", basic(ListVulnBackportsHandler))
	mux.Handle("POST /api/vuln-backports", edit(CreateVulnBackportHandler))
	mux.Handle("DELETE /api/vuln-backports/{id}", edit(DeleteVulnBackportHandler))

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

	// --- Payment Method routes ---
	mux.Handle("GET /api/payment-methods", basic(ListPaymentMethodsHandler))
	mux.Handle("POST /api/payment-methods", edit(CreatePaymentMethodHandler))
	mux.Handle("GET /api/payment-methods/{id}", basic(GetPaymentMethodHandler))
	mux.Handle("PATCH /api/payment-methods/{id}", edit(UpdatePaymentMethodHandler))
	mux.Handle("DELETE /api/payment-methods/{id}", edit(DeletePaymentMethodHandler))

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
	mux.Handle("GET /api/sites/{id}/traffic", basic(SiteTrafficHandler))
	mux.Handle("GET /api/sites/{id}/organization", basic(GetSiteOrganizationHandler))
	mux.Handle("POST /api/sites/{id}/link", edit(LinkSiteHandler))
	mux.Handle("DELETE /api/sites/{id}/link", edit(UnlinkSiteHandler))
	mux.Handle("PATCH /api/sites/{id}/environment", edit(SetSiteEnvironmentHandler))

	// --- Plugin update routes ---
	mux.Handle("GET /api/sites/{id}/plugin-updates", execute(SitePluginUpdatesHandler))
	mux.Handle("POST /api/plugin-update-jobs", execute(CreatePluginUpdateJobsHandler))
	// The plugin-update-jobs list/get routes predate core jobs; they return
	// the same jobs as update-jobs.
	mux.Handle("GET /api/plugin-update-jobs", basic(ListUpdateJobsHandler))
	mux.Handle("GET /api/plugin-update-jobs/{id}", basic(GetUpdateJobHandler))

	// --- Background update jobs (plugins and core) ---
	mux.Handle("GET /api/update-jobs", basic(ListUpdateJobsHandler))
	mux.Handle("GET /api/update-jobs/{id}", basic(GetUpdateJobHandler))

	// --- Plugin management routes (background jobs) ---
	mux.Handle("GET /api/sites/{id}/plugins", basic(SitePluginsHandler))
	mux.Handle("GET /api/sites/{id}/users", basic(SiteUserListHandler))
	mux.Handle("POST /api/sites/{id}/users", execute(SiteUserCreateHandler))
	mux.Handle("DELETE /api/sites/{id}/users/{userId}", execute(SiteUserDeleteHandler))
	mux.Handle("POST /api/sites/{id}/plugin-actions", execute(SitePluginActionHandler))
	mux.Handle("POST /api/sites/{id}/plugin-install", execute(SitePluginInstallHandler))

	// --- Update locks (fix-release-only updates) ---
	mux.Handle("GET /api/update-locks", basic(ListUpdateLocksHandler))
	mux.Handle("POST /api/sites/{id}/update-locks", execute(CreateUpdateLockHandler))
	mux.Handle("DELETE /api/update-locks/{id}", execute(DeleteUpdateLockHandler))

	// --- Core update routes ---
	mux.Handle("GET /api/sites/{id}/core-update", execute(SiteCoreCheckHandler))
	mux.Handle("POST /api/sites/{id}/core-update", execute(SiteCoreUpdateHandler))

	// --- Update Ledger routes ---
	mux.Handle("GET /api/sites/{id}/update-ledger", basic(SiteUpdateLedgerHandler))
	mux.Handle("POST /api/sites/{id}/update-ledger", edit(CreateSiteUpdateLedgerHandler))

	// --- Note routes ---
	mux.Handle("GET /api/notes", basic(ListNotesHandler))
	mux.Handle("POST /api/notes", edit(CreateNoteHandler))
	mux.Handle("PATCH /api/notes/{id}", edit(UpdateNoteHandler))
	mux.Handle("DELETE /api/notes/{id}", edit(DeleteNoteHandler))

	// --- Task routes ---
	mux.Handle("GET /api/tasks", basic(ListTasksHandler))
	mux.Handle("GET /api/tasks/{id}", basic(GetTaskHandler))
	mux.Handle("POST /api/tasks", edit(CreateTaskHandler))
	mux.Handle("PATCH /api/tasks/{id}", edit(UpdateTaskHandler))
	mux.Handle("POST /api/tasks/{id}/complete", edit(CompleteTaskHandler))
	mux.Handle("DELETE /api/tasks/{id}", edit(DeleteTaskHandler))

	// --- Report routes ---
	mux.Handle("GET /api/reports", basic(ListReportsHandler))
	mux.Handle("GET /api/reports/{id}/run", basic(RunReportHandler))

	// --- Settings routes ---
	mux.Handle("GET /api/settings", basic(ListSettingsHandler))
	mux.Handle("GET /api/settings/{key}", basic(GetSettingHandler))
	mux.Handle("POST /api/settings/{key}", basic(SaveSettingHandler))
	mux.Handle("PATCH /api/settings/{key}", basic(PatchSettingHandler))
	mux.Handle("DELETE /api/settings/{key}", basic(DeleteSettingHandler))

	// --- Vulnerability task settings (admin) ---
	mux.Handle("GET /api/vuln-settings", admin(GetVulnSettingsHandler))
	mux.Handle("POST /api/vuln-settings", admin(SaveVulnSettingsHandler(&usersCfg)))
}

// HealthHandler returns a simple health check response.
func HealthHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"version": version,
			"webUrl":  config.Cfg.WebURL,
		})
	}
}
