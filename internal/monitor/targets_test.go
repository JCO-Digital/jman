package monitor

import (
	"slices"
	"sort"
	"testing"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

func createCheck(t *testing.T, c models.MonitorCheck) models.MonitorCheck {
	t.Helper()
	if err := db.CreateMonitorCheck(&c, "test"); err != nil {
		t.Fatal(err)
	}
	return c
}

func targetsFor(checks []models.MonitorCheck, sites map[string]monitoredSite) map[string]Target {
	targets := map[string]Target{}
	for _, c := range checks {
		if tg, ok := targetFor(c, sites, nil); ok {
			targets[c.ID] = tg
		}
	}
	return targets
}

func TestReconcileDefaultChecks(t *testing.T) {
	setupEngineTest(t)

	existing := createCheck(t, models.MonitorCheck{SiteID: "site-1", IsDefault: true, Enabled: true})
	paused := createCheck(t, models.MonitorCheck{SiteID: "site-3", IsDefault: true, Enabled: false})
	gone := createCheck(t, models.MonitorCheck{SiteID: "site-gone", IsDefault: true, Enabled: true})
	custom := createCheck(t, models.MonitorCheck{Host: "API.example", Path: "/healthz", Enabled: true})

	sites := map[string]monitoredSite{
		"site-1": {domain: "one.example", active: true},
		"site-2": {domain: "two.example", active: false},
		"site-3": {domain: "three.example", active: true},
	}

	checks, err := db.ListMonitorChecks()
	if err != nil {
		t.Fatal(err)
	}
	checks = reconcileDefaultChecks(checks, sites, true)

	stored, err := db.ListMonitorChecks()
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 5 || len(stored) != 5 {
		t.Fatalf("got %d checks (%d stored), want 5: a new default check for site-2 only", len(checks), len(stored))
	}
	var siteIDs []string
	for _, c := range stored {
		if c.IsDefault {
			siteIDs = append(siteIDs, c.SiteID)
		}
		if c.ID == gone.ID && c.OrphanedAt == nil {
			t.Errorf("check for a gone site was not orphaned")
		}
		if c.ID == paused.ID && c.Enabled {
			t.Errorf("reconcile resumed a paused check")
		}
	}
	sort.Strings(siteIDs)
	if want := []string{"site-1", "site-2", "site-3", "site-gone"}; !slices.Equal(siteIDs, want) {
		t.Errorf("default checks for %v, want %v", siteIDs, want)
	}

	targets := targetsFor(checks, sites)
	if len(targets) != 2 {
		t.Errorf("got targets %+v, want only site-1's default check and the custom check", targets)
	}
	if tg := targets[existing.ID]; tg.Probe.URL != "https://one.example/" || tg.Label != "one.example" || tg.Domain != "one.example" {
		t.Errorf("default check target = %+v", tg)
	}
	if tg := targets[custom.ID]; tg.Probe.URL != "https://api.example/healthz" || tg.Label != "api.example/healthz" {
		t.Errorf("custom check target = %+v", tg)
	}

	// The site comes back: the orphan mark clears.
	sites["site-gone"] = monitoredSite{domain: "back.example", active: true}
	checks = reconcileDefaultChecks(checks, sites, true)
	if again, _ := db.GetMonitorCheck(gone.ID); again == nil || again.OrphanedAt != nil {
		t.Errorf("returning site's check still orphaned: %+v", again)
	}
	if _, ok := targetsFor(checks, sites)[gone.ID]; !ok {
		t.Errorf("returning site's check is not run")
	}
}

func TestReconcileKeepsSpinupWPChecksWithoutList(t *testing.T) {
	setupEngineTest(t)

	if err := db.SaveManagedSite(models.ManagedSite{ID: "spin-1", Provider: "spinupwp", Domain: "spin.example", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	spin := createCheck(t, models.MonitorCheck{SiteID: "spin-1", IsDefault: true, Enabled: true})
	manual := createCheck(t, models.MonitorCheck{SiteID: "manual-gone", IsDefault: true, Enabled: true})

	checks, err := db.ListMonitorChecks()
	if err != nil {
		t.Fatal(err)
	}
	// An empty SpinupWP list: SpinupWP sites look gone, but aren't orphaned.
	reconcileDefaultChecks(checks, map[string]monitoredSite{}, false)

	if c, _ := db.GetMonitorCheck(spin.ID); c == nil || c.OrphanedAt != nil {
		t.Errorf("SpinupWP check orphaned without a SpinupWP list: %+v", c)
	}
	if c, _ := db.GetMonitorCheck(manual.ID); c == nil || c.OrphanedAt == nil {
		t.Errorf("manual site's check not orphaned: %+v", c)
	}
}

func TestLoadMonitoredSites(t *testing.T) {
	setupEngineTest(t)
	oldCacheDir := config.RunData.CacheDir
	config.RunData.CacheDir = t.TempDir()
	t.Cleanup(func() { config.RunData.CacheDir = oldCacheDir })

	if err := cache.WriteJSONCache("sites", []models.Site{
		{ID: 1, Domain: "live.example", Status: "deployed"},
		{ID: 2, Domain: "failed.example", Status: "failed"},
		{ID: 3, Domain: "deploying.example", Status: "deploying"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, ms := range []models.ManagedSite{
		{ID: "m-active", Provider: "manual", Domain: "active.example", Status: "active"},
		{ID: "m-paused", Provider: "manual", Domain: "paused.example", Status: "paused"},
		{ID: "m-archived", Provider: "manual", Domain: "archived.example", Status: "archived"},
		// A SpinupWP row the list no longer has: monitoring follows the list.
		{ID: utils.SpinupWPSiteUUID(9), Provider: "spinupwp", Domain: "gone.example", Status: "deployed"},
	} {
		if err := db.SaveManagedSite(ms); err != nil {
			t.Fatal(err)
		}
	}

	sites, listed, err := loadMonitoredSites()
	if err != nil || !listed {
		t.Fatalf("loadMonitoredSites: listed %v, %v", listed, err)
	}
	want := map[string]bool{
		utils.SpinupWPSiteUUID(1): true,
		utils.SpinupWPSiteUUID(2): false,
		utils.SpinupWPSiteUUID(3): false,
		"m-active":                true,
		"m-paused":                false,
	}
	if len(sites) != len(want) {
		t.Errorf("got %d sites, want %d (archived and unlisted ones are gone): %+v", len(sites), len(want), sites)
	}
	for id, active := range want {
		if s, ok := sites[id]; !ok || s.active != active {
			t.Errorf("site %s: %+v (present %v), want active=%v", id, s, ok, active)
		}
	}
}
