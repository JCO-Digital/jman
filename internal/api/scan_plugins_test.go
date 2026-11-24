package api

import (
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/sitestate"
	"github.com/JCO-Digital/jman/internal/utils"
)

func TestMain(m *testing.M) {
	// Other tests store plugin lists too; the plugin check would look
	// their slugs up on the real wordpress.org, after the test ended.
	runPluginCheck = func(func()) {}
	os.Exit(m.Run())
}

// fakeWPOrg makes the plugin check run in place and answer from slugs
// instead of wordpress.org, counting the lookups. A slug mapped to false
// fails to look up.
func fakeWPOrg(t *testing.T, slugs map[string]bool) *int {
	t.Helper()
	lookups := 0
	oldInfo, oldRun := wporgPluginInfo, runPluginCheck
	wporgPluginInfo = func(slug string) (*models.PluginInfo, error) {
		lookups++
		ok, listed := slugs[slug]
		switch {
		case !listed:
			return nil, nil
		case !ok:
			return nil, errors.New("wordpress.org is down")
		}
		return &models.PluginInfo{Slug: slug, Name: slug}, nil
	}
	runPluginCheck = func(f func()) { f() }
	t.Cleanup(func() { wporgPluginInfo, runPluginCheck = oldInfo, oldRun })
	return &lookups
}

func unknownPluginFindings(t *testing.T) map[string]models.StoredScanFinding {
	t.Helper()
	got := map[string]models.StoredScanFinding{}
	for _, f := range listScanFindings(t, "") {
		if f.Rule == ruleUnknownPlugin {
			got[f.SiteID+" "+f.Path] = f
		}
	}
	return got
}

func severities(findings map[string]models.StoredScanFinding) []string {
	var got []string
	for k, f := range findings {
		got = append(got, k+" "+f.Severity)
	}
	slices.Sort(got)
	return got
}

// A client site's backdoor: a plugin calling itself WordPress that isn't on
// wordpress.org is high even when it was already there, a plugin that
// appears without jman is medium, and a premium plugin that was there is
// low, with one hash for the slug on every site.
func TestUnknownPluginFindings(t *testing.T) {
	setupAgentTest(t)
	seedAgentSites(t,
		agentSite{601, 9, "shop.example.com", "deployed", true},
		agentSite{602, 9, "other.example.com", "deployed", true})
	lookups := fakeWPOrg(t, map[string]bool{"woocommerce": true, "flaky": false})
	shop, other := utils.SpinupWPSiteUUID(601), utils.SpinupWPSiteUUID(602)
	if err := db.SavePluginInfo(models.PluginInfo{Slug: "wordpresskey", Name: "WordPress", Author: "WordPress"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SavePluginInfo(models.PluginInfo{Slug: "polylang-pro", Name: "Polylang Pro", Author: "<a href=\"https://polylang.pro\">WP SYNTEX</a>"}); err != nil {
		t.Fatal(err)
	}
	apply := func(siteID string, src sitestate.Source, slugs ...string) {
		t.Helper()
		var plugins []models.WPPlugin
		for _, s := range slugs {
			status := "active"
			if s == "bojaco" {
				status = "must-use"
			}
			plugins = append(plugins, models.WPPlugin{Name: s, Status: status, Version: "1.0"})
		}
		if _, err := sitestate.ApplyPlugins(siteID, plugins, src, time.Now()); err != nil {
			t.Fatal(err)
		}
	}

	base := []string{"woocommerce", "jcore-ruudukko", "bojaco", "polylang-pro", "wordpresskey", "flaky"}
	apply(shop, sitestate.SourceAgent, base...)
	apply(other, sitestate.SourceAgent, "woocommerce", "polylang-pro")
	pp := "wp-content/plugins/polylang-pro"
	want := []string{
		shop + " " + pp + " low",
		shop + " wp-content/plugins/wordpresskey high",
		other + " " + pp + " low",
	}
	slices.Sort(want)
	findings := unknownPluginFindings(t)
	if got := severities(findings); !slices.Equal(got, want) {
		t.Fatalf("findings = %q, want %q", got, want)
	}
	if a, b := findings[shop+" "+pp].SHA256, findings[other+" "+pp].SHA256; a != b {
		t.Fatalf("polylang-pro hashes differ between sites: %s, %s", a, b)
	}
	if active := securityIncidents(t, "active"); len(active) != 1 || active[0].Severity != "high" {
		t.Fatalf("incidents = %+v", active)
	}
	// woocommerce, polylang-pro and wordpresskey are cached; flaky is
	// asked about again, as nothing was learnt.
	if *lookups != 4 {
		t.Fatalf("lookups = %d, want 4", *lookups)
	}

	// A plugin that appears without jman is medium, and stays medium when
	// the daily sweep sees it as already there.
	apply(shop, sitestate.SourceAgent, append(base, "cache-helper")...)
	sweepUnknownPlugins()
	// One that jman installs isn't.
	apply(other, sitestate.SourceJob, "woocommerce", "polylang-pro", "premium-forms")
	want = append(want, shop+" wp-content/plugins/cache-helper medium", other+" wp-content/plugins/premium-forms low")
	slices.Sort(want)
	if got := severities(unknownPluginFindings(t)); !slices.Equal(got, want) {
		t.Fatalf("findings = %q, want %q", got, want)
	}
}

func TestImpersonatesWordPress(t *testing.T) {
	for _, tc := range []struct {
		name, author string
		want         bool
	}{
		{"WordPress", "WordPress", true},
		{"Auto Updates", " <a href=\"https://wordpress.org\">WordPress.org</a> ", true},
		{"WordPress SEO Premium", "Team Yoast", false},
		{"WooCommerce Subscriptions", "Automattic", false},
	} {
		if got := impersonatesWordPress(&models.PluginInfo{Name: tc.name, Author: tc.author}); got != tc.want {
			t.Errorf("impersonatesWordPress(%q, %q) = %v, want %v", tc.name, tc.author, got, tc.want)
		}
	}
	if impersonatesWordPress(nil) {
		t.Error("impersonatesWordPress(nil) = true")
	}
}

// A plugin is trusted on every site by allowlisting its slug, several at
// once, and not by path acknowledgements; a plugin posing as WordPress
// with a trusted slug is still flagged.
func TestAllowlistUnknownPlugins(t *testing.T) {
	setupAgentTest(t)
	seedAgentSites(t,
		agentSite{611, 9, "one.example.com", "deployed", true},
		agentSite{612, 9, "two.example.com", "deployed", true},
		agentSite{613, 9, "three.example.com", "deployed", true})
	fakeWPOrg(t, map[string]bool{})
	one, two, three := utils.SpinupWPSiteUUID(611), utils.SpinupWPSiteUUID(612), utils.SpinupWPSiteUUID(613)
	apply := func(siteID string, slugs ...string) {
		t.Helper()
		var plugins []models.WPPlugin
		for _, s := range slugs {
			plugins = append(plugins, models.WPPlugin{Name: s, Status: "active", Version: "1.0"})
		}
		if _, err := sitestate.ApplyPlugins(siteID, plugins, sitestate.SourceAgent, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	apply(one, "polylang-pro", "gravityforms")
	apply(two, "polylang-pro")
	findings := unknownPluginFindings(t)
	pp, gf := findings[one+" wp-content/plugins/polylang-pro"], findings[one+" wp-content/plugins/gravityforms"]

	w := postJSON(t, AddScanPathAcksHandler, "POST", "/api/scan/acks", map[string]any{"finding_ids": []int64{pp.ID}, "reason": "premium"})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("acknowledging a plugin path: %d", w.Code)
	}

	w = postJSON(t, AddScanAllowlistHandler, "POST", "/api/scan/allowlist", map[string]any{"finding_ids": []int64{pp.ID, gf.ID}, "reason": "premium", "close_matching": true})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"allowlisted":3`) {
		t.Fatalf("allowlist plugins: %d %s", w.Code, w.Body.String())
	}
	if entries, _ := db.ListScanAllowlist(false); len(entries) != 2 {
		t.Fatalf("allowlist = %+v", entries)
	}

	// The slug is trusted on a site that gets the plugin later.
	apply(three, "polylang-pro")
	if f := unknownPluginFindings(t)[three+" wp-content/plugins/polylang-pro"]; f.Status != models.ScanFindingAllowlisted {
		t.Fatalf("allowlisted plugin on a new site = %+v", f)
	}

	// A plugin with a trusted slug that poses as WordPress isn't, and
	// can't be allowlisted.
	if err := db.SavePluginInfo(models.PluginInfo{Slug: "gravityforms", Name: "WordPress", Author: "WordPress"}); err != nil {
		t.Fatal(err)
	}
	apply(two, "polylang-pro", "gravityforms")
	fake := unknownPluginFindings(t)[two+" wp-content/plugins/gravityforms"]
	if fake.Status != models.ScanFindingOpen || fake.Severity != "high" || fake.IncidentID == nil {
		t.Fatalf("plugin posing as WordPress with a trusted slug = %+v", fake)
	}
	w = postJSON(t, AddScanAllowlistHandler, "POST", "/api/scan/allowlist", map[string]any{"finding_id": fake.ID, "reason": "x"})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("allowlisting a plugin posing as WordPress: %d", w.Code)
	}
}

// Per-site acknowledgements of plugins made before plugins could be
// allowlisted become one allowlist entry per plugin on startup.
func TestMovePluginPathAcks(t *testing.T) {
	setupAgentTest(t)
	seedAgentSites(t,
		agentSite{621, 9, "one.example.com", "deployed", true},
		agentSite{622, 9, "two.example.com", "deployed", true},
		agentSite{623, 9, "three.example.com", "deployed", true})
	fakeWPOrg(t, map[string]bool{})
	sites := []string{utils.SpinupWPSiteUUID(621), utils.SpinupWPSiteUUID(622), utils.SpinupWPSiteUUID(623)}
	for _, siteID := range sites {
		if _, err := sitestate.ApplyPlugins(siteID, []models.WPPlugin{{Name: "polylang-pro", Status: "active", Version: "1.0"}}, sitestate.SourceAgent, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	findings := unknownPluginFindings(t)
	path := "wp-content/plugins/polylang-pro"
	// The old acknowledgements, on two of the three sites.
	for i, reason := range []string{"premium", "client's licence"} {
		siteID := sites[i]
		if _, err := db.AddScanPathAck(models.ScanPathAck{SiteID: siteID, Rule: ruleUnknownPlugin, Path: path, Reason: reason, AuditFields: models.AuditFields{CreatedBy: "alice"}}); err != nil {
			t.Fatal(err)
		}
		if _, err := db.SetScanFindingStatus([]int64{findings[siteID+" "+path].ID}, models.ScanFindingAcknowledged, "alice"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.AddScanPathAck(models.ScanPathAck{SiteID: sites[0], Rule: "stray-file", Path: "backup.sql", Reason: "kept", AuditFields: models.AuditFields{CreatedBy: "alice"}}); err != nil {
		t.Fatal(err)
	}

	MovePluginPathAcks()
	MovePluginPathAcks()

	entries, _ := db.ListScanAllowlist(false)
	if len(entries) != 1 || entries[0].SHA256 != unknownPluginHash("polylang-pro") || entries[0].Path != path || entries[0].CreatedBy != "alice" || entries[0].Reason != "premium; client's licence" {
		t.Fatalf("allowlist = %+v", entries)
	}
	if acks, _ := db.ListScanPathAcks("", false); len(acks) != 1 || acks[0].Rule != "stray-file" {
		t.Fatalf("path acks left = %+v", acks)
	}
	if f := unknownPluginFindings(t)[sites[2]+" "+path]; f.Status != models.ScanFindingAllowlisted {
		t.Fatalf("plugin on the site without an acknowledgement = %+v", f)
	}
}
