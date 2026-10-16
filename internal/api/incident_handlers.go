package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

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

	// Reset monitor status so if the check still fails, it will re-investigate and alert again
	if existing.CheckID != "" {
		monitor.ResetCheckStatus(existing.CheckID)
	}

	WriteJSON(w, http.StatusOK, inc)
}

// PauseIncidentCheckRequest is the optional payload when pausing a check from an incident.
type PauseIncidentCheckRequest struct {
	Reason string `json:"reason"`
}

// PauseIncidentCheckHandler handles POST /api/incidents/{id}/pause: it
// pauses the incident's monitor check and closes the incident.
func PauseIncidentCheckHandler(w http.ResponseWriter, r *http.Request) {
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
		verb.LogPrintf(verb.Normal, "PauseIncidentCheckHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if existing == nil {
		WriteError(w, http.StatusNotFound, "Incident not found")
		return
	}
	if existing.CheckID == "" {
		WriteError(w, http.StatusConflict, "Incident has no monitor check")
		return
	}
	check, err := db.GetMonitorCheck(existing.CheckID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "PauseIncidentCheckHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if check == nil {
		WriteError(w, http.StatusNotFound, "Monitor check not found")
		return
	}

	var req PauseIncidentCheckRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.Reason == "" {
		req.Reason = fmt.Sprintf("Paused from incident #%d (%s)", existing.ID, existing.Domain)
	}

	claims := GetAuthClaims(r.Context())
	username := "User"
	if claims != nil && claims.Username != "" {
		username = claims.Username
	}

	if err := pauseMonitorCheck(check, existing.Domain, username, req.Reason); err != nil {
		verb.LogPrintf(verb.Normal, "PauseIncidentCheckHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	inc, err := db.GetIncidentByID(id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "PauseIncidentCheckHandler: %v", err)
	}
	WriteJSON(w, http.StatusOK, inc)
}
