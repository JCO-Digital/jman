package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/sitestate"
	"github.com/JCO-Digital/jman/internal/utils"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

func listIncidents(t *testing.T, query string) IncidentsResponse {
	t.Helper()
	w := httptest.NewRecorder()
	ListIncidentsHandler(w, httptest.NewRequest("GET", "/api/incidents"+query, nil))
	var resp IncidentsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("incidents: %v: %s", err, w.Body.String())
	}
	return resp
}

func securityIncidents(t *testing.T, filter string) []models.Incident {
	t.Helper()
	return listIncidents(t, "?type=security&filter="+filter).Incidents
}

func postJSON(t *testing.T, h http.HandlerFunc, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(method, target, bytes.NewReader(b)))
	return w
}

func closeIncident(t *testing.T, id int64) {
	t.Helper()
	req := httptest.NewRequest("POST", "/", nil)
	req.SetPathValue("id", fmt.Sprint(id))
	w := httptest.NewRecorder()
	CloseIncidentHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("close incident %d: %d %s", id, w.Code, w.Body.String())
	}
}

func findingByPath(t *testing.T, findings []models.StoredScanFinding, path string) models.StoredScanFinding {
	t.Helper()
	for _, f := range findings {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("no finding for %s in %+v", path, findings)
	return models.StoredScanFinding{}
}

func TestSecurityIncidentLifecycle(t *testing.T) {
	setupAgentTest(t)
	seedAgentSites(t,
		agentSite{501, 8, "one.example.com", "deployed", true},
		agentSite{502, 8, "two.example.com", "deployed", true},
	)
	one, two := utils.SpinupWPSiteUUID(501), utils.SpinupWPSiteUUID(502)
	report := func(site string, findings ...models.ScanFinding) {
		t.Helper()
		postAgentReport(t, 8, models.AgentReportSite{SiteID: site, Scan: &models.AgentScan{RuleSetVersion: "4", Findings: findings}})
	}
	shell := func(path, sha string) models.ScanFinding {
		return models.ScanFinding{Engine: "builtin", Rule: "uploads-php", Severity: "high", Path: path, SHA256: sha}
	}
	foreign := models.ScanFinding{Engine: "builtin", Rule: "foreign-owner", Severity: "medium", Path: "wp-content/x", SHA256: "f1"}
	stray := models.ScanFinding{Engine: "builtin", Rule: "stray-file", Severity: "low", Path: "dump.sql"}

	// Findings of medium and above open one incident per site; low ones
	// stay out of incidents.
	report(one, shell("wp-content/uploads/a.php", "aaa"), stray)
	report(one, foreign)
	active := securityIncidents(t, "active")
	if len(active) != 1 || active[0].Domain != "one.example.com" || active[0].Severity != "high" || active[0].ErrorMessage != "2 open findings: 1 uploads-php, 1 foreign-owner" {
		t.Fatalf("active security incidents = %+v", active)
	}
	first := active[0].ID
	if f := listScanFindings(t, "?unassigned=1&status=open"); len(f) != 1 || f[0].Rule != "stray-file" {
		t.Fatalf("unassigned findings = %+v", f)
	}
	if resp := listIncidents(t, ""); len(resp.Incidents) != 0 || resp.ActiveCount != 1 || resp.ActiveByType["security"] != 1 {
		t.Fatalf("uptime incidents = %+v", resp)
	}

	// Closing closes its findings, and nothing reopens it on its own.
	closeIncident(t, first)
	for _, f := range listScanFindings(t, fmt.Sprintf("?incident_id=%d", first)) {
		if f.Status != models.ScanFindingClosed {
			t.Fatalf("finding after closing its incident: %+v", f)
		}
	}

	// A new file opens a new incident, even at the same path.
	report(one, shell("wp-content/uploads/a.php", "bbb"))
	active = securityIncidents(t, "active")
	if len(active) != 1 || active[0].ID == first {
		t.Fatalf("active after a new file = %+v", active)
	}
	second := active[0].ID

	// Content seen in the closed incident reopens it, here at another path.
	report(one, shell("wp-content/uploads/b.php", "aaa"))
	inc, err := db.GetIncidentByID(first)
	if err != nil || inc.Status != models.IncidentStatusOpen || inc.ResolvedBy != nil {
		t.Fatalf("first incident after its hash came back = %+v, %v", inc, err)
	}
	if f := findingByPath(t, listScanFindings(t, ""), "wp-content/uploads/b.php"); f.IncidentID == nil || *f.IncidentID != first {
		t.Fatalf("returning hash went to %+v", f)
	}

	// Allowlisting with close_matching covers the hash on every site and
	// closes the incidents left without open findings.
	report(two, shell("wp-content/uploads/c.php", "aaa"))
	onTwo := securityIncidents(t, "active")
	if len(onTwo) != 3 {
		t.Fatalf("active incidents = %+v", onTwo)
	}
	target := findingByPath(t, listScanFindings(t, ""), "wp-content/uploads/b.php")
	w := postJSON(t, AddScanAllowlistHandler, "POST", "/api/scan/allowlist", map[string]any{"finding_id": target.ID, "reason": ""})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("allowlisting without a reason: %d", w.Code)
	}
	w = postJSON(t, AddScanAllowlistHandler, "POST", "/api/scan/allowlist", map[string]any{"finding_id": target.ID, "reason": "plugin's data file", "close_matching": true})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"allowlisted":2`) {
		t.Fatalf("allowlist: %d %s", w.Code, w.Body.String())
	}
	active = securityIncidents(t, "active")
	if len(active) != 1 || active[0].ID != second {
		t.Fatalf("active after allowlisting = %+v", active)
	}

	// Allowlisted content doesn't open an incident anywhere.
	report(two, shell("wp-content/uploads/d.php", "aaa"))
	if f := findingByPath(t, listScanFindings(t, ""), "wp-content/uploads/d.php"); f.Status != models.ScanFindingAllowlisted || f.IncidentID != nil {
		t.Fatalf("allowlisted hash found again = %+v", f)
	}

	// Removing the entry keeps it for the record and stops covering new
	// findings.
	entries, _ := db.ListScanAllowlist(false)
	if len(entries) != 1 || entries[0].Reason != "plugin's data file" || entries[0].CreatedBy != "User" || entries[0].Path != "wp-content/uploads/b.php" {
		t.Fatalf("allowlist = %+v", entries)
	}
	req := httptest.NewRequest("DELETE", "/", nil)
	req.SetPathValue("id", fmt.Sprint(entries[0].ID))
	rw := httptest.NewRecorder()
	RemoveScanAllowlistHandler(rw, req)
	if rw.Code != http.StatusNoContent {
		t.Fatalf("remove allowlist entry: %d", rw.Code)
	}
	if all, _ := db.ListScanAllowlist(true); len(all) != 1 || !all[0].Removed {
		t.Fatalf("allowlist with removed = %+v", all)
	}

	// A path acknowledgement covers a state rule's findings for that path
	// on that site, whatever the file holds.
	report(two, foreign)
	f := findingByPath(t, listScanFindings(t, "?site_id="+two), "wp-content/x")
	w = postJSON(t, AddScanPathAcksHandler, "POST", "/api/scan/acks", map[string]any{"finding_ids": []int64{f.ID}, "reason": "deploy user"})
	if w.Code != http.StatusOK {
		t.Fatalf("ack: %d %s", w.Code, w.Body.String())
	}
	report(two, models.ScanFinding{Engine: "builtin", Rule: "foreign-owner", Severity: "medium", Path: "wp-content/x", SHA256: "f2"})
	for _, f := range listScanFindings(t, "?site_id="+two+"&sha256=f2") {
		if f.Status != models.ScanFindingAcknowledged {
			t.Fatalf("acknowledged path found again = %+v", f)
		}
	}
	shellF := findingByPath(t, listScanFindings(t, "?site_id="+two), "wp-content/uploads/c.php")
	w = postJSON(t, AddScanPathAcksHandler, "POST", "/api/scan/acks", map[string]any{"finding_ids": []int64{shellF.ID}, "reason": "x"})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("acknowledging an uploads-php path: %d", w.Code)
	}

	// Dismissing the last open finding closes the incident.
	open := listScanFindings(t, fmt.Sprintf("?incident_id=%d&status=open", second))
	w = postJSON(t, CloseScanFindingsHandler, "POST", "/api/scan/findings/close", map[string]any{"finding_ids": findingIDs(open)})
	if w.Code != http.StatusOK {
		t.Fatalf("close findings: %d %s", w.Code, w.Body.String())
	}
	if active := securityIncidents(t, "active"); len(active) != 0 {
		t.Fatalf("active after dismissing = %+v", active)
	}
}

// Findings stored before security incidents existed get one on startup,
// grouped per site.
func TestTriagePendingScanFindings(t *testing.T) {
	setupAgentTest(t)
	seedAgentSites(t, agentSite{503, 8, "old.example.com", "deployed", true})
	site := utils.SpinupWPSiteUUID(503)
	seen := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	for _, f := range []models.ScanFinding{
		{Engine: "builtin", Rule: "uploads-php", Severity: "high", Path: "wp-content/uploads/a.php", SHA256: "a"},
		{Engine: "builtin", Rule: "foreign-owner", Severity: "medium", Path: "wp-content/b"},
		{Engine: "builtin", Rule: "stray-file", Severity: "low", Path: "c.sql"},
	} {
		if err := db.UpsertScanFinding(site, f, "3", seen); err != nil {
			t.Fatal(err)
		}
	}
	TriagePendingScanFindings()
	TriagePendingScanFindings()
	active := securityIncidents(t, "active")
	if len(active) != 1 || !active[0].DownSince.Equal(seen) || active[0].ErrorMessage != "2 open findings: 1 uploads-php, 1 foreign-owner" {
		t.Fatalf("active = %+v", active)
	}
}

func TestSecuritySlackMessage(t *testing.T) {
	inc := &models.Incident{ID: 7, Domain: "x.example.com"}
	var findings []models.StoredScanFinding
	for i := range 12 {
		f := models.StoredScanFinding{}
		f.Rule, f.Severity, f.Path = "uploads-php", "high", fmt.Sprintf("wp-content/uploads/%d.php", i)
		findings = append(findings, f)
	}
	msg := securitySlackMessage(inc, findings, false)
	if !strings.Contains(msg, "*x.example.com*") || !strings.Contains(msg, "`wp-content/uploads/9.php`") || strings.Contains(msg, "10.php") || !strings.Contains(msg, "… and 2 more") {
		t.Fatalf("message = %s", msg)
	}
}

// The agent reporting that flagged files are gone marks their findings
// cleared without closing anything; finding one again undoes that.
func TestScanFindingsCleared(t *testing.T) {
	setupAgentTest(t)
	seedAgentSites(t, agentSite{504, 8, "clean.example.com", "deployed", true})
	site := utils.SpinupWPSiteUUID(504)
	report := func(s models.AgentScan) {
		t.Helper()
		s.RuleSetVersion = "4"
		postAgentReport(t, 8, models.AgentReportSite{SiteID: site, Scan: &s})
	}
	shell := models.ScanFinding{Engine: "builtin", Rule: "uploads-php", Severity: "high", Path: "wp-content/uploads/a.php", SHA256: "aaa"}
	foreign := models.ScanFinding{Engine: "builtin", Rule: "foreign-owner", Severity: "medium", Path: "wp-content/b"}
	report(models.AgentScan{Findings: []models.ScanFinding{shell, foreign}})

	// Replaced content clears only the finding for the old content.
	report(models.AgentScan{Cleared: []models.ScanCleared{
		{Path: shell.Path, Rule: "uploads-php", SHA256: "other", Detail: "replaced"},
		{Path: shell.Path, Detail: "removed"},
	}})
	active := securityIncidents(t, "active")
	if len(active) != 1 || active[0].ErrorMessage != "2 open findings: 1 uploads-php, 1 foreign-owner (1 cleared)" {
		t.Fatalf("active = %+v", active)
	}
	if f := findingByPath(t, listScanFindings(t, ""), shell.Path); f.ClearedAt == nil || f.ClearedDetail != "removed" || f.Status != models.ScanFindingOpen {
		t.Fatalf("removed finding = %+v", f)
	}

	report(models.AgentScan{Cleared: []models.ScanCleared{{Path: foreign.Path, Rule: "foreign-owner", Detail: "now owned by the site's owner"}}})
	if active := securityIncidents(t, "active"); len(active) != 1 || !strings.HasSuffix(active[0].ErrorMessage, "(all cleared)") {
		t.Fatalf("active after clearing all = %+v", active)
	}

	// The same file back isn't cleared any more.
	report(models.AgentScan{Findings: []models.ScanFinding{shell}})
	if f := findingByPath(t, listScanFindings(t, ""), shell.Path); f.ClearedAt != nil || f.ClearedDetail != "" {
		t.Fatalf("finding found again = %+v", f)
	}
	if active := securityIncidents(t, "active"); len(active) != 1 || active[0].ErrorMessage != "2 open findings: 1 uploads-php, 1 foreign-owner (1 cleared)" {
		t.Fatalf("active after the file came back = %+v", active)
	}
}

// Administrators that appear without jman creating them raise a finding;
// the ones jman created, and a site's first users, don't.
func TestNewAdminFindings(t *testing.T) {
	setupAgentTest(t)
	seedAgentSites(t, agentSite{505, 8, "admins.example.com", "deployed", true})
	site := utils.SpinupWPSiteUUID(505)
	apply := func(users ...models.SiteUser) {
		t.Helper()
		if _, err := sitestate.ApplyUsers(site, wpcli.NewSiteUsers(users, nil), sitestate.SourceAgent, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	owner := models.SiteUser{ID: 1, Login: "owner", Email: "o@example.com", Roles: "administrator"}
	editor := models.SiteUser{ID: 2, Login: "editor", Email: "e@example.com", Roles: "editor"}
	apply(owner, editor)
	if f := listScanFindings(t, ""); len(f) != 0 {
		t.Fatalf("findings for the first users: %+v", f)
	}

	// jman created this one.
	if err := db.SaveSiteUpdateLedgerEntry(&models.SiteUpdateLedgerEntry{SiteID: site, UpdateType: "user", Status: models.LedgerCreated,
		DataJSON: `{"action":"user_create","login":"JCO","email":"jco@example.com","role":"administrator"}`, UpdatedBy: "niklas", UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	jco := models.SiteUser{ID: 3, Login: "jco", Email: "jco@example.com", Roles: "administrator"}
	staff := models.SiteUser{ID: 5, Login: "anna", Email: "Anna@JCO.fi", Roles: "administrator"}
	config.Cfg.TrustedAdminDomains = []string{"jco.fi"}
	t.Cleanup(func() { config.Cfg.TrustedAdminDomains = nil })
	hidden := models.SiteUser{ID: 4, Login: "wpsvc_1d4e", Email: "x@example.net", Roles: "administrator", Registered: "2026-04-07 00:48:40"}
	promoted := editor
	promoted.Roles = "editor,administrator"
	apply(owner, promoted, jco, hidden, staff)

	findings := listScanFindings(t, "")
	var got []string
	for _, f := range findings {
		got = append(got, f.Rule+" "+f.Path+" "+f.Severity)
	}
	slices.Sort(got)
	want := []string{"new-admin users/anna medium", "new-admin users/editor high", "new-admin users/wpsvc_1d4e high"}
	if !slices.Equal(got, want) {
		t.Fatalf("findings = %q, want %q", got, want)
	}
	if active := securityIncidents(t, "active"); len(active) != 1 || active[0].Severity != "high" {
		t.Fatalf("incidents = %+v", active)
	}

	// Acknowledging an account keeps it from being reported again.
	f := findingByPath(t, findings, "users/editor")
	if w := postJSON(t, AddScanPathAcksHandler, "POST", "/api/scan/acks", map[string]any{"finding_ids": []int64{f.ID}, "reason": "the client's own admin"}); w.Code != http.StatusOK {
		t.Fatalf("ack: %d %s", w.Code, w.Body.String())
	}
	apply(owner, editor, jco, hidden, staff)
	apply(owner, promoted, jco, hidden, staff)
	if f := findingByPath(t, listScanFindings(t, ""), "users/editor"); f.Status != models.ScanFindingAcknowledged {
		t.Fatalf("acknowledged admin = %+v", f)
	}
}

// Application passwords that appear raise a finding, high on an
// administrator; the first ones collected are a baseline.
func TestNewAppPasswordFindings(t *testing.T) {
	setupAgentTest(t)
	seedAgentSites(t, agentSite{506, 8, "apps.example.com", "deployed", true})
	site := utils.SpinupWPSiteUUID(506)
	apply := func(collected bool, at time.Time, users ...models.SiteUser) {
		t.Helper()
		su := wpcli.NewSiteUsers(users, nil)
		su.AppPasswordsCollected = collected
		if _, err := sitestate.ApplyUsers(site, su, sitestate.SourceAgent, at); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	admin := models.SiteUser{ID: 1, Login: "admin", Email: "a@example.com", Roles: "administrator"}
	editor := models.SiteUser{ID: 2, Login: "ed", Email: "e@example.com", Roles: "editor"}
	integration := models.SiteAppPassword{UUID: "11111111-0000-0000-0000-000000000000", Name: "Zapier", Created: now.Add(-48 * time.Hour).Unix()}

	// Users from an agent that doesn't read application passwords, then
	// the first ones read: both baselines.
	apply(false, now.Add(-3*time.Hour), admin, editor)
	withIntegration := admin
	withIntegration.AppPasswords = []models.SiteAppPassword{integration}
	apply(true, now.Add(-2*time.Hour), withIntegration, editor)
	if f := listScanFindings(t, ""); len(f) != 0 {
		t.Fatalf("findings for the baseline: %+v", f)
	}

	// One with a faked creation date on the administrator, one on the editor.
	forged := models.SiteAppPassword{UUID: "22222222-0000-0000-0000-000000000000", Name: "wordpress_1778988921", Created: 1711152199}
	edPassword := models.SiteAppPassword{UUID: "33333333-0000-0000-0000-000000000000", Name: "phone", Created: now.Unix()}
	adminNow := admin
	adminNow.AppPasswords = []models.SiteAppPassword{integration, forged}
	editorNow := editor
	editorNow.AppPasswords = []models.SiteAppPassword{edPassword}
	apply(true, now.Add(-time.Hour), adminNow, editorNow)

	findings := listScanFindings(t, "")
	var got []string
	for _, f := range findings {
		got = append(got, f.Rule+" "+f.Path+" "+f.Severity)
	}
	slices.Sort(got)
	want := []string{
		"new-app-password users/admin/app-passwords/22222222-0000-0000-0000-000000000000 high",
		"new-app-password users/ed/app-passwords/33333333-0000-0000-0000-000000000000 medium",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("findings = %q, want %q", got, want)
	}
	if f := findingByPath(t, findings, want[0][len("new-app-password "):len(want[0])-len(" high")]); !strings.Contains(f.Detail, "faked") || !strings.Contains(f.Snippet, "wordpress_1778988921") {
		t.Fatalf("forged password finding = %+v", f)
	}

	// Using a password isn't a new one.
	used := now.Unix()
	adminNow.AppPasswords[0].LastUsed = &used
	apply(true, now, adminNow, editorNow)
	if f := listScanFindings(t, ""); len(f) != 2 {
		t.Fatalf("findings after a password was used: %d", len(f))
	}
}

// Snippets can be removed by hand, and expire once a finding has been
// triaged for the retention period; the finding itself stays.
func TestScanSnippetRetention(t *testing.T) {
	setupAgentTest(t)
	seedAgentSites(t, agentSite{507, 8, "logs.example.com", "deployed", true})
	site := utils.SpinupWPSiteUUID(507)
	long := time.Now().Add(-200 * 24 * time.Hour)
	add := func(path string, seen time.Time) models.StoredScanFinding {
		t.Helper()
		f := models.ScanFinding{Engine: "builtin", Rule: "stray-file", Severity: "low", Path: path, SHA256: path, Snippet: "user=admin ip=203.0.113.9", Detail: "failed login"}
		if err := db.UpsertScanFinding(site, f, "9", seen); err != nil {
			t.Fatal(err)
		}
		return findingByPath(t, listScanFindings(t, "?site_id="+site), path)
	}
	add("open.log", long)
	closed := add("closed.log", long)
	byHand := add("by-hand.log", time.Now())
	if _, err := db.SetScanFindingStatus([]int64{closed.ID}, models.ScanFindingClosed, "niklas"); err != nil {
		t.Fatal(err)
	}
	redacted := func() map[string]bool {
		got := map[string]bool{}
		for _, f := range listScanFindings(t, "?site_id="+site) {
			got[f.Path] = f.RedactedAt != nil
			if f.RedactedAt != nil && (f.Snippet != "" || f.Detail != "") {
				t.Errorf("redacted finding kept its content: %+v", f)
			}
		}
		return got
	}

	// Closed today: not expired 80 days on, expired 100 days on (the
	// default is 90). The open one, seen 200 days ago, keeps its snippet.
	expireScanSnippets(time.Now().Add(80 * 24 * time.Hour))
	if r := redacted(); r["closed.log"] || r["open.log"] {
		t.Fatalf("redacted 80 days on: %v", r)
	}
	expireScanSnippets(time.Now().Add(100 * 24 * time.Hour))
	if r := redacted(); !r["closed.log"] || r["open.log"] {
		t.Fatalf("redacted 100 days on: %v", r)
	}

	// By hand, at once, whatever the status.
	w := postJSON(t, RedactScanFindingsHandler, "POST", "/api/scan/findings/redact", map[string]any{"finding_ids": []int64{byHand.ID}})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"redacted":1`) {
		t.Fatalf("redact: %d %s", w.Code, w.Body.String())
	}
	if f := findingByPath(t, listScanFindings(t, "?site_id="+site), "by-hand.log"); f.RedactedAt == nil || f.RedactedBy != "User" || f.Rule != "stray-file" || f.SHA256 == "" {
		t.Fatalf("redacted by hand = %+v", f)
	}

	// The same file reported again brings its snippet back.
	add("by-hand.log", time.Now())
	if f := findingByPath(t, listScanFindings(t, "?site_id="+site), "by-hand.log"); f.RedactedAt != nil || f.Snippet == "" {
		t.Fatalf("reported again = %+v", f)
	}
}
