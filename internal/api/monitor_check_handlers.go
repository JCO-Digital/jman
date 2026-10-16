package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/monitor"
	"github.com/JCO-Digital/jman/internal/verb"
)

// MonitorCheckResponse is a monitor check with the host and URL it checks
// and the names of its secret headers. Default checks take the host from
// their site, so Domain is empty when that site is unknown.
type MonitorCheckResponse struct {
	models.MonitorCheck
	Domain        string                      `json:"domain"`
	URL           string                      `json:"url,omitempty"`
	SecretHeaders []models.MonitorCheckSecret `json:"secret_headers"`
	// TokenPrefix and TokenCreatedAt describe a heartbeat check's check-in
	// token. Token is the token itself, only when it was just issued.
	TokenPrefix    string     `json:"token_prefix,omitempty"`
	TokenCreatedAt *time.Time `json:"token_created_at,omitempty"`
	Token          string     `json:"token,omitempty"`
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
	secrets, err := db.ListMonitorCheckSecrets()
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListMonitorChecksHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	secretsByCheck := map[string][]models.MonitorCheckSecret{}
	for _, s := range secrets {
		secretsByCheck[s.CheckID] = append(secretsByCheck[s.CheckID], s)
	}
	tokens, err := db.ListMonitorCheckTokens()
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
		resp = append(resp, describeMonitorCheck(c, domains, secretsByCheck[c.ID], tokens[c.ID]))
	}
	WriteJSON(w, http.StatusOK, resp)
}

// GetMonitorCheckHandler handles GET /api/monitor/checks/{id}.
func GetMonitorCheckHandler(w http.ResponseWriter, r *http.Request) {
	_, desc, ok := loadMonitorCheckForRequest(w, r, "GetMonitorCheckHandler")
	if !ok {
		return
	}
	WriteJSON(w, http.StatusOK, desc)
}

// CreateMonitorCheckHandler handles POST /api/monitor/checks. It creates a
// check of its own (not a site's default check), enabled. A heartbeat
// check gets a check-in token, returned in the response this once.
func CreateMonitorCheckHandler(w http.ResponseWriter, r *http.Request) {
	var req MonitorCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	check := models.MonitorCheck{Type: models.MonitorCheckTypeHTTP, Enabled: true}
	if req.Type != nil {
		switch *req.Type {
		case models.MonitorCheckTypeHTTP, models.MonitorCheckTypeHeartbeat:
			check.Type = *req.Type
		default:
			WriteError(w, http.StatusBadRequest, "type must be http or heartbeat")
			return
		}
	}
	if req.SiteID != nil && strings.TrimSpace(*req.SiteID) != "" {
		siteID, ok := resolveCheckSite(w, *req.SiteID)
		if !ok {
			return
		}
		check.SiteID = siteID
	}
	if err := req.applyTo(&check); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	username := requestUsername(r)
	if err := db.CreateMonitorCheck(&check, username); err != nil {
		verb.LogPrintf(verb.Normal, "CreateMonitorCheckHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if check.Type == models.MonitorCheckTypeHeartbeat {
		writeIssuedToken(w, http.StatusCreated, &check, username, "CreateMonitorCheckHandler")
		return
	}
	writeMonitorCheck(w, http.StatusCreated, &check, "CreateMonitorCheckHandler")
}

// UpdateMonitorCheckHandler handles PATCH /api/monitor/checks/{id}. A
// change to what the check probes closes out its outage, as the old
// result no longer says anything about the new settings.
func UpdateMonitorCheckHandler(w http.ResponseWriter, r *http.Request) {
	check, desc, ok := loadMonitorCheckForRequest(w, r, "UpdateMonitorCheckHandler")
	if !ok {
		return
	}
	var req MonitorCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.SiteID != nil || req.Type != nil {
		WriteError(w, http.StatusBadRequest, "site_id and type can't be changed")
		return
	}

	before := *check
	if err := req.applyTo(check); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	saveMonitorCheckEdit(w, r, check, probeChanged(before, *check), desc.Domain, "UpdateMonitorCheckHandler")
}

// ResetMonitorCheckHandler handles POST /api/monitor/checks/{id}/reset: it
// puts a site's default check back to path /, expecting HTML, with no
// assertion and the global cache bypass setting.
func ResetMonitorCheckHandler(w http.ResponseWriter, r *http.Request) {
	check, desc, ok := loadMonitorCheckForRequest(w, r, "ResetMonitorCheckHandler")
	if !ok {
		return
	}
	if !check.IsDefault {
		WriteError(w, http.StatusConflict, "Only a site's default check can be reset")
		return
	}

	before := *check
	check.Path = "/"
	check.Expect = models.MonitorExpectHTML
	check.AssertPath, check.AssertOp, check.AssertValue = "", "", nil
	check.CacheBypass = nil
	saveMonitorCheckEdit(w, r, check, probeChanged(before, *check), desc.Domain, "ResetMonitorCheckHandler")
}

// DeleteMonitorCheckHandler handles DELETE /api/monitor/checks/{id}. A
// site's default check can only be deleted once orphaned: while the site
// exists, the reconcile would just create it again.
func DeleteMonitorCheckHandler(w http.ResponseWriter, r *http.Request) {
	check, desc, ok := loadMonitorCheckForRequest(w, r, "DeleteMonitorCheckHandler")
	if !ok {
		return
	}
	if check.IsDefault && check.OrphanedAt == nil {
		WriteError(w, http.StatusConflict, "A site's default check can't be deleted while the site exists; pause it instead")
		return
	}

	username := requestUsername(r)
	if err := monitor.CloseOutCheck(check.ID, monitor.CheckLabel(*check, desc.Domain), monitor.StopDeleted, username, ""); err != nil {
		verb.LogPrintf(verb.Normal, "DeleteMonitorCheckHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if err := db.DeleteMonitorCheck(check.ID); err != nil {
		verb.LogPrintf(verb.Normal, "DeleteMonitorCheckHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// MonitorCheckSecretRequest is the body for setting a secret header.
type MonitorCheckSecretRequest struct {
	Value string `json:"value"`
}

// SetMonitorCheckSecretHandler handles PUT /api/monitor/checks/{id}/secrets/{header}.
// The value is stored and sent with the check's requests, never returned.
func SetMonitorCheckSecretHandler(w http.ResponseWriter, r *http.Request) {
	check, desc, ok := loadMonitorCheckForRequest(w, r, "SetMonitorCheckSecretHandler")
	if !ok {
		return
	}
	var req MonitorCheckSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	name, err := normalizeSecretHeader(r.PathValue("header"), req.Value)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	username := requestUsername(r)
	if err := db.SetMonitorCheckSecret(check.ID, name, req.Value, username); err != nil {
		verb.LogPrintf(verb.Normal, "SetMonitorCheckSecretHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	closeOutEditedCheck(check, desc.Domain, username)
	writeMonitorCheck(w, http.StatusOK, check, "SetMonitorCheckSecretHandler")
}

// DeleteMonitorCheckSecretHandler handles DELETE /api/monitor/checks/{id}/secrets/{header}.
func DeleteMonitorCheckSecretHandler(w http.ResponseWriter, r *http.Request) {
	check, desc, ok := loadMonitorCheckForRequest(w, r, "DeleteMonitorCheckSecretHandler")
	if !ok {
		return
	}
	found, err := db.DeleteMonitorCheckSecret(check.ID, http.CanonicalHeaderKey(r.PathValue("header")))
	if err != nil {
		verb.LogPrintf(verb.Normal, "DeleteMonitorCheckSecretHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if !found {
		WriteError(w, http.StatusNotFound, "Secret header not found")
		return
	}
	closeOutEditedCheck(check, desc.Domain, requestUsername(r))
	writeMonitorCheck(w, http.StatusOK, check, "DeleteMonitorCheckSecretHandler")
}

// MonitorCheckTestResponse is the result of running a check once.
type MonitorCheckTestResponse struct {
	Up         bool   `json:"up"`
	Message    string `json:"message,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
	URL        string `json:"url"`
	DurationMS int64  `json:"duration_ms"`
}

// TestMonitorCheckHandler handles POST /api/monitor/checks/test: it runs a
// check's settings once and reports the result, recording nothing. With
// "id", the stored check is tested with the request's fields applied on
// top, sending its secret headers; secret_headers adds or overrides them.
func TestMonitorCheckHandler(w http.ResponseWriter, r *http.Request) {
	var req MonitorCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	check := &models.MonitorCheck{Type: models.MonitorCheckTypeHTTP}
	headers := map[string]string{}
	if req.ID != nil && *req.ID != "" {
		stored, err := db.GetMonitorCheck(*req.ID)
		if err != nil {
			verb.LogPrintf(verb.Normal, "TestMonitorCheckHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		if stored == nil {
			WriteError(w, http.StatusNotFound, "Monitor check not found")
			return
		}
		check = stored
		values, err := db.GetMonitorCheckSecretValues()
		if err != nil {
			verb.LogPrintf(verb.Normal, "TestMonitorCheckHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		for name, value := range values[check.ID] {
			headers[name] = value
		}
	} else if req.SiteID != nil && strings.TrimSpace(*req.SiteID) != "" {
		siteID, ok := resolveCheckSite(w, *req.SiteID)
		if !ok {
			return
		}
		check.SiteID = siteID
	}
	if err := req.applyTo(check); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if check.Type != models.MonitorCheckTypeHTTP {
		WriteError(w, http.StatusBadRequest, "Only HTTP checks can be tested")
		return
	}
	for name, value := range req.SecretHeaders {
		canonical, err := normalizeSecretHeader(name, value)
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		headers[canonical] = value
	}

	host := check.Host
	if check.IsDefault {
		domains, err := siteDomains()
		if err != nil {
			verb.LogPrintf(verb.Normal, "TestMonitorCheckHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		host = domains[check.SiteID]
		if host == "" {
			WriteError(w, http.StatusConflict, "The check's site is gone")
			return
		}
	}

	probe := monitor.NewHTTPProbe(*check, host, headers)
	started := time.Now()
	result := monitor.ProbeOnce(probe)
	WriteJSON(w, http.StatusOK, MonitorCheckTestResponse{
		Up:         result.Up,
		Message:    result.Message,
		StatusCode: result.Code,
		URL:        probe.URL,
		DurationMS: time.Since(started).Milliseconds(),
	})
}

// IssueMonitorCheckTokenHandler handles POST /api/monitor/checks/{id}/token:
// it issues a new check-in token for a heartbeat check, replacing the old
// one, and returns it this once.
func IssueMonitorCheckTokenHandler(w http.ResponseWriter, r *http.Request) {
	check, _, ok := loadMonitorCheckForRequest(w, r, "IssueMonitorCheckTokenHandler")
	if !ok {
		return
	}
	if check.Type != models.MonitorCheckTypeHeartbeat {
		WriteError(w, http.StatusBadRequest, "Only heartbeat checks have check-in tokens")
		return
	}
	writeIssuedToken(w, http.StatusOK, check, requestUsername(r), "IssueMonitorCheckTokenHandler")
}

// writeIssuedToken issues a check-in token for check and writes the check
// with the token included.
func writeIssuedToken(w http.ResponseWriter, status int, check *models.MonitorCheck, username, handler string) {
	token, err := db.IssueMonitorCheckToken(check.ID, username)
	if err != nil {
		verb.LogPrintf(verb.Normal, "%s: %v", handler, err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	desc, err := describeStoredMonitorCheck(*check)
	if err != nil {
		verb.LogPrintf(verb.Normal, "%s: %v", handler, err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	desc.Token = token
	WriteJSON(w, status, desc)
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

	if err := pauseMonitorCheck(check, monitor.CheckLabel(*check, desc.Domain), username, req.Reason); err != nil {
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

// saveMonitorCheckEdit stores an edited check and, when what it probes
// changed, closes out its outage.
func saveMonitorCheckEdit(w http.ResponseWriter, r *http.Request, check *models.MonitorCheck, changed bool, domain, handler string) {
	username := requestUsername(r)
	if err := db.UpdateMonitorCheck(check, username); err != nil {
		verb.LogPrintf(verb.Normal, "%s: %v", handler, err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if changed {
		closeOutEditedCheck(check, domain, username)
	}
	writeMonitorCheck(w, http.StatusOK, check, handler)
}

// closeOutEditedCheck closes out the outage of a check whose probe
// settings changed. Failing to is logged, not returned: the edit is saved.
func closeOutEditedCheck(check *models.MonitorCheck, domain, username string) {
	host := check.Host
	if check.IsDefault {
		host = domain
	}
	if err := monitor.CloseOutCheck(check.ID, monitor.CheckLabel(*check, host), monitor.StopEdited, username, ""); err != nil {
		verb.LogPrintf(verb.Normal, "Failed to close out edited check %s: %v", check.ID, err)
	}
}

// probeChanged reports whether an edit changed what a check probes.
func probeChanged(a, b models.MonitorCheck) bool {
	return a.Host != b.Host || a.Path != b.Path || a.Expect != b.Expect ||
		a.AssertPath != b.AssertPath || a.AssertOp != b.AssertOp || string(a.AssertValue) != string(b.AssertValue) ||
		!sameBoolPtr(a.CacheBypass, b.CacheBypass) ||
		a.IntervalSeconds != b.IntervalSeconds || a.GraceSeconds != b.GraceSeconds || a.FailThreshold != b.FailThreshold
}

func sameBoolPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// resolveCheckSite resolves a site ID given for a check, writing the error
// response and returning false when the site doesn't exist.
func resolveCheckSite(w http.ResponseWriter, raw string) (string, bool) {
	siteID, err := resolveSiteUUID(raw)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	site, err := db.GetManagedSite(siteID)
	if err != nil {
		verb.LogPrintf(verb.Normal, "resolveCheckSite: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return "", false
	}
	if site == nil {
		WriteError(w, http.StatusBadRequest, "Site not found")
		return "", false
	}
	return siteID, true
}

// writeMonitorCheck writes a check as a MonitorCheckResponse.
func writeMonitorCheck(w http.ResponseWriter, status int, check *models.MonitorCheck, handler string) {
	desc, err := describeStoredMonitorCheck(*check)
	if err != nil {
		verb.LogPrintf(verb.Normal, "%s: %v", handler, err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, status, desc)
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

	desc, err := describeStoredMonitorCheck(*check)
	if err != nil {
		verb.LogPrintf(verb.Normal, "%s: %v", handler, err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return nil, MonitorCheckResponse{}, false
	}
	return check, desc, true
}

// describeStoredMonitorCheck describes one check, loading its site's
// domain and its secret header names.
func describeStoredMonitorCheck(c models.MonitorCheck) (MonitorCheckResponse, error) {
	domains := map[string]string{}
	if c.IsDefault {
		site, err := db.GetManagedSite(c.SiteID)
		if err != nil {
			return MonitorCheckResponse{}, err
		}
		if site != nil {
			domains[c.SiteID] = site.Domain
		}
	}
	secrets, err := db.ListMonitorCheckSecrets(c.ID)
	if err != nil {
		return MonitorCheckResponse{}, err
	}
	tokens, err := db.ListMonitorCheckTokens()
	if err != nil {
		return MonitorCheckResponse{}, err
	}
	return describeMonitorCheck(c, domains, secrets, tokens[c.ID]), nil
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

func describeMonitorCheck(c models.MonitorCheck, siteDomains map[string]string, secrets []models.MonitorCheckSecret, token db.MonitorCheckToken) MonitorCheckResponse {
	host := c.Host
	if c.IsDefault {
		host = siteDomains[c.SiteID]
	}
	if secrets == nil {
		secrets = []models.MonitorCheckSecret{}
	}
	resp := MonitorCheckResponse{MonitorCheck: c, Domain: host, SecretHeaders: secrets}
	if host != "" {
		resp.URL = "https://" + host + c.Path
	}
	if token.Prefix != "" {
		resp.TokenPrefix = token.Prefix
		createdAt := token.CreatedAt
		resp.TokenCreatedAt = &createdAt
	}
	return resp
}

func requestUsername(r *http.Request) string {
	if claims := GetAuthClaims(r.Context()); claims != nil && claims.Username != "" {
		return claims.Username
	}
	return "User"
}
