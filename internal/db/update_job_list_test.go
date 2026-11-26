package db

import (
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// Lists carry what GetUpdateJob would summarize, without the diff and
// output themselves.
func TestListUpdateJobsIsLight(t *testing.T) {
	setupTestAPIDB(t)
	newJob := func() int64 {
		job := models.NewUpdateJob(models.UpdateJobKindPlugins, "s1", models.PluginsPayload{})
		if err := CreateUpdateJob(&job); err != nil {
			t.Fatal(err)
		}
		return job.ID
	}
	withAll, noOutput, emptyOutput := newJob(), newJob(), newJob()
	diff := models.TextDiff{Status: "done", Pages: []models.TextDiffPage{
		{Path: "/", Outcome: models.TextDiffChanged, Hunks: []models.TextDiffHunk{{Lines: []models.TextDiffLine{{Kind: "added", Text: "hello"}}}}},
		{Path: "/a", Outcome: models.TextDiffError},
		{Path: "/b", Outcome: models.TextDiffUnchanged},
	}}
	if err := SetUpdateJobTextDiff(withAll, diff); err != nil {
		t.Fatal(err)
	}
	if err := SetUpdateJobCalls(withAll, []models.WPCLICall{{Args: []string{"plugin", "update"}, Stdout: "Success"}}); err != nil {
		t.Fatal(err)
	}
	if err := SetUpdateJobCalls(emptyOutput, []models.WPCLICall{}); err != nil {
		t.Fatal(err)
	}

	full, err := GetUpdateJob(withAll)
	if err != nil || full.TextDiff == nil || len(full.Calls) != 1 || !full.HasOutput {
		t.Fatalf("full job = %+v, %v", full, err)
	}
	jobs, err := ListUpdateJobs(time.Now().Add(-time.Hour))
	if err != nil || len(jobs) != 3 {
		t.Fatalf("listed %d jobs, %v", len(jobs), err)
	}
	byID := map[int64]models.UpdateJob{}
	for _, j := range jobs {
		byID[j.ID] = j
	}
	listed := byID[withAll]
	if listed.TextDiff != nil || listed.Calls != nil || !listed.HasOutput {
		t.Errorf("listed job carries the diff or output: %+v", listed)
	}
	if listed.TextDiffSummary == nil || *listed.TextDiffSummary != *full.TextDiffSummary {
		t.Errorf("summary = %+v, want %+v", listed.TextDiffSummary, full.TextDiffSummary)
	}
	if byID[noOutput].HasOutput || byID[emptyOutput].HasOutput {
		t.Errorf("HasOutput without output: %+v / %+v", byID[noOutput], byID[emptyOutput])
	}

	diffJobs, err := ListTextDiffJobs(time.Now().Add(-time.Hour))
	if err != nil || len(diffJobs) != 1 || diffJobs[0].TextDiff != nil || diffJobs[0].TextDiffSummary == nil {
		t.Errorf("text diff jobs = %+v, %v", diffJobs, err)
	}
}

// A config change isn't a site's latest update.
func TestLatestLedgerEntrySkipsConfigChanges(t *testing.T) {
	setupTestAPIDB(t)
	for _, e := range []models.SiteUpdateLedgerEntry{
		{SiteID: "s1", UpdateType: "plugin", Status: models.LedgerFull, DataJSON: "{}"},
		{SiteID: "s1", UpdateType: "config", Status: models.LedgerChanged, DataJSON: "{}"},
	} {
		if err := SaveSiteUpdateLedgerEntry(&e); err != nil {
			t.Fatal(err)
		}
	}
	latest, err := GetLatestSiteUpdateLedgerEntries()
	if err != nil || latest["s1"].Status != models.LedgerFull {
		t.Errorf("latest = %+v, %v", latest["s1"], err)
	}
	one, err := GetLatestSiteUpdateLedgerEntry("s1")
	if err != nil || one == nil || one.Status != models.LedgerFull {
		t.Errorf("latest for s1 = %+v, %v", one, err)
	}
}
