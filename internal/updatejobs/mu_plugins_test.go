package updatejobs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

func muJob(t *testing.T, upload string) models.UpdateJob {
	t.Helper()
	job := models.NewUpdateJob(models.UpdateJobKindMuInstall, "", models.MuInstallPayload{File: "bojaco.php", Source: "https://example/bojaco.php", SHA256: "aa11"})
	job.UploadPath = upload
	return createJob(t, job)
}

func writeUpload(t *testing.T, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(UploadDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(UploadDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunMuInstallOverSSH(t *testing.T) {
	setupRunnerTest(t)
	findSite = func(siteID string) (*models.CliSite, error) {
		return &models.CliSite{ID: siteID, Name: "example.com", SSH: "user@host", Path: "/sites/example.com/files"}, nil
	}
	upload := writeUpload(t, "mu-1.php", "<?php // bojaco")
	var uploadedTo string
	var sshCmds [][]string
	uploadFile = func(_, local, remote string) error {
		if local != upload {
			t.Errorf("uploaded %s, want %s", local, upload)
		}
		uploadedTo = remote
		return nil
	}
	runSSH = func(_ string, args ...string) (wpcli.RunResult, error) {
		sshCmds = append(sshCmds, args)
		return wpcli.RunResult{}, nil
	}
	refreshed := false
	refreshPluginCache = func(models.CliSite) error { refreshed = true; return nil }

	job := muJob(t, upload)
	run(job)

	dir := "/sites/example.com/files/wp-content/mu-plugins"
	tmp := dir + "/.bojaco.php.jman-" + strconv.FormatInt(job.ID, 10)
	if uploadedTo != tmp {
		t.Errorf("uploaded to %s, want %s", uploadedTo, tmp)
	}
	want := [][]string{{"mkdir", "-p", dir}, {"mv", "-f", tmp, dir + "/bojaco.php"}}
	if !reflect.DeepEqual(sshCmds, want) {
		t.Errorf("ssh commands = %v, want %v", sshCmds, want)
	}
	if !refreshed {
		t.Error("plugin cache not refreshed")
	}
	if _, err := os.Stat(upload); !os.IsNotExist(err) {
		t.Errorf("upload still exists (err %v)", err)
	}
	stored := storedJob(t, job.ID)
	if stored.Status != models.UpdateJobDone {
		t.Errorf("status %s", stored.Status)
	}
	assertResults(t, stored.Results, []models.UpdateResult{{Name: "bojaco.php", Status: models.UpdateDone}})
	entry, data := actionLedger(t)
	if entry.UpdateType != "mu-plugin" || entry.Status != models.LedgerInstalled || data["file"] != "bojaco.php" || data["sha256"] != "aa11" {
		t.Errorf("ledger = %+v %v", entry, data)
	}
}

func TestRunMuInstallMoveFails(t *testing.T) {
	setupRunnerTest(t)
	findSite = func(siteID string) (*models.CliSite, error) {
		return &models.CliSite{ID: siteID, Name: "example.com", SSH: "user@host", Path: "/s"}, nil
	}
	upload := writeUpload(t, "mu-2.php", "<?php")
	uploadFile = func(string, string, string) error { return nil }
	var sshCmds []string
	runSSH = func(_ string, args ...string) (wpcli.RunResult, error) {
		sshCmds = append(sshCmds, args[0])
		if args[0] == "mv" {
			return wpcli.RunResult{Error: "denied"}, errors.New("exit 1")
		}
		return wpcli.RunResult{}, nil
	}

	job := muJob(t, upload)
	run(job)

	if !reflect.DeepEqual(sshCmds, []string{"mkdir", "mv", "rm"}) {
		t.Errorf("ssh commands = %v", sshCmds)
	}
	stored := storedJob(t, job.ID)
	if len(stored.Results) != 1 || stored.Results[0].Status != models.UpdateFailed || !strings.Contains(stored.Results[0].Error, "denied") {
		t.Errorf("results = %+v", stored.Results)
	}
	if entry, _ := actionLedger(t); entry.Status != models.LedgerFailed {
		t.Errorf("ledger status %s", entry.Status)
	}
}

func TestRunMuInstallLocal(t *testing.T) {
	setupRunnerTest(t)
	root := t.TempDir()
	findSite = func(siteID string) (*models.CliSite, error) {
		return &models.CliSite{ID: siteID, Name: "local", Path: filepath.ToSlash(root)}, nil
	}
	upload := writeUpload(t, "mu-3.php", "<?php // local")
	run(muJob(t, upload))

	got, err := os.ReadFile(filepath.Join(root, "wp-content", "mu-plugins", "bojaco.php"))
	if err != nil || string(got) != "<?php // local" {
		t.Errorf("installed file = %q, %v", got, err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "wp-content", "mu-plugins"))
	if len(entries) != 1 {
		t.Errorf("mu-plugins holds %d files, want 1", len(entries))
	}
}

func TestRunMuInstallRejectsBadFile(t *testing.T) {
	setupRunnerTest(t)
	job := models.NewUpdateJob(models.UpdateJobKindMuInstall, "", models.MuInstallPayload{File: "../wp-config.php"})
	job.UploadPath = writeUpload(t, "mu-4.php", "<?php")
	job = createJob(t, job)
	runSSH = func(string, ...string) (wpcli.RunResult, error) {
		t.Error("ran an SSH command for an invalid file name")
		return wpcli.RunResult{}, nil
	}
	run(job)
	if stored := storedJob(t, job.ID); stored.Status != models.UpdateJobFailed {
		t.Errorf("status %s", stored.Status)
	}
}

func TestStartInterruptsMuInstall(t *testing.T) {
	setupRunnerTest(t)
	job := muJob(t, "")
	if err := db.StartUpdateJob(job.ID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Start(ctx); err != nil {
		t.Fatal(err)
	}
	stored := storedJob(t, job.ID)
	if stored.Status != models.UpdateJobInterrupted || len(stored.Results) != 1 || stored.Results[0].Name != "bojaco.php" {
		t.Errorf("job = %+v", stored)
	}
	if entry, _ := actionLedger(t); entry.UpdateType != "mu-plugin" || entry.Status != models.LedgerFailed {
		t.Errorf("ledger = %+v", entry)
	}
}
