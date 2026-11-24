package updatejobs

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// readUpdateLog returns the lines of the update log in the test's data
// directory.
func readUpdateLog(t *testing.T) []attemptLine {
	t.Helper()
	f, err := os.Open(filepath.Join(config.RunData.DataDir, updateLogFile))
	if err != nil {
		t.Fatalf("update log: %v", err)
	}
	defer f.Close()
	var lines []attemptLine
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var l attemptLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatalf("update log line %q: %v", sc.Text(), err)
		}
		lines = append(lines, l)
	}
	return lines
}

// A plugin job's update call goes in the log with the site's PHP and
// WordPress versions. When it fails, or a plugin's version doesn't change
// although wp-cli reported an update, the log and the job get wp-cli's
// whole output, and the job's messages are wp-cli's own warnings.
func TestUpdateLogRecordsAttempts(t *testing.T) {
	setupRunnerTest(t)
	stderr := "PHP Deprecated:  Creation of dynamic property in /srv/x.php on line 3\n" +
		"Warning: Download failed. Unauthorized\n" +
		"Error: No plugins updated (1 failed)."
	updatePlugins = func(_ models.CliSite, plugins []string, rec *[]models.WPCLICall) ([]wpcli.UpdateResult, error) {
		*rec = append(*rec, models.WPCLICall{Args: append([]string{"plugin", "update"}, plugins...), Stderr: stderr, Error: "Error: No plugins updated (1 failed)."})
		return []wpcli.UpdateResult{
			{Name: "a", OldVersion: "1.0", NewVersion: "1.0", Status: "Error", Error: wpcli.Messages(stderr)},
			{Name: "b", OldVersion: "2.0", NewVersion: "2.1", Status: "Updated"},
			{Name: "c", OldVersion: "3.0", NewVersion: "3.1", Status: "Updated"},
		}, &wpcli.UpdateFailure{
			Err:      errors.New("failed to update plugin: " + wpcli.Messages(stderr)),
			Versions: map[string]string{"a": "1.0", "b": "2.0", "c": "3.1"},
		}
	}
	// b's premium updater "succeeded" without installing anything new.
	refreshPluginCache = func(models.CliSite) error {
		setSitePlugins(t,
			models.WPPlugin{Name: "a", Version: "1.0"},
			models.WPPlugin{Name: "b", Version: "2.0"},
			models.WPPlugin{Name: "c", Version: "3.1"},
		)
		return nil
	}

	job := pluginsJob(models.UpdateJobKindPlugins,
		models.PluginUpdateRequest{Name: "a", OldVersion: "1.0"},
		models.PluginUpdateRequest{Name: "b", OldVersion: "2.0"},
		models.PluginUpdateRequest{Name: "c", OldVersion: "3.0"})
	job.CreatedBy = "nightly"
	job = createJob(t, job)
	run(job)

	stored := storedJob(t, job.ID)
	want := "Warning: Download failed. Unauthorized; Error: No plugins updated (1 failed)."
	if stored.Results[0].Error != want || stored.Error != "failed to update plugin: "+want {
		t.Errorf("messages = %q / %q, want wp-cli's warning without PHP's notice", stored.Results[0].Error, stored.Error)
	}
	if b := stored.Results[1]; b.Status != models.UpdateUpdated || b.NewVersion != "2.0" || b.Note == "" {
		t.Errorf("unchanged plugin = %+v, want its old version and a note", b)
	}
	if !stored.HasOutput || len(stored.Calls) != 1 || stored.Calls[0].Stderr != stderr {
		t.Errorf("stored calls = %+v", stored.Calls)
	}

	lines := readUpdateLog(t)
	if len(lines) != 1 {
		t.Fatalf("log lines = %+v, want one for the call", lines)
	}
	l := lines[0]
	if l.JobID != job.ID || l.Type != attemptPlugins || l.Site != "example.com" || l.CreatedBy != "nightly" ||
		l.WPVersion != "6.4.0" || l.PHPVersion != "8.3" || l.CLIPHPVersion != "8.1.2" || !l.Failed || len(l.Calls) != 1 {
		t.Errorf("log line = %+v", l)
	}
	if len(l.Results) != 3 || l.Results[0].Status != models.UpdateFailed || !l.Results[1].VersionUnchanged || l.Results[2].VersionUnchanged {
		t.Errorf("log results = %+v", l.Results)
	}
}

// A successful attempt is logged without its output, and the job keeps
// none.
func TestUpdateLogSuccessWithoutOutput(t *testing.T) {
	setupRunnerTest(t)
	updatePlugins = func(_ models.CliSite, plugins []string, rec *[]models.WPCLICall) ([]wpcli.UpdateResult, error) {
		*rec = append(*rec, models.WPCLICall{Args: []string{"plugin", "update", "a"}, Stdout: "[...]"})
		return []wpcli.UpdateResult{{Name: "a", OldVersion: "1.0", NewVersion: "1.1", Status: "Updated"}}, nil
	}
	refreshPluginCache = func(models.CliSite) error {
		setSitePlugins(t, models.WPPlugin{Name: "a", Version: "1.1"})
		return nil
	}
	job := pluginsJob(models.UpdateJobKindPlugins, models.PluginUpdateRequest{Name: "a", OldVersion: "1.0"})
	job = createJob(t, job)
	run(job)

	if stored := storedJob(t, job.ID); stored.HasOutput || len(stored.Calls) != 0 {
		t.Errorf("job of a successful update kept output: %+v", stored.Calls)
	}
	lines := readUpdateLog(t)
	if len(lines) != 1 || lines[0].Failed || len(lines[0].Calls) != 0 || lines[0].Results[0].NewVersion != "1.1" {
		t.Errorf("log = %+v", lines)
	}
}

// A core update is one attempt with both its calls, and "off" turns the
// log off.
func TestUpdateLogCore(t *testing.T) {
	setupRunnerTest(t)
	updateCore = func(_ models.CliSite, _ bool, rec *[]models.WPCLICall) (wpcli.CoreUpdateResult, error) {
		*rec = append(*rec, models.WPCLICall{Args: []string{"core", "update", "--minor"}}, models.WPCLICall{Args: []string{"core", "update-db"}})
		return wpcli.CoreUpdateResult{Success: true, Version: "6.4.1"}, nil
	}
	// The update reported success, but the version didn't change.
	refreshCore = func(models.CliSite) (*models.SiteCore, error) {
		return &models.SiteCore{SiteID: testSite, Version: "6.4.0"}, nil
	}
	job := models.NewUpdateJob(models.UpdateJobKindCore, testSite, models.CorePayload{Target: "minor"})
	job = createJob(t, job)
	run(job)

	lines := readUpdateLog(t)
	if len(lines) != 1 || lines[0].Type != attemptCore || len(lines[0].Calls) != 2 || !lines[0].Results[0].VersionUnchanged {
		t.Errorf("log = %+v", lines)
	}
	if stored := storedJob(t, job.ID); len(stored.Calls) != 2 || stored.Results[0].NewVersion != "6.4.0" {
		t.Errorf("core job = %+v", stored)
	}

	old := config.Cfg.UpdateLog
	config.Cfg.UpdateLog = "off"
	t.Cleanup(func() { config.Cfg.UpdateLog = old })
	job = models.NewUpdateJob(models.UpdateJobKindCore, testSite, models.CorePayload{Target: "minor"})
	job = createJob(t, job)
	run(job)
	if lines := readUpdateLog(t); len(lines) != 1 {
		t.Errorf("log lines with the log off = %d, want still 1", len(lines))
	}
}
