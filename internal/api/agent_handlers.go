package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
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
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to load sites: %v", err))
		return
	}

	manifest := models.AgentManifest{
		ServerID: claims.ServerID,
		Sites:    make([]models.AgentManifestSite, 0, len(sites)),
	}
	for _, site := range sites {
		// SpinupWP sites go through deploying -> deployed (or failed); a
		// site that isn't deployed yet (e.g. a staging site mid-clone) has
		// nothing for the agent to collect, so leave it out of the manifest
		// entirely. site_user is passed through only as an optional
		// fallback hint (see AgentManifestSite doc comment) — it's not
		// required, since most servers use SpinupWP's shared /sites/<domain>
		// layout rather than a dedicated Unix user per site.
		if site.Status != "deployed" {
			verb.LogPrintf(verb.Verbose, "Excluding %s from agent manifest: status=%q", site.Domain, site.Status)
			continue
		}

		manifest.Sites = append(manifest.Sites, models.AgentManifestSite{
			SiteID:   site.ID,
			Domain:   site.Domain,
			SiteUser: site.SiteUser,
		})
	}

	WriteJSON(w, http.StatusOK, manifest)
}

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
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("Invalid request body: %v", err))
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
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to validate sites: %v", err))
		return
	}
	allowedSiteIDs := make(map[int]bool, len(serverSites))
	for _, site := range serverSites {
		allowedSiteIDs[site.ID] = true
	}

	measuredAt := report.CollectedAt
	if measuredAt == "" {
		measuredAt = time.Now().UTC().Format(time.RFC3339)
	}

	accepted := 0
	for _, siteReport := range report.Sites {
		if !allowedSiteIDs[siteReport.SiteID] {
			verb.LogPrintf(verb.Normal, "Rejected agent report for site %d: does not belong to server %d", siteReport.SiteID, claims.ServerID)
			continue
		}

		if siteReport.DiskUsageBytes != nil {
			if err := db.RecordSiteDiskUsage(siteReport.SiteID, *siteReport.DiskUsageBytes, measuredAt); err != nil {
				verb.LogPrintf(verb.Normal, "Failed to record disk usage for site %d: %v", siteReport.SiteID, err)
			}
		}

		if siteReport.IsMultisite != nil || siteReport.DisallowFileMods != nil {
			isMultisite := siteReport.IsMultisite != nil && *siteReport.IsMultisite
			disallowFileMods := siteReport.DisallowFileMods != nil && *siteReport.DisallowFileMods
			if err := db.SetSiteWpFlags(siteReport.SiteID, isMultisite, disallowFileMods); err != nil {
				verb.LogPrintf(verb.Normal, "Failed to set wp flags for site %d: %v", siteReport.SiteID, err)
			}
		}

		accepted++
	}

	WriteJSON(w, http.StatusOK, map[string]int{"accepted": accepted, "rejected": len(report.Sites) - accepted})
}

// ListAgentTokensHandler returns every agent token (admin only).
func ListAgentTokensHandler(w http.ResponseWriter, r *http.Request) {
	tokens, err := db.ListAgentTokens()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Database error: %v", err))
		return
	}
	WriteJSON(w, http.StatusOK, tokens)
}

type createAgentTokenRequest struct {
	ServerID    int    `json:"server_id"`
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
	if req.ServerID == 0 {
		WriteError(w, http.StatusBadRequest, "server_id is required")
		return
	}

	claims := GetAuthClaims(r.Context())
	createdBy := ""
	if claims != nil {
		createdBy = claims.Username
	}

	token, plaintext, err := db.CreateAgentToken(req.ServerID, req.ServerName, req.Description, createdBy)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create agent token: %v", err))
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
		WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}
