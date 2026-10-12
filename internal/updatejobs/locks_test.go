package updatejobs

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

const wporgPackage = "https://downloads.wordpress.org/plugin/b.1.0.3.zip"

func lock(t *testing.T, plugin string) {
	t.Helper()
	if err := db.SaveUpdateLock(&models.UpdateLock{SiteID: testSite, Plugin: plugin, Comment: "test", CreatedBy: "alice"}); err != nil {
		t.Fatal(err)
	}
}

func runPluginsJob(t *testing.T, allowMajor bool, plugins ...string) models.UpdateJob {
	t.Helper()
	reqs := make([]models.PluginUpdateRequest, len(plugins))
	for i, p := range plugins {
		reqs[i] = models.PluginUpdateRequest{Name: p, OldVersion: "1.0.0"}
	}
	job := models.UpdateJob{SiteID: testSite, CreatedBy: "alice", AllowMajor: allowMajor, Plugins: reqs}
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)
	stored, err := db.GetUpdateJob(job.ID)
	if err != nil || stored == nil {
		t.Fatalf("stored job: %v %v", stored, err)
	}
	return *stored
}

// TestRunLockedPlugins covers a plugins job on a site with plugin locks: the
// unlocked plugin is updated normally, the locked WordPress.org plugin only
// to a fix release, and the locked plugin with a non-WordPress.org update
// is held back. The locks are added after the job is queued.
func TestRunLockedPlugins(t *testing.T) {
	setupRunnerTest(t)

	var normal, patch []string
	updatePlugins = func(_ models.CliSite, plugins []string) ([]wpcli.UpdateResult, error) {
		normal = plugins
		return []wpcli.UpdateResult{{Name: "a", OldVersion: "1.0.0", NewVersion: "2.0.0", Status: "Updated"}}, nil
	}
	updatePluginsPatch = func(_ models.CliSite, plugins []string) ([]wpcli.UpdateResult, error) {
		patch = plugins
		return []wpcli.UpdateResult{{Name: "b", OldVersion: "1.0.0", NewVersion: "1.0.3", Status: "Updated"}}, nil
	}
	updatePackages = func(models.CliSite) (map[string]string, error) {
		return map[string]string{
			"a": "https://downloads.wordpress.org/plugin/a.2.0.0.zip",
			"b": wporgPackage,
			"c": "https://github.com/example/c/releases/download/v1.1.0/c.zip",
		}, nil
	}
	refreshPluginCache = func(models.CliSite) error {
		setSitePlugins(t,
			models.WPPlugin{Name: "a", Version: "2.0.0"},
			models.WPPlugin{Name: "b", Version: "1.0.3", Update: "1.1.0"},
			models.WPPlugin{Name: "c", Version: "1.0.0", Update: "1.1.0"},
		)
		return nil
	}
	pluginVulnerable = func(_, plugin, version string) bool { return plugin == "c" && version == "1.0.0" }
	pluginFixedIn = func(_, plugin, _ string) string { return "1.1.0" }

	reqs := []models.PluginUpdateRequest{{Name: "a", OldVersion: "1.0.0"}, {Name: "b", OldVersion: "1.0.0"}, {Name: "c", OldVersion: "1.0.0"}}
	job := models.UpdateJob{SiteID: testSite, CreatedBy: "alice", Plugins: reqs}
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	lock(t, "b")
	lock(t, "c")
	run(job)

	if !slices.Equal(normal, []string{"a"}) {
		t.Errorf("normal update got %v, want [a]", normal)
	}
	if !slices.Equal(patch, []string{"b"}) {
		t.Errorf("patch update got %v, want [b] (c's update isn't from WordPress.org)", patch)
	}

	stored, _ := db.GetUpdateJob(job.ID)
	want := []models.UpdateResult{
		{Name: "a", OldVersion: "1.0.0", NewVersion: "2.0.0", Status: models.UpdateUpdated},
		{Name: "b", OldVersion: "1.0.0", NewVersion: "1.0.3", Status: models.UpdateUpdated},
		{Name: "c", OldVersion: "1.0.0", NewVersion: "1.0.0", Status: models.UpdateSkippedLocked,
			Note: "Still vulnerable; fixed in 1.1.0 or later, which the update lock holds back"},
	}
	assertResults(t, stored.Results, want)

	ledger, _ := db.GetSiteUpdateLedger(testSite)
	if len(ledger) != 1 || ledger[0].Status != models.LedgerPartial {
		t.Fatalf("ledger = %+v, want one partial entry", ledger)
	}
	var data struct {
		Updates []map[string]any `json:"updates"`
		Summary string           `json:"summary"`
	}
	if err := json.Unmarshal([]byte(ledger[0].DataJSON), &data); err != nil {
		t.Fatal(err)
	}
	if data.Updates[2]["status"] != "skipped" || !strings.Contains(data.Summary, "1 held back by update locks") {
		t.Errorf("ledger data = %s", ledger[0].DataJSON)
	}
}

// A locked WordPress.org plugin with no fix release available is skipped:
// wp-cli leaves it out of its output, so only the refreshed cache shows it
// still has an update.
func TestRunLockedPluginWithoutFixRelease(t *testing.T) {
	setupRunnerTest(t)
	lock(t, "")
	updatePlugins = func(models.CliSite, []string) ([]wpcli.UpdateResult, error) {
		t.Error("locked plugins must not get a normal update")
		return nil, nil
	}
	updatePluginsPatch = func(models.CliSite, []string) ([]wpcli.UpdateResult, error) { return nil, nil }
	updatePackages = func(models.CliSite) (map[string]string, error) {
		return map[string]string{"b": wporgPackage}, nil
	}
	refreshPluginCache = func(models.CliSite) error {
		setSitePlugins(t, models.WPPlugin{Name: "b", Version: "1.0.0", Update: "1.1.0"})
		return nil
	}

	stored := runPluginsJob(t, false, "b")
	want := []models.UpdateResult{{Name: "b", OldVersion: "1.0.0", NewVersion: "1.0.0", Status: models.UpdateSkippedLocked}}
	assertResults(t, stored.Results, want)
}

func TestRunLockedPluginAllowMajor(t *testing.T) {
	setupRunnerTest(t)
	lock(t, "b")
	var normal []string
	updatePlugins = func(_ models.CliSite, plugins []string) ([]wpcli.UpdateResult, error) {
		normal = plugins
		return []wpcli.UpdateResult{{Name: "b", OldVersion: "1.0.0", NewVersion: "2.0.0", Status: "Updated"}}, nil
	}
	updatePluginsPatch = func(models.CliSite, []string) ([]wpcli.UpdateResult, error) {
		t.Error("a confirmed major update must not be limited to fix releases")
		return nil, nil
	}

	stored := runPluginsJob(t, true, "b")
	if !slices.Equal(normal, []string{"b"}) || stored.Results[0].Status != models.UpdateUpdated {
		t.Errorf("normal update got %v, results %+v", normal, stored.Results)
	}
	locks, _ := db.GetSiteLocks(testSite)
	if !locks.PluginLocked("b") {
		t.Error("the lock must stay in place after a major update")
	}
}

// If the update sources can't be checked, locked plugins aren't updated at
// all, while unlocked plugins still are.
func TestRunLockedPluginsSourceCheckFails(t *testing.T) {
	setupRunnerTest(t)
	lock(t, "b")
	updatePlugins = func(models.CliSite, []string) ([]wpcli.UpdateResult, error) {
		return []wpcli.UpdateResult{{Name: "a", OldVersion: "1.0.0", NewVersion: "2.0.0", Status: "Updated"}}, nil
	}
	updatePluginsPatch = func(models.CliSite, []string) ([]wpcli.UpdateResult, error) {
		t.Error("no patch update without a source check")
		return nil, nil
	}
	updatePackages = func(models.CliSite) (map[string]string, error) { return nil, errors.New("ssh down") }

	stored := runPluginsJob(t, false, "a", "b")
	if stored.Results[0].Status != models.UpdateUpdated || stored.Results[1].Status != models.UpdateFailed {
		t.Errorf("results = %+v", stored.Results)
	}
	if !strings.Contains(stored.Error, "ssh down") {
		t.Errorf("job error = %q", stored.Error)
	}
}

func TestRunCoreJobOnLockedSite(t *testing.T) {
	for _, allowMajor := range []bool{false, true} {
		t.Run(map[bool]string{false: "held back", true: "allowed"}[allowMajor], func(t *testing.T) {
			setupRunnerTest(t)
			lock(t, "")
			var gotMajor bool
			updateCore = func(_ models.CliSite, major bool) (wpcli.CoreUpdateResult, error) {
				gotMajor = major
				return wpcli.CoreUpdateResult{Success: true, Version: "6.4.3"}, nil
			}
			refreshCore = func(site models.CliSite) (*models.SiteCore, error) {
				return &models.SiteCore{SiteID: site.ID, Version: "6.4.3"}, nil
			}

			job := models.UpdateJob{Kind: models.UpdateJobKindCore, SiteID: testSite, Target: "major", AllowMajor: allowMajor, CreatedBy: "alice"}
			if err := db.CreateUpdateJob(&job); err != nil {
				t.Fatal(err)
			}
			run(job)

			if gotMajor != allowMajor {
				t.Errorf("major = %v, want %v", gotMajor, allowMajor)
			}
			stored, _ := db.GetUpdateJob(job.ID)
			if hasNote := stored.Results[0].Note != ""; hasNote == allowMajor {
				t.Errorf("result = %+v", stored.Results[0])
			}
		})
	}
}

// A plugin lock doesn't lock core.
func TestRunCoreJobWithOnlyPluginLocks(t *testing.T) {
	setupRunnerTest(t)
	lock(t, "b")
	var gotMajor bool
	updateCore = func(_ models.CliSite, major bool) (wpcli.CoreUpdateResult, error) {
		gotMajor = major
		return wpcli.CoreUpdateResult{Success: true, Version: "6.5.0"}, nil
	}
	refreshCore = func(site models.CliSite) (*models.SiteCore, error) {
		return &models.SiteCore{SiteID: site.ID, Version: "6.5.0"}, nil
	}
	runCoreJob(t, "major")
	if !gotMajor {
		t.Error("a plugin lock must not hold back a major core update")
	}
}
