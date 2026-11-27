package updatejobs

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/textdiff"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

const testSite = "11111111-1111-1111-1111-111111111111"

func setupRunnerTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	oldData, oldConfig := config.RunData.DataDir, config.RunData.ConfigDir
	config.RunData.DataDir, config.RunData.ConfigDir = dir, dir
	if err := db.InitInventory(); err != nil {
		t.Fatal(err)
	}
	if err := db.InitAPI(); err != nil {
		t.Fatal(err)
	}

	oldFind, oldUpdate, oldRefresh, oldQueue := findSite, updatePlugins, refreshPluginCache, q
	oldUpdateCore, oldRefreshCore, oldCachedCore := updateCore, refreshCore, cachedCoreVersion
	oldAction, oldInstall, oldUpload, oldSSH := pluginAction, installPlugin, uploadFile, runSSH
	oldVulnerable, oldFixedIn := pluginVulnerable, pluginFixedIn
	oldPatch, oldPackages := updatePluginsPatch, updatePackages
	oldCLIPHP, oldSitePHP := cliPHPVersion, sitePHPVersion
	oldReset := resetRewriteRules
	resetRewriteRules = func(models.CliSite) error { return nil }
	cliPHPVersion = func(models.CliSite) (string, error) { return "8.1.2", nil }
	sitePHPVersion = func(string) string { return "8.3" }
	oldServers := siteServers
	siteServers = func() map[string]string { return nil }
	q = newQueue()
	pluginVulnerable = func(string, string, string) bool { return false }
	pluginFixedIn = func(string, string, string) string { return "" }
	updatePackages = func(models.CliSite) (map[string]string, error) { return map[string]string{}, nil }
	cachedCoreVersion = func(string) string { return "6.4.0" }
	findSite = func(siteID string) (*models.CliSite, error) {
		return &models.CliSite{ID: siteID, Name: "example.com"}, nil
	}
	refreshPluginCache = func(models.CliSite) error { return nil }
	oldSync := syncSiteExposures
	syncSiteExposures = func(string) error { return nil }
	// No network in tests: every page fetch fails unless a test sets one.
	oldFetch, oldPaths, oldHeaders, oldMutes, oldQuery := fetchPage, textDiffPaths, textDiffHeaders, textDiffMutes, textDiffQuery
	textDiffQuery = func() string { return "" }
	fetchPage = func(string, map[string]string) textdiff.Page { return textdiff.Page{Error: "not fetched in tests"} }
	textDiffPaths = func(string) ([]string, error) { return []string{"/"}, nil }
	textDiffHeaders = func(string, string) map[string]string { return nil }
	textDiffMutes = func(string) ([]models.TextDiffMute, error) { return nil, nil }
	t.Cleanup(func() {
		// Stop any runner a test started (its context is already cancelled)
		// and wait for it before restoring the package state it reads.
		q.close()
		running.Wait()
		findSite, updatePlugins, refreshPluginCache, q = oldFind, oldUpdate, oldRefresh, oldQueue
		syncSiteExposures = oldSync
		siteServers = oldServers
		fetchPage, textDiffPaths, textDiffHeaders, textDiffMutes, textDiffQuery = oldFetch, oldPaths, oldHeaders, oldMutes, oldQuery
		updateCore, refreshCore, cachedCoreVersion = oldUpdateCore, oldRefreshCore, oldCachedCore
		pluginAction, installPlugin, uploadFile, runSSH = oldAction, oldInstall, oldUpload, oldSSH
		pluginVulnerable, pluginFixedIn = oldVulnerable, oldFixedIn
		updatePluginsPatch, updatePackages = oldPatch, oldPackages
		cliPHPVersion, sitePHPVersion = oldCLIPHP, oldSitePHP
		resetRewriteRules = oldReset
		db.Close()
		config.RunData.DataDir, config.RunData.ConfigDir = oldData, oldConfig
	})
}

func TestBuildResults(t *testing.T) {
	reqs := []models.PluginUpdateRequest{
		{Name: "a", OldVersion: "1.0"},
		{Name: "b", OldVersion: "2.0"},
		{Name: "c", OldVersion: "3.0"},
		{Name: "d", OldVersion: "4.0"},
	}

	t.Run("success", func(t *testing.T) {
		got := buildResults(reqs[:2], []wpcli.UpdateResult{{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: "Updated"}}, nil)
		want := []models.UpdateResult{
			{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: models.UpdateUpdated},
			{Name: "b", OldVersion: "2.0", NewVersion: "2.0", Status: models.UpdateUpToDate},
		}
		assertResults(t, got, want)
	})

	t.Run("failed or timed-out batch", func(t *testing.T) {
		failure := &wpcli.UpdateFailure{
			Err:      errors.New("failed to update plugin: wp-cli timed out after 16m0s"),
			Versions: map[string]string{"b": "2.1", "c": "3.0"},
		}
		updates := []wpcli.UpdateResult{
			{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: "Updated"},
			{Name: "d", OldVersion: "4.0", NewVersion: "4.1", Status: "Error"},
		}
		got := buildResults(reqs, updates, failure)
		want := []models.UpdateResult{
			// Reported by WP-CLI before the failure.
			{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: models.UpdateUpdated},
			// Not reported, but the check found the new version installed.
			{Name: "b", OldVersion: "2.0", NewVersion: "2.1", Status: models.UpdateUpdated},
			// Not reported and unchanged.
			{Name: "c", OldVersion: "3.0", NewVersion: "3.0", Status: models.UpdateFailed, Error: failure.Error()},
			// Reported as failed by WP-CLI.
			{Name: "d", OldVersion: "4.0", NewVersion: "4.0", Status: models.UpdateFailed, Error: "Error"},
		}
		assertResults(t, got, want)
	})

	t.Run("unknown old version is not assumed updated", func(t *testing.T) {
		failure := &wpcli.UpdateFailure{Err: errors.New("boom"), Versions: map[string]string{"x": "9.9"}}
		got := buildResults([]models.PluginUpdateRequest{{Name: "x"}}, nil, failure)
		if got[0].Status != models.UpdateFailed || got[0].NewVersion != "9.9" {
			t.Errorf("got %+v, want failed with the observed version", got[0])
		}
	})
}

func assertResults(t *testing.T, got, want []models.UpdateResult) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d results %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("result %d:\n got  %+v\n want %+v", i, got[i], want[i])
		}
	}
}

func TestRunWritesResultsAndOneLedgerEntry(t *testing.T) {
	setupRunnerTest(t)
	var gotPlugins []string
	updatePlugins = func(_ models.CliSite, plugins []string, _ *[]models.WPCLICall) ([]wpcli.UpdateResult, error) {
		gotPlugins = plugins
		return []wpcli.UpdateResult{
			{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: "Updated"},
			{Name: "b", OldVersion: "2.0", NewVersion: "2.1", Status: "Updated"},
		}, nil
	}

	job := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a", OldVersion: "1.0"}, models.PluginUpdateRequest{Name: "b", OldVersion: "2.0"})
	job.CreatedBy = "alice"
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)

	if len(gotPlugins) != 2 {
		t.Errorf("plugins updated in one call = %v, want both", gotPlugins)
	}
	stored, err := db.GetUpdateJob(job.ID)
	if err != nil || stored == nil {
		t.Fatalf("GetUpdateJob: %v, %v", stored, err)
	}
	if stored.Status != models.UpdateJobDone || stored.StartedAt == nil || stored.FinishedAt == nil || len(stored.Results) != 2 {
		t.Errorf("stored job = %+v", stored)
	}

	ledger, err := db.GetSiteUpdateLedger(testSite)
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger) != 1 {
		t.Fatalf("got %d ledger entries, want one for the whole batch", len(ledger))
	}
	entry := ledger[0]
	if entry.Status != "full" || entry.UpdatedBy != "alice" {
		t.Errorf("ledger entry status=%q by=%q, want full by alice", entry.Status, entry.UpdatedBy)
	}
	var data struct {
		Summary string           `json:"summary"`
		Updates []map[string]any `json:"updates"`
	}
	if err := json.Unmarshal([]byte(entry.DataJSON), &data); err != nil || len(data.Updates) != 2 || data.Summary == "" {
		t.Errorf("ledger data = %s (%v)", entry.DataJSON, err)
	}
}

func TestRunRecordsUnreachableSite(t *testing.T) {
	setupRunnerTest(t)
	findSite = func(string) (*models.CliSite, error) { return nil, errors.New("site gone") }
	updatePlugins = func(models.CliSite, []string, *[]models.WPCLICall) ([]wpcli.UpdateResult, error) {
		t.Fatal("update must not run for an unreachable site")
		return nil, nil
	}

	job := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a", OldVersion: "1.0"})
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)

	stored, _ := db.GetUpdateJob(job.ID)
	if stored.Status != models.UpdateJobFailed || stored.Error != "site gone" || stored.Results[0].Status != models.UpdateFailed {
		t.Errorf("stored job = %+v", stored)
	}
	ledger, _ := db.GetSiteUpdateLedger(testSite)
	if len(ledger) != 1 || ledger[0].Status != "failed" {
		t.Errorf("ledger = %+v, want one failed entry", ledger)
	}
}

func TestStartInterruptsRunningAndResumesQueued(t *testing.T) {
	setupRunnerTest(t)

	running := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a"})
	queued := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "b"})
	queued.SiteID = "22222222-2222-2222-2222-222222222222"
	for _, j := range []*models.UpdateJob{&running, &queued} {
		if err := db.CreateUpdateJob(j); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.StartUpdateJob(running.ID); err != nil {
		t.Fatal(err)
	}

	ran := make(chan string, 2)
	updatePlugins = func(site models.CliSite, plugins []string, _ *[]models.WPCLICall) ([]wpcli.UpdateResult, error) {
		ran <- site.ID
		return nil, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := Start(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case id := <-ran:
		if id != queued.SiteID {
			t.Errorf("ran job for site %s, want the queued job's site", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queued job did not run")
	}

	stored, _ := db.GetUpdateJob(running.ID)
	if stored.Status != models.UpdateJobInterrupted {
		t.Errorf("previously running job status = %q, want interrupted", stored.Status)
	}
	select {
	case id := <-ran:
		t.Errorf("interrupted job was run again (site %s)", id)
	case <-time.After(200 * time.Millisecond):
	}
}

func runCoreJob(t *testing.T, target string) *models.UpdateJob {
	t.Helper()
	job := models.NewUpdateJob(models.UpdateJobKindCore, testSite, models.CorePayload{Target: target})
	job.CreatedBy = "alice"
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)
	stored, err := db.GetUpdateJob(job.ID)
	if err != nil || stored == nil {
		t.Fatalf("GetUpdateJob: %v, %v", stored, err)
	}
	return stored
}

func coreLedger(t *testing.T) (models.SiteUpdateLedgerEntry, map[string]any) {
	t.Helper()
	ledger, err := db.GetSiteUpdateLedger(testSite)
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger) != 1 || ledger[0].UpdateType != "core" {
		t.Fatalf("ledger = %+v, want one core entry", ledger)
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(ledger[0].DataJSON), &data); err != nil {
		t.Fatal(err)
	}
	return ledger[0], data
}

func TestRunCoreJobSuccess(t *testing.T) {
	setupRunnerTest(t)
	var gotMajor bool
	updateCore = func(_ models.CliSite, major bool, _ *[]models.WPCLICall) (wpcli.CoreUpdateResult, error) {
		gotMajor = major
		return wpcli.CoreUpdateResult{Success: true, Version: "6.5.0"}, nil
	}
	refreshCore = func(site models.CliSite) (*models.SiteCore, error) {
		return &models.SiteCore{SiteID: site.ID, Version: "6.5.0"}, nil
	}

	stored := runCoreJob(t, "major")
	if !gotMajor {
		t.Error("major target did not request a major update")
	}
	if stored.Status != models.UpdateJobDone || jobCore(*stored).Target != "major" || stored.Core == nil || stored.Core.Version != "6.5.0" {
		t.Errorf("stored job = %+v", stored)
	}
	want := []models.UpdateResult{{Name: "WordPress", OldVersion: "6.4.0", NewVersion: "6.5.0", Status: models.UpdateUpdated}}
	assertResults(t, stored.Results, want)

	entry, data := coreLedger(t)
	if entry.Status != "full" || entry.UpdatedBy != "alice" || data["new_version"] != "6.5.0" || data["target"] != "major" {
		t.Errorf("ledger entry = %+v data %v", entry, data)
	}
}

func TestRunCoreJobAlreadyLatest(t *testing.T) {
	setupRunnerTest(t)
	updateCore = func(models.CliSite, bool, *[]models.WPCLICall) (wpcli.CoreUpdateResult, error) {
		return wpcli.CoreUpdateResult{Version: "unknown"}, nil
	}
	refreshCore = func(site models.CliSite) (*models.SiteCore, error) {
		return &models.SiteCore{SiteID: site.ID, Version: "6.4.0"}, nil
	}

	stored := runCoreJob(t, "minor")
	if stored.Status != models.UpdateJobDone || stored.Error != "" {
		t.Errorf("stored job = %+v", stored)
	}
	want := []models.UpdateResult{{Name: "WordPress", OldVersion: "6.4.0", NewVersion: "6.4.0", Status: models.UpdateUpToDate}}
	assertResults(t, stored.Results, want)

	entry, data := coreLedger(t)
	if entry.Status != "partial" || data["new_version"] != "6.4.0" {
		t.Errorf("ledger entry = %+v data %v", entry, data)
	}
}

func TestRunCoreJobFailure(t *testing.T) {
	setupRunnerTest(t)
	updateCore = func(models.CliSite, bool, *[]models.WPCLICall) (wpcli.CoreUpdateResult, error) {
		return wpcli.CoreUpdateResult{Version: "unknown"}, errors.New("download failed")
	}
	refreshCore = func(models.CliSite) (*models.SiteCore, error) {
		return nil, errors.New("ssh down")
	}

	stored := runCoreJob(t, "minor")
	if stored.Status != models.UpdateJobDone || stored.Error != "download failed" || stored.Core != nil {
		t.Errorf("stored job = %+v", stored)
	}
	if len(stored.Results) != 1 || stored.Results[0].Status != models.UpdateFailed || stored.Results[0].NewVersion != "6.4.0" {
		t.Errorf("results = %+v", stored.Results)
	}

	entry, data := coreLedger(t)
	if entry.Status != "failed" || data["error"] != "download failed" {
		t.Errorf("ledger entry = %+v data %v", entry, data)
	}
}

func TestStartInterruptsRunningCoreJob(t *testing.T) {
	setupRunnerTest(t)
	job := models.NewUpdateJob(models.UpdateJobKindCore, testSite, models.CorePayload{Target: "minor"})
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	if err := db.StartUpdateJob(job.ID); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := Start(ctx); err != nil {
		t.Fatal(err)
	}

	stored, _ := db.GetUpdateJob(job.ID)
	if stored.Status != models.UpdateJobInterrupted || stored.Error != interruptedCoreReason {
		t.Errorf("stored job = %+v", stored)
	}
	entry, data := coreLedger(t)
	if entry.Status != "failed" || data["error"] != interruptedCoreReason {
		t.Errorf("ledger entry = %+v data %v", entry, data)
	}
}

func TestLedgerStatusForPluginUpdates(t *testing.T) {
	setupRunnerTest(t)
	ok := []models.UpdateResult{{Name: "a", Status: models.UpdateUpdated}}
	failed := []models.UpdateResult{{Name: "a", Status: models.UpdateFailed}}
	// Vulnerable: "vuln-left" at 1.0. "a" was vulnerable at 1.0 too.
	pluginVulnerable = func(_, plugin, version string) bool {
		return (plugin == "vuln-left" || plugin == "a") && version == "1.0"
	}

	cases := []struct {
		name              string
		cached            []models.WPPlugin
		results           []models.UpdateResult
		cacheFresh        bool
		updatedVulnerable bool
		want              string
	}{
		{"failed plugin", nil, failed, true, true, models.LedgerFailed},
		{"stale cache", nil, ok, false, true, models.LedgerPartial},
		{"site up to date", []models.WPPlugin{{Name: "a", Version: "1.1"}}, ok, true, false, models.LedgerFull},
		{"vulnerable fixed, others left", []models.WPPlugin{{Name: "a", Version: "1.1"}, {Name: "other", Version: "2.0", Update: "2.1"}}, ok, true, true, models.LedgerVuln},
		{"vulnerable update left", []models.WPPlugin{{Name: "vuln-left", Version: "1.0", Update: "1.1"}}, ok, true, true, models.LedgerPartial},
		{"nothing vulnerable updated", []models.WPPlugin{{Name: "other", Version: "2.0", Update: "2.1"}}, ok, true, false, models.LedgerPartial},
	}
	for _, c := range cases {
		setSitePlugins(t, c.cached...)
		if got := ledgerStatus(testSite, c.results, c.cacheFresh, c.updatedVulnerable); got != c.want {
			t.Errorf("%s: status %q, want %q", c.name, got, c.want)
		}
	}

	if !anyVulnerable(testSite, []models.PluginUpdateRequest{{Name: "b", OldVersion: "1.0"}, {Name: "a", OldVersion: "1.0"}}) {
		t.Error("anyVulnerable missed the vulnerable plugin")
	}
	if anyVulnerable(testSite, []models.PluginUpdateRequest{{Name: "a"}}) {
		t.Error("anyVulnerable counted a plugin with an unknown version")
	}
}

func TestRunPluginUpdateWritesVulnStatus(t *testing.T) {
	setupRunnerTest(t)
	setSitePlugins(t,
		models.WPPlugin{Name: "a", Version: "1.0", Update: "1.1"},
		models.WPPlugin{Name: "b", Version: "2.0", Update: "2.1"},
	)
	pluginVulnerable = func(_, plugin, version string) bool { return plugin == "a" && version == "1.0" }
	updatePlugins = func(models.CliSite, []string, *[]models.WPCLICall) ([]wpcli.UpdateResult, error) {
		return []wpcli.UpdateResult{{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: "Updated"}}, nil
	}
	refreshPluginCache = func(models.CliSite) error {
		setSitePlugins(t,
			models.WPPlugin{Name: "a", Version: "1.1"},
			models.WPPlugin{Name: "b", Version: "2.0", Update: "2.1"},
		)
		return nil
	}

	job := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a", OldVersion: "1.0"})
	job.CreatedBy = "alice"
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)

	ledger, _ := db.GetSiteUpdateLedger(testSite)
	if len(ledger) != 1 || ledger[0].Status != models.LedgerVuln {
		t.Errorf("ledger = %+v, want one vuln entry", ledger)
	}
}

func TestRunSyncsSiteExposuresAfterPluginJob(t *testing.T) {
	setupRunnerTest(t)
	var synced []string
	syncSiteExposures = func(siteID string) error {
		synced = append(synced, siteID)
		return nil
	}
	updatePlugins = func(_ models.CliSite, plugins []string, _ *[]models.WPCLICall) ([]wpcli.UpdateResult, error) {
		return []wpcli.UpdateResult{{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: "Updated"}}, nil
	}

	job := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a", OldVersion: "1.0"})
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)

	if len(synced) != 1 || synced[0] != job.SiteID {
		t.Errorf("synced exposures for %v, want once for %s", synced, job.SiteID)
	}
}

// A job that updated something resets the site's rewrite rules, before the
// text diff's after fetch; one that updated nothing, or only installed a
// new plugin, doesn't.
func TestRewriteRulesResetAfterUpdates(t *testing.T) {
	setupRunnerTest(t)
	var resets, events []string
	resetRewriteRules = func(site models.CliSite) error {
		resets = append(resets, site.ID)
		events = append(events, "reset")
		return nil
	}
	fetchPage = func(string, map[string]string) textdiff.Page {
		events = append(events, "fetch")
		return textdiff.Page{Error: "not fetched in tests"}
	}
	version := "1.1"
	updatePlugins = func(_ models.CliSite, plugins []string, _ *[]models.WPCLICall) ([]wpcli.UpdateResult, error) {
		if version == "1.0" {
			return nil, nil
		}
		return []wpcli.UpdateResult{{Name: "a", OldVersion: "1.0", NewVersion: version, Status: "Updated"}}, nil
	}

	job := createJob(t, pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a", OldVersion: "1.0"}))
	run(job)
	if len(resets) != 1 || resets[0] != testSite {
		t.Fatalf("resets after an update = %v, want one", resets)
	}
	if n := len(events); n < 2 || events[n-2] != "reset" || events[n-1] != "fetch" {
		t.Errorf("events = %v, want the reset right before the after fetch", events)
	}

	version = "1.0"
	job = createJob(t, pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a", OldVersion: "1.0"}))
	run(job)
	if len(resets) != 1 {
		t.Errorf("resets after a job that updated nothing = %v", resets)
	}

	// A failed reset doesn't fail the job.
	version = "1.2"
	resetRewriteRules = func(models.CliSite) error { return errors.New("wp-cli broke") }
	job = createJob(t, pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a", OldVersion: "1.1"}))
	run(job)
	if stored := storedJob(t, job.ID); stored.Status != models.UpdateJobDone || stored.Error != "" {
		t.Errorf("job after a failed reset = %+v", stored)
	}

	// Jobs that run together reset the rules once.
	version = "1.3"
	resets = nil
	resetRewriteRules = func(site models.CliSite) error { resets = append(resets, site.ID); return nil }
	first := createJob(t, pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a", OldVersion: "1.2"}))
	second := createJob(t, pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a", OldVersion: "1.2"}))
	runBatch([]models.UpdateJob{first, second})
	if len(resets) != 1 {
		t.Errorf("resets after a combined run = %v, want one", resets)
	}

	if updatesSite(models.NewUpdateJob(models.UpdateJobKindInstall, testSite, models.InstallPayload{Source: "akismet"})) {
		t.Error("installing a new plugin counts as an update")
	}
	if !updatesSite(models.NewUpdateJob(models.UpdateJobKindInstall, testSite, models.InstallPayload{Source: "x.zip", Plugin: "x", OldVersion: "1.0"})) {
		t.Error("a ZIP update doesn't count as an update")
	}
}

// A plugin whose cached update needs a newer WordPress isn't passed to
// wp-cli: it's skipped with the reason, and the others still update.
func TestRunSkipsUpdatesTheSiteCantTake(t *testing.T) {
	setupRunnerTest(t)
	reason := "This update requires WordPress version 7.0, but the version installed is 6.9.12."
	if err := db.ReplaceSitePlugins(testSite, []models.WPPlugin{
		{Name: "a", Status: "active", Version: "1.0", Update: "1.1"},
		{Name: "wordpress-seo", Status: "active", Version: "26.4", Update: "27.0", UpdateBlocked: reason},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	var gotPlugins []string
	updatePlugins = func(_ models.CliSite, plugins []string, _ *[]models.WPCLICall) ([]wpcli.UpdateResult, error) {
		gotPlugins = plugins
		return []wpcli.UpdateResult{{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: "Updated"}}, nil
	}
	refreshPluginCache = func(models.CliSite) error {
		return db.ReplaceSitePlugins(testSite, []models.WPPlugin{
			{Name: "a", Status: "active", Version: "1.1"},
			{Name: "wordpress-seo", Status: "active", Version: "26.4", Update: "27.0", UpdateBlocked: reason},
		}, time.Now())
	}

	job := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a", OldVersion: "1.0"}, models.PluginUpdateRequest{Name: "wordpress-seo", OldVersion: "26.4"})
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)

	if !slices.Equal(gotPlugins, []string{"a"}) {
		t.Errorf("plugins passed to wp-cli = %v, want only a", gotPlugins)
	}
	stored, err := db.GetUpdateJob(job.ID)
	if err != nil || stored == nil {
		t.Fatalf("GetUpdateJob: %v, %v", stored, err)
	}
	want := []models.UpdateResult{
		{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: models.UpdateUpdated},
		{Name: "wordpress-seo", OldVersion: "26.4", NewVersion: "26.4", Status: models.UpdateSkippedRequirements, Note: strings.TrimSuffix(reason, ".")},
	}
	assertResults(t, stored.Results, want)
	if stored.Status != models.UpdateJobDone || stored.Error != "" {
		t.Errorf("job status = %q error = %q, want done without an error", stored.Status, stored.Error)
	}
}

// When every plugin is blocked, wp-cli isn't run at all.
func TestRunRechecksStaleBlock(t *testing.T) {
	setupRunnerTest(t)
	if err := db.ReplaceSitePlugins(testSite, []models.WPPlugin{
		{Name: "wordpress-seo", Status: "active", Version: "26.4", Update: "27.0", UpdateBlocked: "This update requires WordPress version 7.0, but the version installed is 6.9.12."},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	// WordPress updated itself to 7.0 since the list was cached.
	refreshPluginCache = func(models.CliSite) error {
		return db.ReplaceSitePlugins(testSite, []models.WPPlugin{
			{Name: "wordpress-seo", Status: "active", Version: "26.4", Update: "27.0"},
		}, time.Now().Add(time.Second))
	}
	var gotPlugins []string
	updatePlugins = func(_ models.CliSite, plugins []string, _ *[]models.WPCLICall) ([]wpcli.UpdateResult, error) {
		gotPlugins = plugins
		return []wpcli.UpdateResult{{Name: "wordpress-seo", OldVersion: "26.4", NewVersion: "27.0", Status: "Updated"}}, nil
	}
	job := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "wordpress-seo", OldVersion: "26.4"})
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)
	if !slices.Equal(gotPlugins, []string{"wordpress-seo"}) {
		t.Errorf("plugins passed to wp-cli = %v, want the no longer blocked update", gotPlugins)
	}
}

func TestRunAllBlockedRunsNoUpdate(t *testing.T) {
	setupRunnerTest(t)
	if err := db.ReplaceSitePlugins(testSite, []models.WPPlugin{
		{Name: "b", Status: "active", Version: "1.0", Update: "2.0", UpdateBlocked: "This update requires PHP version 8.0, but the version installed is 7.4.33."},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	updatePlugins = func(models.CliSite, []string, *[]models.WPCLICall) ([]wpcli.UpdateResult, error) {
		t.Error("wp-cli was run for a blocked update")
		return nil, nil
	}
	job := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "b", OldVersion: "1.0"})
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)
	stored, _ := db.GetUpdateJob(job.ID)
	if stored == nil || len(stored.Results) != 1 || stored.Results[0].Status != models.UpdateSkippedRequirements {
		t.Errorf("stored job = %+v", stored)
	}
}

// A core update that changed the version refreshes the plugin list, so
// updates that waited for it are no longer blocked.
func TestRunCoreJobRefreshesPlugins(t *testing.T) {
	setupRunnerTest(t)
	refreshed := false
	refreshPluginCache = func(models.CliSite) error { refreshed = true; return nil }
	updateCore = func(models.CliSite, bool, *[]models.WPCLICall) (wpcli.CoreUpdateResult, error) {
		return wpcli.CoreUpdateResult{Success: true, Version: "7.0"}, nil
	}
	refreshCore = func(site models.CliSite) (*models.SiteCore, error) {
		return &models.SiteCore{SiteID: site.ID, Version: "7.0"}, nil
	}
	runCoreJob(t, "major")
	if !refreshed {
		t.Error("plugin list not refreshed after the core update")
	}
}

// A failed plugin gets its cause from the call's output; one that updated
// in the same call doesn't.
func TestRunLabelsFailureCause(t *testing.T) {
	setupRunnerTest(t)
	updatePlugins = func(_ models.CliSite, plugins []string, rec *[]models.WPCLICall) ([]wpcli.UpdateResult, error) {
		*rec = append(*rec, models.WPCLICall{
			Args:   append([]string{"plugin", "update"}, plugins...),
			Stderr: "Warning: Update package not available.\nError: No plugins updated (1 failed).",
			Error:  "Error: No plugins updated (1 failed).",
		})
		return []wpcli.UpdateResult{
			{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: "Updated"},
			{Name: "polylang-pro", OldVersion: "3.5", NewVersion: "3.9.2", Status: "Error", Error: "Warning: Update package not available.; Error: No plugins updated (1 failed)."},
		}, &wpcli.UpdateFailure{Err: errors.New("failed to update plugin: Error: No plugins updated (1 failed).")}
	}
	job := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a", OldVersion: "1.0"}, models.PluginUpdateRequest{Name: "polylang-pro", OldVersion: "3.5"})
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)
	stored, _ := db.GetUpdateJob(job.ID)
	if stored == nil || len(stored.Results) != 2 {
		t.Fatalf("stored job = %+v", stored)
	}
	if r := stored.Results[0]; r.Cause != "" {
		t.Errorf("updated plugin has cause %q", r.Cause)
	}
	if r := stored.Results[1]; r.Status != models.UpdateFailed || r.Cause != wpcli.CauseNoPackage {
		t.Errorf("failed plugin = %+v, want cause %q", r, wpcli.CauseNoPackage)
	}
}
