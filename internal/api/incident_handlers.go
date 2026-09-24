package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/monitor"
	"github.com/JCO-Digital/jman/internal/pagerduty"
	"github.com/JCO-Digital/jman/internal/slack"
	"github.com/JCO-Digital/jman/internal/verb"
)

// IncidentsResponse represents the JSON response for listing incidents.
type IncidentsResponse struct {
	Incidents   []models.Incident `json:"incidents"`
	Total       int               `json:"total"`
	ActiveCount int               `json:"active_count"`
}

// ListIncidentsHandler handles GET /api/incidents.
// Query params: filter (active|resolved|closed|history|all), page, limit.
func ListIncidentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	filter := r.URL.Query().Get("filter")
	if filter == "" {
		filter = "active"
	}

	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}

	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}

	offset := (page - 1) * limit

	incidents, total, err := db.GetIncidents(filter, limit, offset)
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListIncidentsHandler: failed to fetch incidents: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	activeCount, err := db.GetActiveIncidentCount()
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListIncidentsHandler: failed to get active count: %v", err)
	}

	if incidents == nil {
		incidents = []models.Incident{}
	}

	WriteJSON(w, http.StatusOK, IncidentsResponse{
		Incidents:   incidents,
		Total:       total,
		ActiveCount: activeCount,
	})
}

// AcknowledgeIncidentHandler handles POST /api/incidents/{id}/acknowledge.
func AcknowledgeIncidentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid incident ID")
		return
	}

	existing, err := db.GetIncidentByID(id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "AcknowledgeIncidentHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if existing == nil {
		WriteError(w, http.StatusNotFound, "Incident not found")
		return
	}

	claims := GetAuthClaims(r.Context())
	username := "User"
	if claims != nil && claims.Username != "" {
		username = claims.Username
	}

	inc, err := db.AcknowledgeIncident(id, username)
	if err != nil {
		verb.LogPrintf(verb.Normal, "AcknowledgeIncidentHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// If PagerDuty was triggered, resolve it in PagerDuty so on-call paging stops
	if inc.PDTriggered && pagerduty.Enabled() {
		if err := pagerduty.ResolveEvent(inc.Domain); err != nil {
			verb.LogPrintf(verb.Normal, "Failed to resolve PagerDuty on acknowledge for %s: %v", inc.Domain, err)
		}
	}

	// Send Slack acknowledgment notification
	slackChannel := config.Cfg.SlackMonitorChannel
	if slackChannel == "" {
		slackChannel = config.Cfg.SlackChannel
	}
	slackMsg := fmt.Sprintf("👀 Outage for site *%s* acknowledged by *%s*", inc.Domain, username)
	if err := slack.SendMessageToChannel(slackMsg, slackChannel, true); err != nil {
		verb.LogPrintf(verb.Normal, "Failed to send Slack ack message: %v", err)
	}

	WriteJSON(w, http.StatusOK, inc)
}

// CloseIncidentHandler handles POST /api/incidents/{id}/close.
func CloseIncidentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid incident ID")
		return
	}

	existing, err := db.GetIncidentByID(id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "CloseIncidentHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if existing == nil {
		WriteError(w, http.StatusNotFound, "Incident not found")
		return
	}

	claims := GetAuthClaims(r.Context())
	username := "User"
	if claims != nil && claims.Username != "" {
		username = claims.Username
	}

	inc, err := db.CloseIncident(id, username)
	if err != nil {
		verb.LogPrintf(verb.Normal, "CloseIncidentHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Resolve in PagerDuty if triggered
	if existing.PDTriggered && pagerduty.Enabled() {
		_ = pagerduty.ResolveEvent(existing.Domain)
	}

	// Reset monitor status so if site is still down, it will re-investigate and alert again
	monitor.ResetDomainStatus(existing.Domain)

	WriteJSON(w, http.StatusOK, inc)
}

// IgnoreIncidentRequest represents the optional payload when ignoring from an incident.
type IgnoreIncidentRequest struct {
	Reason        string `json:"reason"`
	UseForMonitor bool   `json:"use_for_monitor"`
	UseForVuln    bool   `json:"use_for_vuln"`
}

// IgnoreIncidentHandler handles POST /api/incidents/{id}/ignore.
func IgnoreIncidentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid incident ID")
		return
	}

	existing, err := db.GetIncidentByID(id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "IgnoreIncidentHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if existing == nil {
		WriteError(w, http.StatusNotFound, "Incident not found")
		return
	}

	var req IgnoreIncidentRequest
	req.UseForMonitor = true // default true
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.Reason == "" {
		req.Reason = fmt.Sprintf("Ignored from incident #%d (%s)", existing.ID, existing.Domain)
	}

	claims := GetAuthClaims(r.Context())
	username := "User"
	if claims != nil && claims.Username != "" {
		username = claims.Username
	}

	// Look up site ID in cache for site-level ignore
	target := existing.Domain
	entryType := "site"
	if cachedSites, err := cache.GetCachedSites(); err == nil {
		for _, s := range cachedSites {
			if strings.EqualFold(s.Domain, existing.Domain) {
				target = strconv.Itoa(s.ID)
				break
			}
		}
	}

	ignoreEntry := &models.IgnoreEntry{
		Type:          entryType,
		Target:        target,
		Reason:        req.Reason,
		UseForMonitor: req.UseForMonitor,
		UseForVuln:    req.UseForVuln,
		CreatedAt:     time.Now(),
		CreatedBy:     username,
		UpdatedAt:     time.Now(),
		UpdatedBy:     username,
	}

	if err := db.SaveIgnoreEntry(ignoreEntry, username); err != nil {
		verb.LogPrintf(verb.Normal, "IgnoreIncidentHandler: failed to save ignore entry: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Close incident
	inc, err := db.CloseIncident(id, username)
	if err != nil {
		verb.LogPrintf(verb.Normal, "IgnoreIncidentHandler: failed to close incident: %v", err)
	}

	// Resolve in PagerDuty if triggered
	if existing.PDTriggered && pagerduty.Enabled() {
		_ = pagerduty.ResolveEvent(existing.Domain)
	}

	// Reset monitor status and notify
	monitor.ResetDomainStatus(existing.Domain)
	monitor.NotifyIfAlertingSiteIgnored(existing.Domain, req.Reason)

	WriteJSON(w, http.StatusOK, inc)
}
