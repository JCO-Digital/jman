package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func TestPauseAndResumeMonitorCheck(t *testing.T) {
	setupIncidentTest(t)
	ctx := contextWithClaims(context.Background(), &AuthClaims{Username: "alice", Level: config.LevelEdit})

	check := models.MonitorCheck{Host: "api.example", Path: "/healthz", Enabled: true}
	if err := db.CreateMonitorCheck(&check, "test"); err != nil {
		t.Fatal(err)
	}
	inc, err := db.CreateIncident(check.ID, "api.example", "HTTP 503", 503, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/api/monitor/checks/"+check.ID+"/pause", strings.NewReader(`{"reason":"maintenance"}`)).WithContext(ctx)
	req.SetPathValue("id", check.ID)
	w := httptest.NewRecorder()
	PauseMonitorCheckHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pause: %d %s", w.Code, w.Body.String())
	}
	var paused MonitorCheckResponse
	if err := json.Unmarshal(w.Body.Bytes(), &paused); err != nil {
		t.Fatal(err)
	}
	if paused.Enabled || paused.URL != "https://api.example/healthz" {
		t.Errorf("pause response = %+v", paused)
	}
	if got, _ := db.GetIncidentByID(inc.ID); got == nil || got.Status != models.IncidentStatusResolved || got.ResolvedBy == nil || *got.ResolvedBy != "alice" {
		t.Errorf("incident after pause = %+v, want resolved by alice", got)
	}

	req = httptest.NewRequest("POST", "/api/monitor/checks/"+check.ID+"/resume", nil).WithContext(ctx)
	req.SetPathValue("id", check.ID)
	w = httptest.NewRecorder()
	ResumeMonitorCheckHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("resume: %d %s", w.Code, w.Body.String())
	}
	if got, _ := db.GetMonitorCheck(check.ID); got == nil || !got.Enabled {
		t.Errorf("check after resume = %+v, want enabled", got)
	}
}

func TestPauseIncidentCheck(t *testing.T) {
	setupIncidentTest(t)
	ctx := contextWithClaims(context.Background(), &AuthClaims{Username: "alice", Level: config.LevelEdit})

	check := models.MonitorCheck{Host: "shop.example", Enabled: true}
	if err := db.CreateMonitorCheck(&check, "test"); err != nil {
		t.Fatal(err)
	}
	inc, err := db.CreateIncident(check.ID, "shop.example", "HTTP 500", 500, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	url := fmt.Sprintf("/api/incidents/%d/pause", inc.ID)
	req := httptest.NewRequest("POST", url, nil).WithContext(ctx)
	req.SetPathValue("id", fmt.Sprintf("%d", inc.ID))
	w := httptest.NewRecorder()
	PauseIncidentCheckHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("pause from incident: %d %s", w.Code, w.Body.String())
	}
	var got models.Incident
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != models.IncidentStatusResolved {
		t.Errorf("incident status = %s, want resolved", got.Status)
	}
	if c, _ := db.GetMonitorCheck(check.ID); c == nil || c.Enabled {
		t.Errorf("check after pause = %+v, want paused", c)
	}
}

// doCheckRequest runs a monitor check handler as an edit-level user.
func doCheckRequest(t *testing.T, h http.HandlerFunc, method, url, body string, pathValues map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	ctx := contextWithClaims(context.Background(), &AuthClaims{Username: "alice", Level: config.LevelEdit})
	req := httptest.NewRequest(method, url, strings.NewReader(body)).WithContext(ctx)
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func TestCreateMonitorCheckValidation(t *testing.T) {
	setupIncidentTest(t)

	bad := []string{
		`{"path":"/healthz"}`,                       // no host
		`{"host":"https://mcp.example","path":"/"}`, // scheme in host
		`{"host":"mcp.example/healthz"}`,            // path in host
		`{"host":"mcp.example","path":"healthz"}`,   // path without /
		`{"host":"mcp.example","expect":"xml"}`,     // unknown expectation
		`{"host":"mcp.example","assert_path":"ok","assert_op":"equals","assert_value":true}`, // assertion on HTML
		`{"host":"mcp.example","expect":"json","assert_path":"ok","assert_op":"equals"}`,     // equals without value
		`{"host":"mcp.example","expect":"json","assert_path":"ok","assert_op":"exists","assert_value":1}`,
		`{"host":"mcp.example","expect":"json","assert_path":"ok..x","assert_op":"exists"}`,
		`{"host":"mcp.example","expect":"json","assert_path":"ok","assert_op":"matches","assert_value":1}`,
	}
	for _, body := range bad {
		if w := doCheckRequest(t, CreateMonitorCheckHandler, "POST", "/api/monitor/checks", body, nil); w.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d %s, want 400", body, w.Code, w.Body.String())
		}
	}

	w := doCheckRequest(t, CreateMonitorCheckHandler, "POST", "/api/monitor/checks",
		`{"name":"MCP","host":"MCP.example","path":"/healthz","expect":"json","assert_path":"ok","assert_op":"equals","assert_value": true ,"cache_bypass":false}`, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var got MonitorCheckResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Host != "mcp.example" || got.URL != "https://mcp.example/healthz" || got.IsDefault || !got.Enabled ||
		string(got.AssertValue) != "true" || got.CacheBypass == nil || *got.CacheBypass {
		t.Errorf("created check = %+v", got)
	}
}

func TestUpdateMonitorCheckClosesOutOnProbeChange(t *testing.T) {
	setupIncidentTest(t)

	check := models.MonitorCheck{Host: "mcp.example", Path: "/healthz", Enabled: true}
	if err := db.CreateMonitorCheck(&check, "test"); err != nil {
		t.Fatal(err)
	}
	inc, err := db.CreateIncident(check.ID, "mcp.example", "HTTP 503", 503, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	path := map[string]string{"id": check.ID}

	// Renaming changes nothing probed: the incident stays open.
	if w := doCheckRequest(t, UpdateMonitorCheckHandler, "PATCH", "/", `{"name":"MCP"}`, path); w.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", w.Code, w.Body.String())
	}
	if got, _ := db.GetIncidentByID(inc.ID); got.Status != models.IncidentStatusOpen {
		t.Errorf("rename closed the incident: %s", got.Status)
	}

	// cache_bypass null keeps following the global setting; true overrides.
	if w := doCheckRequest(t, UpdateMonitorCheckHandler, "PATCH", "/", `{"cache_bypass":null}`, path); w.Code != http.StatusOK {
		t.Fatalf("null cache_bypass: %d %s", w.Code, w.Body.String())
	}
	if got, _ := db.GetIncidentByID(inc.ID); got.Status != models.IncidentStatusOpen {
		t.Errorf("unchanged cache_bypass closed the incident")
	}

	w := doCheckRequest(t, UpdateMonitorCheckHandler, "PATCH", "/", `{"expect":"json","assert_path":"ok","assert_op":"exists"}`, path)
	if w.Code != http.StatusOK {
		t.Fatalf("edit: %d %s", w.Code, w.Body.String())
	}
	if got, _ := db.GetIncidentByID(inc.ID); got.Status != models.IncidentStatusResolved {
		t.Errorf("probe edit left the incident %s, want resolved", got.Status)
	}
	stored, _ := db.GetMonitorCheck(check.ID)
	if stored.Name != "MCP" || stored.Expect != "json" || stored.AssertOp != "exists" || stored.Path != "/healthz" {
		t.Errorf("stored check = %+v", stored)
	}
}

func TestMonitorCheckSecretsAreWriteOnly(t *testing.T) {
	setupIncidentTest(t)

	check := models.MonitorCheck{Host: "api.example", Enabled: true}
	if err := db.CreateMonitorCheck(&check, "test"); err != nil {
		t.Fatal(err)
	}
	path := map[string]string{"id": check.ID, "header": "x-api-key"}

	if w := doCheckRequest(t, SetMonitorCheckSecretHandler, "PUT", "/", `{"value":"line\nbreak"}`, path); w.Code != http.StatusBadRequest {
		t.Errorf("multi-line value: %d, want 400", w.Code)
	}
	if w := doCheckRequest(t, SetMonitorCheckSecretHandler, "PUT", "/", `{"value":"s3cret"}`, map[string]string{"id": check.ID, "header": "Host"}); w.Code != http.StatusBadRequest {
		t.Errorf("Host header: %d, want 400", w.Code)
	}

	w := doCheckRequest(t, SetMonitorCheckSecretHandler, "PUT", "/", `{"value":"s3cret"}`, path)
	if w.Code != http.StatusOK {
		t.Fatalf("set secret: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "s3cret") {
		t.Errorf("secret value returned: %s", w.Body.String())
	}
	var got MonitorCheckResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.SecretHeaders) != 1 || got.SecretHeaders[0].Name != "X-Api-Key" || got.SecretHeaders[0].UpdatedBy != "alice" {
		t.Errorf("secret headers = %+v", got.SecretHeaders)
	}

	list := doCheckRequest(t, ListMonitorChecksHandler, "GET", "/api/monitor/checks", "", nil)
	if strings.Contains(list.Body.String(), "s3cret") || !strings.Contains(list.Body.String(), "X-Api-Key") {
		t.Errorf("list leaks or lacks the secret: %s", list.Body.String())
	}

	if w := doCheckRequest(t, DeleteMonitorCheckSecretHandler, "DELETE", "/", "", path); w.Code != http.StatusOK {
		t.Fatalf("delete secret: %d %s", w.Code, w.Body.String())
	}
	if w := doCheckRequest(t, DeleteMonitorCheckSecretHandler, "DELETE", "/", "", path); w.Code != http.StatusNotFound {
		t.Errorf("deleting a missing secret: %d, want 404", w.Code)
	}
}

func TestDeleteMonitorCheck(t *testing.T) {
	setupIncidentTest(t)

	def := models.MonitorCheck{SiteID: "site-1", IsDefault: true, Enabled: true}
	own := models.MonitorCheck{Host: "api.example", Enabled: true}
	for _, c := range []*models.MonitorCheck{&def, &own} {
		if err := db.CreateMonitorCheck(c, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.CreateIncident(own.ID, "api.example", "HTTP 500", 500, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.SetMonitorCheckSecret(own.ID, "X-Api-Key", "s3cret", "test"); err != nil {
		t.Fatal(err)
	}

	if w := doCheckRequest(t, DeleteMonitorCheckHandler, "DELETE", "/", "", map[string]string{"id": def.ID}); w.Code != http.StatusConflict {
		t.Errorf("deleting a live default check: %d, want 409", w.Code)
	}
	if w := doCheckRequest(t, ResetMonitorCheckHandler, "POST", "/", "", map[string]string{"id": own.ID}); w.Code != http.StatusConflict {
		t.Errorf("resetting a non-default check: %d, want 409", w.Code)
	}

	if w := doCheckRequest(t, DeleteMonitorCheckHandler, "DELETE", "/", "", map[string]string{"id": own.ID}); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if c, _ := db.GetMonitorCheck(own.ID); c != nil {
		t.Errorf("check still exists")
	}
	if inc, _ := db.GetActiveIncidentByCheck(own.ID); inc != nil {
		t.Errorf("incident survived the delete")
	}
	if secrets, _ := db.ListMonitorCheckSecrets(own.ID); len(secrets) != 0 {
		t.Errorf("secrets survived the delete: %+v", secrets)
	}
}
