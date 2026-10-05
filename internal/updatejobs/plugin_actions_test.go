package updatejobs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// setSitePlugins replaces the cached plugin list, as a refresh after a
// change would.
func setSitePlugins(t *testing.T, plugins ...models.WPPlugin) {
	t.Helper()
	if err := db.DeleteSitePlugins(testSite); err != nil {
		t.Fatal(err)
	}
	for _, p := range plugins {
		p.SiteID = testSite
		if err := db.SaveSitePlugin(p); err != nil {
			t.Fatal(err)
		}
	}
}

func createJob(t *testing.T, job models.UpdateJob) models.UpdateJob {
	t.Helper()
	job.SiteID = testSite
	if job.CreatedBy == "" {
		job.CreatedBy = "alice"
	}
	if err := db.CreateUpdateJob(&job); err != nil {
		t.Fatal(err)
	}
	return job
}

func storedJob(t *testing.T, id int64) *models.UpdateJob {
	t.Helper()
	job, err := db.GetUpdateJob(id)
	if err != nil || job == nil {
		t.Fatalf("GetUpdateJob: %v, %v", job, err)
	}
	return job
}

func actionLedger(t *testing.T) (models.SiteUpdateLedgerEntry, map[string]any) {
	t.Helper()
	ledger, err := db.GetSiteUpdateLedger(testSite)
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger) != 1 {
		t.Fatalf("got %d ledger entries, want 1", len(ledger))
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(ledger[0].DataJSON), &data); err != nil {
		t.Fatal(err)
	}
	return ledger[0], data
}

func TestRunPluginActionChecksResultingState(t *testing.T) {
	setupRunnerTest(t)
	var gotAction string
	var gotPlugins []string
	pluginAction = func(_ models.CliSite, action string, plugins []string) error {
		gotAction, gotPlugins = action, plugins
		return errors.New("Error: b failed to activate")
	}
	// After the call, "a" is active but "b" isn't.
	refreshPluginCache = func(models.CliSite) error {
		setSitePlugins(t,
			models.WPPlugin{Name: "a", Status: "active", Version: "1.0"},
			models.WPPlugin{Name: "b", Status: "inactive", Version: "2.0"},
		)
		return nil
	}

	job := createJob(t, models.UpdateJob{Kind: models.UpdateJobKindActivate, Plugins: []models.PluginUpdateRequest{{Name: "a", OldVersion: "1.0"}, {Name: "b", OldVersion: "2.0"}}})
	run(job)

	if gotAction != "activate" || len(gotPlugins) != 2 {
		t.Errorf("action %q on %v, want activate on both plugins in one call", gotAction, gotPlugins)
	}
	stored := storedJob(t, job.ID)
	if stored.Status != models.UpdateJobDone || stored.Error == "" || len(stored.Results) != 2 {
		t.Fatalf("stored job = %+v", stored)
	}
	if stored.Results[0].Status != models.UpdateDone {
		t.Errorf("a = %+v, want done", stored.Results[0])
	}
	if stored.Results[1].Status != models.UpdateFailed || stored.Results[1].Error != "Error: b failed to activate" {
		t.Errorf("b = %+v, want failed with the WP-CLI error", stored.Results[1])
	}

	entry, data := actionLedger(t)
	if entry.Status != "partial" || entry.UpdateType != "plugin" || data["action"] != "activate" {
		t.Errorf("ledger entry = %+v data %v", entry, data)
	}
}

func TestRunPluginActionDelete(t *testing.T) {
	setupRunnerTest(t)
	pluginAction = func(models.CliSite, string, []string) error { return nil }
	refreshPluginCache = func(models.CliSite) error {
		setSitePlugins(t, models.WPPlugin{Name: "other", Status: "active"})
		return nil
	}

	job := createJob(t, models.UpdateJob{Kind: models.UpdateJobKindDelete, Plugins: []models.PluginUpdateRequest{{Name: "a"}}})
	run(job)

	stored := storedJob(t, job.ID)
	if stored.Status != models.UpdateJobDone || stored.Error != "" || stored.Results[0].Status != models.UpdateDone {
		t.Errorf("stored job = %+v", stored)
	}
	if entry, _ := actionLedger(t); entry.Status != models.LedgerDeleted {
		t.Errorf("ledger status = %q, want deleted", entry.Status)
	}
}

func TestRunPluginActionWithoutFreshCacheUsesCallResult(t *testing.T) {
	setupRunnerTest(t)
	pluginAction = func(models.CliSite, string, []string) error { return nil }
	refreshPluginCache = func(models.CliSite) error { return errors.New("ssh down") }

	job := createJob(t, models.UpdateJob{Kind: models.UpdateJobKindDeactivate, Plugins: []models.PluginUpdateRequest{{Name: "a"}}})
	run(job)

	if r := storedJob(t, job.ID).Results[0]; r.Status != models.UpdateDone {
		t.Errorf("result = %+v, want done", r)
	}
}

func TestRunInstallFromSlug(t *testing.T) {
	setupRunnerTest(t)
	var gotSource string
	var gotActivate bool
	installPlugin = func(_ models.CliSite, source string, activate bool) (bool, error) {
		gotSource, gotActivate = source, activate
		return true, nil
	}
	refreshPluginCache = func(models.CliSite) error {
		setSitePlugins(t, models.WPPlugin{Name: "akismet", Status: "active", Version: "5.3"})
		return nil
	}

	job := createJob(t, models.UpdateJob{Kind: models.UpdateJobKindInstall, Source: "akismet", Activate: true})
	run(job)

	if gotSource != "akismet" || !gotActivate {
		t.Errorf("installed %q (activate=%v), want akismet activated", gotSource, gotActivate)
	}
	stored := storedJob(t, job.ID)
	want := []models.UpdateResult{{Name: "akismet", NewVersion: "5.3", Status: models.UpdateDone}}
	assertResults(t, stored.Results, want)

	entry, data := actionLedger(t)
	if entry.Status != models.LedgerInstalled || data["action"] != "install" || data["source"] != "akismet" {
		t.Errorf("ledger entry = %+v data %v", entry, data)
	}
}

func TestRunInstallUploadedZip(t *testing.T) {
	setupRunnerTest(t)
	findSite = func(siteID string) (*models.CliSite, error) {
		return &models.CliSite{ID: siteID, Name: "example.com", SSH: "user@host"}, nil
	}
	if err := os.MkdirAll(UploadDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	upload := filepath.Join(UploadDir(), "abc.zip")
	if err := os.WriteFile(upload, []byte("PK\x03\x04"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A plugin cached before the install isn't mistaken for the new one.
	setSitePlugins(t, models.WPPlugin{Name: "existing", Status: "active"})

	var uploadedTo, installedFrom string
	var removed []string
	uploadFile = func(_, local, remote string) error {
		if local != upload {
			t.Errorf("uploaded %s, want %s", local, upload)
		}
		uploadedTo = remote
		return nil
	}
	runSSH = func(_ string, args ...string) (wpcli.RunResult, error) {
		removed = args
		return wpcli.RunResult{}, nil
	}
	installPlugin = func(_ models.CliSite, source string, _ bool) (bool, error) {
		installedFrom = source
		return true, nil
	}
	refreshPluginCache = func(models.CliSite) error {
		setSitePlugins(t,
			models.WPPlugin{Name: "existing", Status: "active"},
			models.WPPlugin{Name: "my-plugin", Status: "inactive", Version: "1.2"},
		)
		return nil
	}

	job := createJob(t, models.UpdateJob{Kind: models.UpdateJobKindInstall, Source: "my-plugin.zip", UploadPath: upload})
	run(job)

	if installedFrom != uploadedTo || filepath.Ext(uploadedTo) != ".zip" {
		t.Errorf("installed from %q, uploaded to %q", installedFrom, uploadedTo)
	}
	if len(removed) != 3 || removed[2] != uploadedTo {
		t.Errorf("remote cleanup = %v, want rm of %s", removed, uploadedTo)
	}
	if _, err := os.Stat(upload); !os.IsNotExist(err) {
		t.Errorf("local upload still exists (err %v)", err)
	}
	want := []models.UpdateResult{{Name: "my-plugin", NewVersion: "1.2", Status: models.UpdateDone}}
	assertResults(t, storedJob(t, job.ID).Results, want)
}

func TestRunInstallFailure(t *testing.T) {
	setupRunnerTest(t)
	installPlugin = func(models.CliSite, string, bool) (bool, error) {
		return false, errors.New("plugin not found")
	}

	job := createJob(t, models.UpdateJob{Kind: models.UpdateJobKindInstall, Source: "nope"})
	run(job)

	stored := storedJob(t, job.ID)
	if stored.Error != "plugin not found" || stored.Results[0].Status != models.UpdateFailed {
		t.Errorf("stored job = %+v", stored)
	}
	if entry, _ := actionLedger(t); entry.Status != "failed" {
		t.Errorf("ledger status = %q, want failed", entry.Status)
	}
}

func TestStartInterruptsActionAndCleansUploads(t *testing.T) {
	setupRunnerTest(t)
	if err := os.MkdirAll(UploadDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(UploadDir(), "orphan.zip")
	queuedUpload := filepath.Join(UploadDir(), "queued.zip")
	for _, p := range []string{orphan, queuedUpload} {
		if err := os.WriteFile(p, []byte("PK\x03\x04"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	job := createJob(t, models.UpdateJob{Kind: models.UpdateJobKindDeactivate, Plugins: []models.PluginUpdateRequest{{Name: "a"}}})
	if err := db.StartUpdateJob(job.ID); err != nil {
		t.Fatal(err)
	}
	// Queued on another site, so it doesn't run (and remove its upload)
	// before the assertions; findSite blocks it.
	queued := models.UpdateJob{Kind: models.UpdateJobKindInstall, SiteID: "22222222-2222-2222-2222-222222222222", Source: "q.zip", UploadPath: queuedUpload}
	if err := db.CreateUpdateJob(&queued); err != nil {
		t.Fatal(err)
	}
	block := make(chan struct{})
	findSite = func(string) (*models.CliSite, error) {
		<-block
		return nil, errors.New("stopped")
	}
	t.Cleanup(func() { close(block) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := Start(ctx); err != nil {
		t.Fatal(err)
	}

	stored := storedJob(t, job.ID)
	if stored.Status != models.UpdateJobInterrupted || stored.Error != interruptedActionReason {
		t.Errorf("stored job = %+v", stored)
	}
	entry, data := actionLedger(t)
	if entry.Status != "failed" || data["action"] != "deactivate" {
		t.Errorf("ledger entry = %+v data %v", entry, data)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("orphaned upload not removed (err %v)", err)
	}
	if _, err := os.Stat(queuedUpload); err != nil {
		t.Errorf("queued job's upload removed: %v", err)
	}
}

func TestActionLedgerStatuses(t *testing.T) {
	setupRunnerTest(t)
	ok := []models.UpdateResult{{Name: "a", Status: models.UpdateDone}}
	for kind, want := range map[string]string{
		models.UpdateJobKindActivate:   models.LedgerActivated,
		models.UpdateJobKindDeactivate: models.LedgerDeactivated,
		models.UpdateJobKindDelete:     models.LedgerDeleted,
		models.UpdateJobKindUninstall:  models.LedgerDeleted,
		models.UpdateJobKindInstall:    models.LedgerInstalled,
	} {
		if _, err := db.GetAPIDB().Exec(`DELETE FROM site_update_ledger`); err != nil {
			t.Fatal(err)
		}
		writeActionLedger(models.UpdateJob{Kind: kind, SiteID: testSite}, ok, "")
		if entry, _ := actionLedger(t); entry.Status != want {
			t.Errorf("%s: status %q, want %q", kind, entry.Status, want)
		}
	}
}

func TestLatestLedgerEntrySkipsManagementActions(t *testing.T) {
	setupRunnerTest(t)
	save := func(status string) {
		if err := db.SaveSiteUpdateLedgerEntry(&models.SiteUpdateLedgerEntry{SiteID: testSite, UpdateType: "plugin", Status: status, UpdatedBy: "alice"}); err != nil {
			t.Fatal(err)
		}
	}
	save(models.LedgerVuln)
	save(models.LedgerDeactivated)
	save(models.LedgerInstalled)

	latest, err := db.GetLatestSiteUpdateLedgerEntries()
	if err != nil {
		t.Fatal(err)
	}
	if got := latest[testSite].Status; got != models.LedgerVuln {
		t.Errorf("latest status = %q, want the vuln update", got)
	}
	one, err := db.GetLatestSiteUpdateLedgerEntry(testSite)
	if err != nil || one == nil || one.Status != models.LedgerVuln {
		t.Errorf("GetLatestSiteUpdateLedgerEntry = %+v, %v", one, err)
	}
}
