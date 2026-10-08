package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// maxAgentReportBodyBytes bounds the size of a single agent report to guard
// against a misbehaving or compromised agent token flooding the server.
const maxAgentReportBodyBytes = 2 * 1024 * 1024 // 2 MiB

// AgentManifestHandler tells a jman-agent instance which sites it should
// collect data for on its own server, keyed by the server identified by its
// agent token.
func AgentManifestHandler(w http.ResponseWriter, r *http.Request) {
	claims := GetAgentClaims(r.Context())
	if claims == nil {
		WriteError(w, http.StatusUnauthorized, "Agent authentication required")
		return
	}

	sites, err := cache.GetSitesForServer(claims.ServerID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to load sites for server %s: %v", claims.ServerID, err)
		WriteError(w, http.StatusInternalServerError, "Failed to load sites")
		return
	}

	manifest := models.AgentManifest{
		ServerID:   claims.ServerID,
		Sites:      make([]models.AgentManifestSite, 0, len(sites)),
		APIVersion: config.AppVersion,
	}
	for _, site := range sites {
		// SpinupWP sites go through deploying -> deployed (or failed), and
		// external sites are active/paused/archived; a site that isn't live
		// yet (e.g. a staging site mid-clone) or has been paused has nothing
		// for the agent to collect, so leave it out of the manifest
		// entirely. site_user is passed through only as an optional
		// fallback hint (see AgentManifestSite doc comment) — it's not
		// required, since most servers use SpinupWP's shared /sites/<domain>
		// layout rather than a dedicated Unix user per site.
		if site.Status != "deployed" && site.Status != "active" {
			verb.LogPrintf(verb.Verbose, "Excluding %s from agent manifest: status=%q", site.Domain, site.Status)
			continue
		}

		entry := models.AgentManifestSite{
			SiteID:      site.ID,
			Domain:      site.Domain,
			SiteUser:    site.SSHUser,
			IsWordpress: site.IsWordpress,
		}
		if site.Provider == "spinupwp" {
			entry.LegacySiteID, _ = strconv.Atoi(site.ProviderSiteID)
		}
		manifest.Sites = append(manifest.Sites, entry)
	}

	WriteJSON(w, http.StatusOK, manifest)
}

// validAutoUpdateCore holds the core auto-update settings an agent may
// report; anything else is stored as unknown.
var validAutoUpdateCore = map[string]bool{"true": true, "false": true, "minor": true, "disabled": true, "default": true}

// AgentReportHandler ingests a batched report of freshly collected per-site
// data from a jman-agent instance and persists it.
func AgentReportHandler(w http.ResponseWriter, r *http.Request) {
	claims := GetAgentClaims(r.Context())
	if claims == nil {
		WriteError(w, http.StatusUnauthorized, "Agent authentication required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAgentReportBodyBytes)

	var report models.AgentReport
	if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if report.AgentVersion != "" {
		if err := db.SetAgentTokenVersion(claims.TokenID, report.AgentVersion); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to record agent version for token %d: %v", claims.TokenID, err)
		}
	}

	// Only accept data for sites that actually belong to the reporting
	// server, so a compromised or misconfigured token can't overwrite data
	// for sites on other servers.
	serverSites, err := cache.GetSitesForServer(claims.ServerID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to validate sites for server %s: %v", claims.ServerID, err)
		WriteError(w, http.StatusInternalServerError, "Failed to validate sites")
		return
	}
	allowedSiteIDs := make(map[string]bool, len(serverSites))
	for _, site := range serverSites {
		allowedSiteIDs[site.ID] = true
	}

	measuredAt := time.Now().UTC()
	if report.CollectedAt != "" {
		if t, err := time.Parse(time.RFC3339, report.CollectedAt); err == nil {
			measuredAt = t
		} else {
			verb.LogPrintf(verb.Normal, "Agent report from server %s has invalid collected_at %q; using receive time", claims.ServerID, report.CollectedAt)
		}
	}

	type siteDay struct {
		siteID string
		day    string
	}
	dailyRollupsNeeded := map[siteDay]bool{}

	accepted := 0
	for _, siteReport := range report.Sites {
		if !allowedSiteIDs[siteReport.SiteID] {
			verb.LogPrintf(verb.Normal, "Rejected agent report for site %s: does not belong to server %s", siteReport.SiteID, claims.ServerID)
			continue
		}

		if siteReport.DiskUsageBytes != nil {
			if err := db.RecordSiteDiskUsage(siteReport.SiteID, *siteReport.DiskUsageBytes, measuredAt); err != nil {
				verb.LogPrintf(verb.Normal, "Failed to record disk usage for site %s: %v", siteReport.SiteID, err)
			}
		}

		if siteReport.IsMultisite != nil || siteReport.DisallowFileMods != nil {
			isMultisite := siteReport.IsMultisite != nil && *siteReport.IsMultisite
			disallowFileMods := siteReport.DisallowFileMods != nil && *siteReport.DisallowFileMods
			autoUpdateCore := siteReport.AutoUpdateCore
			if autoUpdateCore != nil && !validAutoUpdateCore[*autoUpdateCore] {
				autoUpdateCore = nil
			}
			if err := db.SetSiteWpFlags(siteReport.SiteID, isMultisite, disallowFileMods, autoUpdateCore); err != nil {
				verb.LogPrintf(verb.Normal, "Failed to set wp flags for site %s: %v", siteReport.SiteID, err)
			}
		}

		for _, hourly := range siteReport.TrafficHourly {
			if err := db.UpsertSiteTrafficHourly(siteReport.SiteID, hourly); err != nil {
				verb.LogPrintf(verb.Normal, "Failed to record traffic for site %s hour %s: %v", siteReport.SiteID, hourly.Hour, err)
				continue
			}
			if hourTime, err := time.Parse(time.RFC3339, hourly.Hour); err == nil {
				dailyRollupsNeeded[siteDay{siteID: siteReport.SiteID, day: hourTime.Format("2006-01-02")}] = true
			}
		}

		// Pre-aggregated backlog beyond jman-agent's hourly retention window
		// (see models.TrafficDailyEntry) — stored directly, and deliberately
		// NOT added to dailyRollupsNeeded, since there's no corresponding
		// hourly data to recompute from (that would just overwrite it with
		// an empty rollup).
		for _, daily := range siteReport.TrafficDaily {
			if err := db.UpsertSiteTrafficDaily(siteReport.SiteID, daily); err != nil {
				verb.LogPrintf(verb.Normal, "Failed to record traffic for site %s day %s: %v", siteReport.SiteID, daily.Day, err)
			}
		}

		accepted++
	}

	for sd := range dailyRollupsNeeded {
		if err := db.RecomputeSiteTrafficDaily(sd.siteID, sd.day); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to recompute daily traffic rollup for site %s day %s: %v", sd.siteID, sd.day, err)
		}
	}

	WriteJSON(w, http.StatusOK, map[string]int{"accepted": accepted, "rejected": len(report.Sites) - accepted})
}

// ListAgentTokensHandler returns every agent token (admin only).
func ListAgentTokensHandler(w http.ResponseWriter, r *http.Request) {
	tokens, err := db.ListAgentTokens()
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListAgentTokens error: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, tokens)
}

type createAgentTokenRequest struct {
	ServerID    FlexID `json:"server_id"` // server UUID (legacy SpinupWP integer accepted)
	ServerName  string `json:"server_name"`
	Description string `json:"description"`
}

type createAgentTokenResponse struct {
	models.AgentToken
	Token string `json:"token"`
}

// CreateAgentTokenHandler mints a new per-server agent token (admin only).
// The plaintext token is returned exactly once — it cannot be retrieved again.
func CreateAgentTokenHandler(w http.ResponseWriter, r *http.Request) {
	var req createAgentTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.ServerID == "" {
		WriteError(w, http.StatusBadRequest, "server_id is required")
		return
	}
	serverID, err := resolveServerUUID(string(req.ServerID))
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	server, err := db.GetManagedServer(serverID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to look up server %s: %v", serverID, err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if server == nil {
		WriteError(w, http.StatusNotFound, "Server not found")
		return
	}
	if server.IsLogical {
		WriteError(w, http.StatusBadRequest, "jman-agent cannot run on a logical server")
		return
	}
	serverName := strings.TrimSpace(req.ServerName)
	if serverName == "" {
		serverName = server.Name
	}

	claims := GetAuthClaims(r.Context())
	createdBy := ""
	if claims != nil {
		createdBy = claims.Username
	}

	token, plaintext, err := db.CreateAgentToken(serverID, serverName, req.Description, createdBy)
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to create agent token: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to create agent token")
		return
	}
	token.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	WriteJSON(w, http.StatusCreated, createAgentTokenResponse{AgentToken: token, Token: plaintext})
}

// RevokeAgentTokenHandler revokes an agent token by ID (admin only).
func RevokeAgentTokenHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid token ID")
		return
	}

	if err := db.RevokeAgentToken(id); err != nil {
		verb.LogPrintf(verb.Normal, "Failed to revoke agent token %d: %v", id, err)
		WriteError(w, http.StatusNotFound, "Agent token not found")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}
