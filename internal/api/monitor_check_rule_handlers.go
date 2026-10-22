package api

import (
	"encoding/json"
	"net/http"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/monitor"
	"github.com/JCO-Digital/jman/internal/verb"
)

// MonitorCheckRuleResponse is a rule with what it applies to: the server's
// name for a server rule, and how many checks it holds paused.
type MonitorCheckRuleResponse struct {
	models.MonitorCheckRule
	ServerName   string `json:"server_name,omitempty"`
	PausedChecks int    `json:"paused_checks"`
}

// MonitorCheckRuleRequest is the payload for creating a rule, or for
// previewing what it would match. PauseExisting also pauses the running
// default checks it matches.
type MonitorCheckRuleRequest struct {
	MatchType     string `json:"match_type"`
	Value         string `json:"value"`
	Note          string `json:"note"`
	PauseExisting bool   `json:"pause_existing"`
}

// ListMonitorCheckRulesHandler handles GET /api/monitor/rules.
func ListMonitorCheckRulesHandler(w http.ResponseWriter, r *http.Request) {
	rules, err := db.ListMonitorCheckRules()
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListMonitorCheckRulesHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	checks, err := db.ListMonitorChecks()
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListMonitorCheckRulesHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	paused := map[string]int{}
	for _, c := range checks {
		if c.PausedByRuleID != "" {
			paused[c.PausedByRuleID]++
		}
	}
	serverNames, err := serverNames()
	if err != nil {
		verb.LogPrintf(verb.Normal, "ListMonitorCheckRulesHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	resp := make([]MonitorCheckRuleResponse, 0, len(rules))
	for _, rule := range rules {
		desc := MonitorCheckRuleResponse{MonitorCheckRule: rule, PausedChecks: paused[rule.ID]}
		if rule.MatchType == models.MonitorRuleMatchServer {
			desc.ServerName = serverNames[rule.Value]
		}
		resp = append(resp, desc)
	}
	WriteJSON(w, http.StatusOK, resp)
}

// PreviewMonitorCheckRuleHandler handles POST /api/monitor/rules/preview:
// the existing default checks a rule would match.
func PreviewMonitorCheckRuleHandler(w http.ResponseWriter, r *http.Request) {
	rule, ok := decodeMonitorCheckRule(w, r, nil)
	if !ok {
		return
	}
	matches, err := monitor.RuleMatches(rule)
	if err != nil {
		verb.LogPrintf(verb.Normal, "PreviewMonitorCheckRuleHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	WriteJSON(w, http.StatusOK, matches)
}

// CreateMonitorCheckRuleHandler handles POST /api/monitor/rules. With
// pause_existing, the running default checks the rule matches are paused
// too, closing out their outages, and count as paused by it.
func CreateMonitorCheckRuleHandler(w http.ResponseWriter, r *http.Request) {
	var req MonitorCheckRuleRequest
	rule, ok := decodeMonitorCheckRule(w, r, &req)
	if !ok {
		return
	}
	username := requestUsername(r)
	if err := db.CreateMonitorCheckRule(&rule, username); err != nil {
		verb.LogPrintf(verb.Normal, "CreateMonitorCheckRuleHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	desc := MonitorCheckRuleResponse{MonitorCheckRule: rule}
	if req.PauseExisting {
		matches, err := monitor.RuleMatches(rule)
		if err != nil {
			verb.LogPrintf(verb.Normal, "CreateMonitorCheckRuleHandler: %v", err)
			WriteError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		reason := "Paused by the monitoring rule " + rule.Value
		for _, m := range matches {
			if !m.Enabled {
				continue
			}
			if err := db.PauseMonitorCheckByRule(m.CheckID, rule.ID, username); err != nil {
				verb.LogPrintf(verb.Normal, "CreateMonitorCheckRuleHandler: %v", err)
				WriteError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
			if err := monitor.CloseOutCheck(m.CheckID, m.Domain, monitor.StopPaused, username, reason); err != nil {
				verb.LogPrintf(verb.Normal, "CreateMonitorCheckRuleHandler: %v", err)
			}
			desc.PausedChecks++
		}
	}
	if rule.MatchType == models.MonitorRuleMatchServer {
		if names, err := serverNames(); err == nil {
			desc.ServerName = names[rule.Value]
		}
	}
	WriteJSON(w, http.StatusCreated, desc)
}

// DeleteMonitorCheckRuleHandler handles DELETE /api/monitor/rules/{id}.
// Checks the rule paused stay paused.
func DeleteMonitorCheckRuleHandler(w http.ResponseWriter, r *http.Request) {
	found, err := db.DeleteMonitorCheckRule(r.PathValue("id"))
	if err != nil {
		verb.LogPrintf(verb.Normal, "DeleteMonitorCheckRuleHandler: %v", err)
		WriteError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if !found {
		WriteError(w, http.StatusNotFound, "Monitor rule not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeMonitorCheckRule reads and validates a rule from the request body,
// into req when it isn't nil. A server rule's value may be a server UUID
// or a legacy SpinupWP server ID.
func decodeMonitorCheckRule(w http.ResponseWriter, r *http.Request, req *MonitorCheckRuleRequest) (models.MonitorCheckRule, bool) {
	if req == nil {
		req = &MonitorCheckRuleRequest{}
	}
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return models.MonitorCheckRule{}, false
	}
	rule := models.MonitorCheckRule{MatchType: req.MatchType, Value: req.Value, Note: req.Note}
	if rule.MatchType == models.MonitorRuleMatchServer && rule.Value != "" {
		serverID, err := resolveServerUUID(rule.Value)
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return rule, false
		}
		rule.Value = serverID
	}
	if err := monitor.NormalizeRule(&rule); err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return rule, false
	}
	return rule, true
}

// serverNames maps server UUIDs to names.
func serverNames() (map[string]string, error) {
	servers, err := db.ListManagedServers()
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(servers))
	for _, s := range servers {
		names[s.ID] = s.Name
	}
	return names, nil
}
