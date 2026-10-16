package api

import (
	"encoding/json"
	"net/http"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/monitor"
	"github.com/JCO-Digital/jman/internal/verb"
)

// MonitorCheckResponse is a monitor check with the host and URL it checks.
// Default checks take the host from their site, so Domain is empty when
// that site is unknown.
type MonitorCheckResponse struct {
	models.MonitorCheck
	Domain string `json:"domain"`
	URL    string `json:"url,omitempty"`
}

// ListMonitorChecksHandler handles GET /api/monitor/checks.
// Query param: site_id (optional) limits the list to one site's checks.
func ListMonitorChecksHandler(w http.ResponseWriter, r *http.Request) {
	checks, err := db.ListMonitorChecks()
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListMonitorChecksHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	siteFilter := r.URL.Query().Get("site_id")
	if siteFilter != "" {
		if siteFilter, err = resolveSiteUUID(siteFilter); err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	domains, err := siteDomains()
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListMonitorChecksHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	resp := []MonitorCheckResponse{}
	for _, c := range checks {
		if siteFilter != "" && c.SiteID != siteFilter {
			continue
		}
		resp = append(resp, describeMonitorCheck(c, domains))
	}
	WriteJSON(w, http.StatusOK, resp)
}

// MonitorCheckPauseRequest is the optional payload when pausing a check.
type MonitorCheckPauseRequest struct {
	Reason string `json:"reason"`
}

// PauseMonitorCheckHandler handles POST /api/monitor/checks/{id}/pause.
func PauseMonitorCheckHandler(w http.ResponseWriter, r *http.Request) {
	check, desc, ok := loadMonitorCheckForRequest(w, r, "PauseMonitorCheckHandler")
	if !ok {
		return
	}

	var req MonitorCheckPauseRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	username := requestUsername(r)
	if req.Reason == "" {
		req.Reason = "Paused by " + username
	}

	if err := pauseMonitorCheck(check, monitorCheckLabel(desc), username, req.Reason); err != nil {
		verb.LogPrintf(verb.Normal, "PauseMonitorCheckHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	desc.Enabled = false
	WriteJSON(w, http.StatusOK, desc)
}

// ResumeMonitorCheckHandler handles POST /api/monitor/checks/{id}/resume.
func ResumeMonitorCheckHandler(w http.ResponseWriter, r *http.Request) {
	check, desc, ok := loadMonitorCheckForRequest(w, r, "ResumeMonitorCheckHandler")
	if !ok {
		return
	}

	if err := db.SetMonitorCheckEnabled(check.ID, true, requestUsername(r)); err != nil {
		verb.LogPrintf(verb.Normal, "ResumeMonitorCheckHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	desc.Enabled = true
	WriteJSON(w, http.StatusOK, desc)
}

// pauseMonitorCheck pauses a check and closes out its outage: an active
// incident is resolved, its PagerDuty alert resolved and Slack told the
// check was paused. label names the check in the Slack message.
func pauseMonitorCheck(check *models.MonitorCheck, label, username, reason string) error {
	if err := db.SetMonitorCheckEnabled(check.ID, false, username); err != nil {
		return err
	}
	return monitor.CloseOutCheck(check.ID, label, monitor.StopPaused, username, reason)
}

// loadMonitorCheckForRequest loads the check named by the {id} path value,
// writing the error response and returning false when there is none.
func loadMonitorCheckForRequest(w http.ResponseWriter, r *http.Request, handler string) (*models.MonitorCheck, MonitorCheckResponse, bool) {
	check, err := db.GetMonitorCheck(r.PathValue("id"))
	if err != nil {
		verb.LogPrintf(verb.Normal, "%s: %v", handler, err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return nil, MonitorCheckResponse{}, false
	}
	if check == nil {
		WriteError(w, http.StatusNotFound, "Monitor check not found")
		return nil, MonitorCheckResponse{}, false
	}

	domains, err := siteDomains()
	if err != nil {
		verb.LogPrintf(verb.Normal, "%s: %v", handler, err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return nil, MonitorCheckResponse{}, false
	}
	return check, describeMonitorCheck(*check, domains), true
}

// siteDomains maps every managed site's UUID to its domain.
func siteDomains() (map[string]string, error) {
	sites, err := db.ListManagedSites()
	if err != nil {
		return nil, err
	}
	domains := make(map[string]string, len(sites))
	for _, s := range sites {
		domains[s.ID] = s.Domain
	}
	return domains, nil
}

func describeMonitorCheck(c models.MonitorCheck, siteDomains map[string]string) MonitorCheckResponse {
	host := c.Host
	if c.IsDefault {
		host = siteDomains[c.SiteID]
	}
	resp := MonitorCheckResponse{MonitorCheck: c, Domain: host}
	if host != "" {
		resp.URL = "https://" + host + c.Path
	}
	return resp
}

// monitorCheckLabel names a check in messages: its name, else its domain
// plus path, else its ID.
func monitorCheckLabel(c MonitorCheckResponse) string {
	switch {
	case c.Name != "":
		return c.Name
	case c.Domain == "":
		return c.ID
	case c.Path != "" && c.Path != "/":
		return c.Domain + c.Path
	default:
		return c.Domain
	}
}

func requestUsername(r *http.Request) string {
	if claims := GetAuthClaims(r.Context()); claims != nil && claims.Username != "" {
		return claims.Username
	}
	return "User"
}
