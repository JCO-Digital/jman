package updatejobs

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

func stubFileMods(t *testing.T, fn func(models.CliSite, bool, *[]models.WPCLICall) error) {
	t.Helper()
	old := setFileMods
	setFileMods = fn
	t.Cleanup(func() { setFileMods = old })
}

func fileModsJob(t *testing.T, disallow bool) models.UpdateJob {
	t.Helper()
	return createJob(t, models.NewUpdateJob(models.UpdateJobKindFileMods, "", models.FileModsPayload{Disallow: disallow}))
}

func TestRunFileMods(t *testing.T) {
	setupRunnerTest(t)
	auto := "default"
	if err := db.SetSiteWpFlags(testSite, false, false, &auto); err != nil {
		t.Fatal(err)
	}
	var got []bool
	stubFileMods(t, func(site models.CliSite, disallow bool, rec *[]models.WPCLICall) error {
		got = append(got, disallow)
		*rec = append(*rec, models.WPCLICall{Args: []string{"config", "set"}, Stdout: "Success"})
		return nil
	})

	job := fileModsJob(t, true)
	run(job)

	if !slices.Equal(got, []bool{true}) {
		t.Errorf("set DISALLOW_FILE_MODS to %v, want [true]", got)
	}
	stored := storedJob(t, job.ID)
	if stored.Status != models.UpdateJobDone || len(stored.Calls) != 1 {
		t.Errorf("job = %+v", stored)
	}
	assertResults(t, stored.Results, []models.UpdateResult{{Name: fileModsName, Status: models.UpdateDone, Note: "Set to true"}})
	flags, err := db.GetAllSiteWpFlags()
	if err != nil || !flags[testSite].DisallowFileMods {
		t.Errorf("stored flags = %+v, %v", flags[testSite], err)
	}
	entry, data := actionLedger(t)
	if entry.UpdateType != "config" || entry.Status != models.LedgerChanged || data["disallow_file_mods"] != true {
		t.Errorf("ledger = %+v %v", entry, data)
	}
}

func TestRunFileModsFails(t *testing.T) {
	setupRunnerTest(t)
	stubFileMods(t, func(models.CliSite, bool, *[]models.WPCLICall) error {
		return errors.New("Error: Can't find wp-config.php")
	})

	job := fileModsJob(t, false)
	run(job)

	stored := storedJob(t, job.ID)
	if len(stored.Results) != 1 || stored.Results[0].Status != models.UpdateFailed || stored.Error == "" {
		t.Errorf("job = %+v", stored)
	}
	if entry, data := actionLedger(t); entry.Status != models.LedgerFailed || data["error"] == nil {
		t.Errorf("ledger = %+v %v", entry, data)
	}
}

func TestStartInterruptsFileMods(t *testing.T) {
	setupRunnerTest(t)
	job := fileModsJob(t, true)
	if err := db.StartUpdateJob(job.ID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Start(ctx); err != nil {
		t.Fatal(err)
	}
	stored := storedJob(t, job.ID)
	if stored.Status != models.UpdateJobInterrupted || len(stored.Results) != 1 || stored.Results[0].Name != fileModsName {
		t.Errorf("job = %+v", stored)
	}
	if entry, _ := actionLedger(t); entry.UpdateType != "config" || entry.Status != models.LedgerFailed {
		t.Errorf("ledger = %+v", entry)
	}
}
