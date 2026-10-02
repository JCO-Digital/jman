package pluginupdates

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
	q = newQueue()
	findSite = func(siteID string) (*models.CliSite, error) {
		return &models.CliSite{ID: siteID, Name: "example.com"}, nil
	}
	refreshPluginCache = func(models.CliSite) error { return nil }
	t.Cleanup(func() {
		q.close()
		findSite, updatePlugins, refreshPluginCache, q = oldFind, oldUpdate, oldRefresh, oldQueue
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
		want := []models.PluginUpdateResult{
			{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: models.PluginUpdateUpdated},
			{Name: "b", OldVersion: "2.0", NewVersion: "2.0", Status: models.PluginUpdateUpToDate},
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
		want := []models.PluginUpdateResult{
			// Reported by WP-CLI before the failure.
			{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: models.PluginUpdateUpdated},
			// Not reported, but the check found the new version installed.
			{Name: "b", OldVersion: "2.0", NewVersion: "2.1", Status: models.PluginUpdateUpdated},
			// Not reported and unchanged.
			{Name: "c", OldVersion: "3.0", NewVersion: "3.0", Status: models.PluginUpdateFailed, Error: failure.Error()},
			// Reported as failed by WP-CLI.
			{Name: "d", OldVersion: "4.0", NewVersion: "4.0", Status: models.PluginUpdateFailed, Error: "Error"},
		}
		assertResults(t, got, want)
	})

	t.Run("unknown old version is not assumed updated", func(t *testing.T) {
		failure := &wpcli.UpdateFailure{Err: errors.New("boom"), Versions: map[string]string{"x": "9.9"}}
		got := buildResults([]models.PluginUpdateRequest{{Name: "x"}}, nil, failure)
		if got[0].Status != models.PluginUpdateFailed || got[0].NewVersion != "9.9" {
			t.Errorf("got %+v, want failed with the observed version", got[0])
		}
	})
}

func assertResults(t *testing.T, got, want []models.PluginUpdateResult) {
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

	job := models.PluginUpdateJob{SiteID: testSite, CreatedBy: "alice", Plugins: []models.PluginUpdateRequest{{Name: "a", OldVersion: "1.0"}, {Name: "b", OldVersion: "2.0"}}}
	if err := db.CreatePluginUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)

	if len(gotPlugins) != 2 {
		t.Errorf("plugins updated in one call = %v, want both", gotPlugins)
	}
	stored, err := db.GetPluginUpdateJob(job.ID)
	if err != nil || stored == nil {
		t.Fatalf("GetPluginUpdateJob: %v, %v", stored, err)
	}
	if stored.Status != models.PluginUpdateJobDone || stored.StartedAt == nil || stored.FinishedAt == nil || len(stored.Results) != 2 {
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

	job := models.PluginUpdateJob{SiteID: testSite, Plugins: []models.PluginUpdateRequest{{Name: "a", OldVersion: "1.0"}}}
	if err := db.CreatePluginUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	run(job)

	stored, _ := db.GetPluginUpdateJob(job.ID)
	if stored.Status != models.PluginUpdateJobFailed || stored.Error != "site gone" || stored.Results[0].Status != models.PluginUpdateFailed {
		t.Errorf("stored job = %+v", stored)
	}
	ledger, _ := db.GetSiteUpdateLedger(testSite)
	if len(ledger) != 1 || ledger[0].Status != "failed" {
		t.Errorf("ledger = %+v, want one failed entry", ledger)
	}
}

func TestStartInterruptsRunningAndResumesQueued(t *testing.T) {
	setupRunnerTest(t)

	running := models.PluginUpdateJob{SiteID: testSite, Plugins: []models.PluginUpdateRequest{{Name: "a"}}}
	queued := models.PluginUpdateJob{SiteID: "22222222-2222-2222-2222-222222222222", Plugins: []models.PluginUpdateRequest{{Name: "b"}}}
	for _, j := range []*models.PluginUpdateJob{&running, &queued} {
		if err := db.CreatePluginUpdateJob(j); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.StartPluginUpdateJob(running.ID); err != nil {
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

	stored, _ := db.GetPluginUpdateJob(running.ID)
	if stored.Status != models.PluginUpdateJobInterrupted {
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
	qu.push(models.PluginUpdateJob{ID: 1, SiteID: "s1"})
	qu.push(models.PluginUpdateJob{ID: 2, SiteID: "s1"})
	qu.push(models.PluginUpdateJob{ID: 3, SiteID: "s2"})

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
