package updatejobs

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
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
	q = newQueue()
	pluginVulnerable = func(string, string, string) bool { return false }
	pluginFixedIn = func(string, string, string) string { return "" }
	updatePackages = func(models.CliSite) (map[string]string, error) { return map[string]string{}, nil }
	cachedCoreVersion = func(string) string { return "6.4.0" }
	findSite = func(siteID string) (*models.CliSite, error) {
		return &models.CliSite{ID: siteID, Name: "example.com"}, nil
	}
	refreshPluginCache = func(models.CliSite) error { return nil }
	t.Cleanup(func() {
		// Stop any runner a test started (its context is already cancelled)
		// and wait for it before restoring the package state it reads.
		q.close()
		running.Wait()
		findSite, updatePlugins, refreshPluginCache, q = oldFind, oldUpdate, oldRefresh, oldQueue
		updateCore, refreshCore, cachedCoreVersion = oldUpdateCore, oldRefreshCore, oldCachedCore
		pluginAction, installPlugin, uploadFile, runSSH = oldAction, oldInstall, oldUpload, oldSSH
		pluginVulnerable, pluginFixedIn = oldVulnerable, oldFixedIn
		updatePluginsPatch, updatePackages = oldPatch, oldPackages
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
	updatePlugins = func(_ models.CliSite, plugins []string) ([]wpcli.UpdateResult, error) {
		gotPlugins = plugins
		return []wpcli.UpdateResult{
			{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: "Updated"},
			{Name: "b", OldVersion: "2.0", NewVersion: "2.1", Status: "Updated"},
		}, nil
	}

	job := models.UpdateJob{SiteID: testSite, CreatedBy: "alice", Plugins: []models.PluginUpdateRequest{{Name: "a", OldVersion: "1.0"}, {Name: "b", OldVersion: "2.0"}}}
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
	updatePlugins = func(models.CliSite, []string) ([]wpcli.UpdateResult, error) {
		t.Fatal("update must not run for an unreachable site")
		return nil, nil
	}

	job := models.UpdateJob{SiteID: testSite, Plugins: []models.PluginUpdateRequest{{Name: "a", OldVersion: "1.0"}}}
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

	running := models.UpdateJob{SiteID: testSite, Plugins: []models.PluginUpdateRequest{{Name: "a"}}}
	queued := models.UpdateJob{SiteID: "22222222-2222-2222-2222-222222222222", Plugins: []models.PluginUpdateRequest{{Name: "b"}}}
	for _, j := range []*models.UpdateJob{&running, &queued} {
		if err := db.CreateUpdateJob(j); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.StartUpdateJob(running.ID); err != nil {
		t.Fatal(err)
	}

	ran := make(chan string, 2)
	updatePlugins = func(site models.CliSite, plugins []string) ([]wpcli.UpdateResult, error) {
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

func TestQueueRunsOneJobPerSiteAtATime(t *testing.T) {
	qu := newQueue()
	defer qu.close()
	qu.push(models.UpdateJob{ID: 1, SiteID: "s1"})
	qu.push(models.UpdateJob{ID: 2, SiteID: "s1"})
	qu.push(models.UpdateJob{ID: 3, SiteID: "s2"})

	first, _ := qu.next()
	second, _ := qu.next()
	if first.ID != 1 || second.ID != 3 {
		t.Fatalf("got jobs %d, %d; want 1 then 3 (job 2 waits for site s1)", first.ID, second.ID)
	}

	got := make(chan int64, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		j, _ := qu.next()
		got <- j.ID
	}()
	select {
	case id := <-got:
		t.Fatalf("job %d started while site s1 was busy", id)
	case <-time.After(100 * time.Millisecond):
	}
	qu.done("s1")
	select {
	case id := <-got:
		if id != 2 {
			t.Errorf("got job %d, want 2", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("job 2 did not start after site s1 was released")
	}
	wg.Wait()
}

func runCoreJob(t *testing.T, target string) *models.UpdateJob {
	t.Helper()
	job := models.UpdateJob{Kind: models.UpdateJobKindCore, SiteID: testSite, Target: target, CreatedBy: "alice"}
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
	updateCore = func(_ models.CliSite, major bool) (wpcli.CoreUpdateResult, error) {
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
	if stored.Status != models.UpdateJobDone || stored.Target != "major" || stored.Core == nil || stored.Core.Version != "6.5.0" {
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
	updateCore = func(models.CliSite, bool) (wpcli.CoreUpdateResult, error) {
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
	updateCore = func(models.CliSite, bool) (wpcli.CoreUpdateResult, error) {
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
	job := models.UpdateJob{Kind: models.UpdateJobKindCore, SiteID: testSite, Target: "minor"}
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
	updatePlugins = func(models.CliSite, []string) ([]wpcli.UpdateResult, error) {
		return []wpcli.UpdateResult{{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: "Updated"}}, nil
	}
	refreshPluginCache = func(models.CliSite) error {
		setSitePlugins(t,
			models.WPPlugin{Name: "a", Version: "1.1"},
			models.WPPlugin{Name: "b", Version: "2.0", Update: "2.1"},
		)
		return nil
	}

	job := models.UpdateJob{SiteID: testSite, CreatedBy: "alice", Plugins: []models.PluginUpdateRequest{{Name: "a", OldVersion: "1.0"}}}
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)

	ledger, _ := db.GetSiteUpdateLedger(testSite)
	if len(ledger) != 1 || ledger[0].Status != models.LedgerVuln {
		t.Errorf("ledger = %+v, want one vuln entry", ledger)
	}
}
