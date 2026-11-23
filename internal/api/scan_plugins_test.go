package api

import (
	"errors"
	"os"
	"slices"
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
