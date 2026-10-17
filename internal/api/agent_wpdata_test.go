package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

func postAgentReport(t *testing.T, serverID int, sites ...models.AgentReportSite) {
	t.Helper()
	body, err := json.Marshal(models.AgentReport{CollectedAt: time.Now().UTC().Format(time.RFC3339), Sites: sites})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/agent/report", bytes.NewBuffer(body))
	req = req.WithContext(contextWithAgentClaims(context.Background(), &AgentClaims{TokenID: 1, ServerID: utils.SpinupWPServerUUID(serverID)}))
	w := httptest.NewRecorder()
	AgentReportHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("report: status %d: %s", w.Code, w.Body.String())
	}
}

func fetchManifest(t *testing.T, serverID int) models.AgentManifest {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/agent/manifest", nil)
	req = req.WithContext(contextWithAgentClaims(context.Background(), &AgentClaims{TokenID: 1, ServerID: utils.SpinupWPServerUUID(serverID)}))
	w := httptest.NewRecorder()
	AgentManifestHandler(w, req)
	var m models.AgentManifest
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// The agent's WordPress data round trip: full data when the manifest hash
// differs, then the manifest carries the stored hash, a hash-only report
// counts as a collection, and the site leaves the SSH refresh.
func TestAgentWPDataRoundTrip(t *testing.T) {
	setupAgentTest(t)
	seedAgentSites(t, agentSite{301, 5, "wp.example.com", "deployed", true})
	site := utils.SpinupWPSiteUUID(301)

	m := fetchManifest(t, 5)
	if m.WPDataIntervalMinutes != 60 || len(m.Sites) != 1 || m.Sites[0].WPDataHash != "" {
		t.Fatalf("manifest = %+v", m)
	}
	if db.AgentCollectedSites()[site] {
		t.Fatal("site is agent-collected before any collection")
	}

	plugins := []models.WPPlugin{{Name: "akismet", Status: "active", Version: "5.0"}}
	core := models.SiteCore{Version: "6.6.1", MajorUpdate: "6.7"}
	hash := models.WPDataHash(plugins, core, nil)
	postAgentReport(t, 5, models.AgentReportSite{SiteID: site, WPData: &models.AgentWPData{
		CollectedAt: time.Now().UTC().Format(time.RFC3339), Hash: hash, Plugins: plugins, Core: &core,
	}})

	stored, _ := db.GetSitePlugins(site)
	if len(stored) != 1 || stored[0].Version != "5.0" {
		t.Errorf("stored plugins = %+v", stored)
	}
	if !db.AgentCollectedSites()[site] {
		t.Error("site isn't marked as agent-collected")
	}
	if got := fetchManifest(t, 5).Sites[0].WPDataHash; got != hash {
		t.Errorf("manifest hash = %q, want %q", got, hash)
	}

	// A hash-only report keeps the collection time current.
	postAgentReport(t, 5, models.AgentReportSite{SiteID: site, WPData: &models.AgentWPData{
		CollectedAt: time.Now().UTC().Format(time.RFC3339), Hash: hash,
	}})
	statuses, _ := db.ListSiteAgentWPStatus()
	if st := statuses[site]; st.CollectedAt.IsZero() || st.Error != "" {
		t.Errorf("status = %+v", st)
	}

	// A failure keeps the last collection and records the error.
	postAgentReport(t, 5, models.AgentReportSite{SiteID: site, WPData: &models.AgentWPData{
		CollectedAt: time.Now().UTC().Format(time.RFC3339), Error: "refusing to run wp-cli: owned by root",
	}})
	statuses, _ = db.ListSiteAgentWPStatus()
	if st := statuses[site]; st.CollectedAt.IsZero() || st.Error == "" {
		t.Errorf("status after failure = %+v", st)
	}
}

func TestAgentWPDataRejectsInconsistentData(t *testing.T) {
	setupAgentTest(t)
	seedAgentSites(t, agentSite{302, 5, "wp2.example.com", "deployed", true})
	site := utils.SpinupWPSiteUUID(302)

	plugins := []models.WPPlugin{{Name: "akismet", Status: "active", Version: "5.0"}}
	core := models.SiteCore{Version: "6.6.1"}
	postAgentReport(t, 5, models.AgentReportSite{SiteID: site, WPData: &models.AgentWPData{
		CollectedAt: time.Now().UTC().Format(time.RFC3339), Hash: "not-the-hash", Plugins: plugins, Core: &core,
	}})
	if stored, _ := db.GetSitePlugins(site); len(stored) != 0 {
		t.Errorf("stored plugins from a report whose hash doesn't match: %+v", stored)
	}
	statuses, _ := db.ListSiteAgentWPStatus()
	if st := statuses[site]; st.Error == "" || !st.CollectedAt.IsZero() {
		t.Errorf("status = %+v", st)
	}
}

// Users sent with the WordPress data are stored and count in the stored
// hash; a later report without users (an agent that couldn't list them)
// clears them, so the hashes agree again.
func TestAgentWPDataUsers(t *testing.T) {
	setupAgentTest(t)
	seedAgentSites(t, agentSite{303, 5, "wp3.example.com", "deployed", true})
	site := utils.SpinupWPSiteUUID(303)

	if !fetchManifest(t, 5).CollectUsers {
		t.Fatal("manifest doesn't ask for users")
	}

	plugins := []models.WPPlugin{{Name: "akismet", Status: "active", Version: "5.0"}}
	core := models.SiteCore{Version: "6.6.1"}
	users := &models.SiteUsers{
		Users:          []models.SiteUser{{ID: 1, Login: "admin", Email: "admin@example.com", Roles: "administrator"}},
		ExcludedCounts: map[string]int{"subscriber": 42, "customer": 0},
	}
	hash := models.WPDataHash(plugins, core, users)
	postAgentReport(t, 5, models.AgentReportSite{SiteID: site, WPData: &models.AgentWPData{
		CollectedAt: time.Now().UTC().Format(time.RFC3339), Hash: hash, Plugins: plugins, Core: &core, Users: users,
	}})

	stored, found, err := db.GetSiteUsers(site)
	if err != nil || !found || len(stored.Users) != 1 || stored.Users[0].Email != "admin@example.com" || stored.ExcludedCounts["subscriber"] != 42 {
		t.Fatalf("stored users = %+v, %v, %v", stored, found, err)
	}
	if got := fetchManifest(t, 5).Sites[0].WPDataHash; got != hash {
		t.Errorf("manifest hash = %q, want %q", got, hash)
	}

	// A hash that leaves the users out is rejected.
	postAgentReport(t, 5, models.AgentReportSite{SiteID: site, WPData: &models.AgentWPData{
		CollectedAt: time.Now().UTC().Format(time.RFC3339), Hash: models.WPDataHash(plugins, core, nil), Plugins: plugins, Core: &core, Users: users,
	}})
	statuses, _ := db.ListSiteAgentWPStatus()
	if st := statuses[site]; st.Error == "" {
		t.Errorf("status = %+v, want a hash mismatch", st)
	}

	noUsers := models.WPDataHash(plugins, core, nil)
	postAgentReport(t, 5, models.AgentReportSite{SiteID: site, WPData: &models.AgentWPData{
		CollectedAt: time.Now().UTC().Format(time.RFC3339), Hash: noUsers, Plugins: plugins, Core: &core,
	}})
	if _, found, _ := db.GetSiteUsers(site); found {
		t.Error("users kept after a report without them")
	}
	if got := fetchManifest(t, 5).Sites[0].WPDataHash; got != noUsers {
		t.Errorf("manifest hash = %q, want %q", got, noUsers)
	}
}
