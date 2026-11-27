package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

const (
	linkProdID    = "11111111-1111-4111-8111-111111111111"
	linkStagingID = "22222222-2222-4222-8222-222222222222"
	linkDevID     = "33333333-3333-4333-8333-333333333333"
	linkOtherProd = "44444444-4444-4444-8444-444444444444"
)

func saveLinkTestSite(t *testing.T, id, domain string, env models.SiteEnvironmentType) {
	t.Helper()
	if err := db.SaveManagedSite(models.ManagedSite{
		ID: id, Provider: "manual", Domain: domain, Environment: env,
		ConnectionType: "none", Status: "active",
	}); err != nil {
		t.Fatalf("save site %s: %v", domain, err)
	}
}

func callSiteHandler(t *testing.T, h http.HandlerFunc, method, siteID, body string) *httptest.ResponseRecorder {
	t.Helper()
	ctx := contextWithClaims(context.Background(), &AuthClaims{Username: "admin", Level: config.LevelEdit})
	req := httptest.NewRequest(method, "/api/sites/"+siteID+"/x", strings.NewReader(body)).WithContext(ctx)
	req.SetPathValue("id", siteID)
	w := httptest.NewRecorder()
	h(w, req)
	return w
}

func findAPISite(t *testing.T, id string) apiSite {
	t.Helper()
	sites, err := loadAPISites()
	if err != nil {
		t.Fatalf("loadAPISites: %v", err)
	}
	for _, s := range sites {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("site %s not in /api/sites", id)
	return apiSite{}
}

func TestSiteProductionLink(t *testing.T) {
	setupSiteManagementTest(t)
	saveLinkTestSite(t, linkProdID, "example.com", models.SiteEnvironmentProduction)
	saveLinkTestSite(t, linkStagingID, "staging.example.com", models.SiteEnvironmentStaging)
	saveLinkTestSite(t, linkDevID, "dev.example.com", models.SiteEnvironmentDevelopment)
	saveLinkTestSite(t, linkOtherProd, "other.com", models.SiteEnvironmentProduction)

	link := func(siteID, prodID string) int {
		return callSiteHandler(t, SetSiteProductionLinkHandler, "PUT", siteID,
			`{"production_site_id":"`+prodID+`"}`).Code
	}

	if code := link(linkStagingID, linkProdID); code != http.StatusOK {
		t.Fatalf("link staging: got %d", code)
	}
	if code := link(linkDevID, linkProdID); code != http.StatusOK {
		t.Fatalf("link dev: got %d", code)
	}
	if got := findAPISite(t, linkStagingID).ProductionSiteID; got != linkProdID {
		t.Errorf("staging production_site_id = %q, want %q", got, linkProdID)
	}

	// Rejected: a production site as the linked site, a non-production
	// site as the parent, itself, an unknown parent.
	for name, c := range map[string][2]string{
		"production child":  {linkOtherProd, linkProdID},
		"staging as parent": {linkDevID, linkStagingID},
		"self":              {linkStagingID, linkStagingID},
	} {
		if code := link(c[0], c[1]); code != http.StatusBadRequest {
			t.Errorf("%s: got %d, want 400", name, code)
		}
	}
	if code := link(linkStagingID, "55555555-5555-4555-8555-555555555555"); code != http.StatusNotFound {
		t.Errorf("unknown parent: got %d, want 404", code)
	}

	// Relinking replaces the link.
	if code := link(linkStagingID, linkOtherProd); code != http.StatusOK {
		t.Fatalf("relink: got %d", code)
	}
	if got := findAPISite(t, linkStagingID).ProductionSiteID; got != linkOtherProd {
		t.Errorf("after relink production_site_id = %q", got)
	}

	if code := callSiteHandler(t, DeleteSiteProductionLinkHandler, "DELETE", linkStagingID, "").Code; code != http.StatusOK {
		t.Fatalf("unlink: got %d", code)
	}
	if got := findAPISite(t, linkStagingID).ProductionSiteID; got != "" {
		t.Errorf("after unlink production_site_id = %q", got)
	}

	// Deleting the production site drops the links to it.
	if err := db.DeleteManagedSite(linkProdID); err != nil {
		t.Fatal(err)
	}
	if got := findAPISite(t, linkDevID).ProductionSiteID; got != "" {
		t.Errorf("link survived its production site: %q", got)
	}
}

func TestSiteTier(t *testing.T) {
	setupSiteManagementTest(t)
	saveLinkTestSite(t, linkProdID, "example.com", models.SiteEnvironmentProduction)
	saveLinkTestSite(t, linkStagingID, "staging.example.com", models.SiteEnvironmentStaging)

	setTier := func(siteID, body string) int {
		return callSiteHandler(t, SetSiteTierHandler, "PUT", siteID, body).Code
	}

	if code := setTier(linkProdID, `{"tier":30}`); code != http.StatusOK {
		t.Fatalf("set tier: got %d", code)
	}
	if got := findAPISite(t, linkProdID).Tier; got != models.TierPremium {
		t.Errorf("tier = %d, want %d", got, models.TierPremium)
	}
	if code := setTier(linkProdID, `{"tier":25}`); code != http.StatusBadRequest {
		t.Errorf("unknown tier: got %d, want 400", code)
	}
	if code := setTier(linkStagingID, `{"tier":30}`); code != http.StatusBadRequest {
		t.Errorf("staging tier: got %d, want 400", code)
	}

	// A site moved out of production reports no tier.
	if err := db.SetSiteEnvironment(linkProdID, "archived", "test"); err != nil {
		t.Fatal(err)
	}
	if got := findAPISite(t, linkProdID).Tier; got != 0 {
		t.Errorf("archived site tier = %d, want 0", got)
	}
	if err := db.SetSiteEnvironment(linkProdID, "production", "test"); err != nil {
		t.Fatal(err)
	}

	if code := setTier(linkProdID, `{"tier":null}`); code != http.StatusOK {
		t.Fatalf("clear tier: got %d", code)
	}
	if got := findAPISite(t, linkProdID).Tier; got != 0 {
		t.Errorf("cleared tier = %d", got)
	}
}

func TestSiteTiersFromConfig(t *testing.T) {
	old := config.Cfg.SiteTiers
	t.Cleanup(func() { config.Cfg.SiteTiers = old })

	config.Cfg.SiteTiers = nil
	tiers, err := SiteTiers()
	if err != nil || len(tiers) != len(models.DefaultSiteTiers) {
		t.Fatalf("defaults: %v, %v", tiers, err)
	}

	config.Cfg.SiteTiers = []config.SiteTierConfig{{ID: 20, Name: "Standard"}, {ID: 15, Name: " Plus "}, {ID: 10, Name: "Basic"}}
	tiers, err = SiteTiers()
	if err != nil {
		t.Fatal(err)
	}
	if tiers[1] != (models.SiteTier{ID: 15, Name: "Plus"}) || tiers[0].ID != 10 || tiers[2].ID != 20 {
		t.Errorf("configured tiers not sorted/trimmed: %v", tiers)
	}

	for name, bad := range map[string][]config.SiteTierConfig{
		"duplicate id": {{ID: 10, Name: "A"}, {ID: 10, Name: "B"}},
		"zero id":      {{ID: 0, Name: "A"}},
		"empty name":   {{ID: 10, Name: " "}},
	} {
		config.Cfg.SiteTiers = bad
		if _, err := SiteTiers(); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// The site list reports each site's default core update policy and its
// override, and the nightly run reads the same through the database.
func TestSiteCoreUpdatePolicy(t *testing.T) {
	setupSiteManagementTest(t)
	saveLinkTestSite(t, linkProdID, "example.com", models.SiteEnvironmentProduction)
	saveLinkTestSite(t, linkStagingID, "staging.example.com", models.SiteEnvironmentStaging)

	setPolicy := func(siteID, body string) int {
		return callSiteHandler(t, SetSiteCoreUpdatePolicyHandler, "PUT", siteID, body).Code
	}
	check := func(id string, override, def, effective models.CoreUpdatePolicy) {
		t.Helper()
		s := findAPISite(t, id)
		if s.CoreUpdatePolicy != override || s.CoreUpdateDefault != def {
			t.Errorf("%s: policy %q, default %q; want %q, %q", s.Domain, s.CoreUpdatePolicy, s.CoreUpdateDefault, override, def)
		}
		all, err := db.GetEffectiveCoreUpdatePolicies()
		if err != nil || all[id] != effective {
			t.Errorf("%s: effective policy %q (%v), want %q", s.Domain, all[id], err, effective)
		}
	}

	check(linkProdID, "", models.CorePolicyVulnMinor, models.CorePolicyVulnMinor)
	check(linkStagingID, "", models.CorePolicyMajor, models.CorePolicyMajor)

	// A basic tier production site updates to the latest version nightly.
	if code := callSiteHandler(t, SetSiteTierHandler, "PUT", linkProdID, `{"tier":10}`).Code; code != http.StatusOK {
		t.Fatalf("set tier: got %d", code)
	}
	check(linkProdID, "", models.CorePolicyMajor, models.CorePolicyMajor)

	if code := setPolicy(linkProdID, `{"policy":"minor"}`); code != http.StatusOK {
		t.Fatalf("set policy: got %d", code)
	}
	check(linkProdID, models.CorePolicyMinor, models.CorePolicyMajor, models.CorePolicyMinor)
	if code := setPolicy(linkStagingID, `{"policy":"vuln_minor"}`); code != http.StatusOK {
		t.Fatalf("set staging policy: got %d", code)
	}
	check(linkStagingID, models.CorePolicyVulnMinor, models.CorePolicyMajor, models.CorePolicyVulnMinor)

	if code := setPolicy(linkProdID, `{"policy":"weekly"}`); code != http.StatusBadRequest {
		t.Errorf("unknown policy: got %d, want 400", code)
	}
	if code := setPolicy(linkOtherProd, `{"policy":"minor"}`); code != http.StatusNotFound {
		t.Errorf("unknown site: got %d, want 404", code)
	}

	if code := setPolicy(linkProdID, `{"policy":""}`); code != http.StatusOK {
		t.Fatalf("clear policy: got %d", code)
	}
	check(linkProdID, "", models.CorePolicyMajor, models.CorePolicyMajor)
}

func TestNightlyCoreUpdatesHandler(t *testing.T) {
	old, oldHour := config.Cfg.NightlyCoreUpdates, config.Cfg.NightlyCoreUpdateHour
	t.Cleanup(func() { config.Cfg.NightlyCoreUpdates, config.Cfg.NightlyCoreUpdateHour = old, oldHour })

	for _, enabled := range []bool{false, true} {
		config.Cfg.NightlyCoreUpdates = enabled
		w := httptest.NewRecorder()
		NightlyCoreUpdatesHandler(w, httptest.NewRequest("GET", "/api/nightly-core-updates", nil))
		want := fmt.Sprintf(`"enabled":%v`, enabled)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), want) || !strings.Contains(w.Body.String(), `"start_hour":`) {
			t.Errorf("enabled %v: %d %s", enabled, w.Code, w.Body.String())
		}
	}
}
