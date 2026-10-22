package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func TestCreateMonitorCheckRulePausesExisting(t *testing.T) {
	setupIncidentTest(t)
	ctx := contextWithClaims(context.Background(), &AuthClaims{Username: "alice", Level: config.LevelEdit})

	serverID := "0190c3a0-0000-7000-8000-000000000001"
	if err := db.SaveManagedServer(models.ManagedServer{ID: serverID, Name: "staging", Provider: "manual", SSHPort: 22}); err != nil {
		t.Fatal(err)
	}
	checkIDs := map[string]string{}
	for i, domain := range []string{"a.staging.example", "b.example"} {
		siteID := []string{"0190c3a0-0000-7000-8000-00000000000a", "0190c3a0-0000-7000-8000-00000000000b"}[i]
		if err := db.SaveManagedSite(models.ManagedSite{
			ID: siteID, ServerID: &serverID, Provider: "manual", Domain: domain,
			Environment: models.SiteEnvironmentProduction, Status: "active", ConnectionType: "none",
		}); err != nil {
			t.Fatal(err)
		}
		check := models.MonitorCheck{SiteID: siteID, IsDefault: true, Enabled: true}
		if err := db.CreateMonitorCheck(&check, "test"); err != nil {
			t.Fatal(err)
		}
		checkIDs[domain] = check.ID
	}

	// Preview a host pattern: only the staging site matches.
	req := httptest.NewRequest("POST", "/api/monitor/rules/preview", strings.NewReader(`{"match_type":"host","value":"*.STAGING.*"}`)).WithContext(ctx)
	w := httptest.NewRecorder()
	PreviewMonitorCheckRuleHandler(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "a.staging.example") || strings.Contains(w.Body.String(), "b.example") {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}

	// A server rule that pauses what is already running on the server.
	req = httptest.NewRequest("POST", "/api/monitor/rules", strings.NewReader(`{"match_type":"server","value":"`+serverID+`","pause_existing":true}`)).WithContext(ctx)
	w = httptest.NewRecorder()
	CreateMonitorCheckRuleHandler(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created MonitorCheckRuleResponse
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.PausedChecks != 2 || created.ServerName != "staging" || created.Action != models.MonitorRuleActionStartPaused {
		t.Errorf("create response = %+v", created)
	}
	for domain, id := range checkIDs {
		if c, _ := db.GetMonitorCheck(id); c.Enabled || c.PausedByRuleID != created.ID {
			t.Errorf("%s check = enabled %v, paused by %q", domain, c.Enabled, c.PausedByRuleID)
		}
	}

	req = httptest.NewRequest("GET", "/api/monitor/rules", nil).WithContext(ctx)
	w = httptest.NewRecorder()
	ListMonitorCheckRulesHandler(w, req)
	var rules []MonitorCheckRuleResponse
	if err := json.Unmarshal(w.Body.Bytes(), &rules); err != nil || len(rules) != 1 || rules[0].PausedChecks != 2 {
		t.Fatalf("list: %v %s", err, w.Body.String())
	}

	req = httptest.NewRequest("POST", "/api/monitor/rules", strings.NewReader(`{"match_type":"host","value":"[bad"}`)).WithContext(ctx)
	w = httptest.NewRecorder()
	CreateMonitorCheckRuleHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid pattern: %d %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("DELETE", "/api/monitor/rules/"+created.ID, nil).WithContext(ctx)
	req.SetPathValue("id", created.ID)
	w = httptest.NewRecorder()
	DeleteMonitorCheckRuleHandler(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if c, _ := db.GetMonitorCheck(checkIDs["b.example"]); c.Enabled || c.PausedByRuleID != "" {
		t.Errorf("check after rule deleted = enabled %v, paused by %q", c.Enabled, c.PausedByRuleID)
	}
}
