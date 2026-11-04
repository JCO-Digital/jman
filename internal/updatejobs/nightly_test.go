package updatejobs

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestNightlyWindowStart(t *testing.T) {
	at := func(day, hour, min int) time.Time { return time.Date(2026, 11, day, hour, min, 0, 0, time.Local) }
	cases := []struct {
		name      string
		now       time.Time
		hour      int
		wantStart time.Time
		wantIn    bool
	}{
		{"at the start", at(5, 3, 0), 3, at(5, 3, 0), true},
		{"inside", at(5, 5, 59), 3, at(5, 3, 0), true},
		{"before", at(5, 2, 59), 3, at(4, 3, 0), false},
		{"after", at(5, 6, 0), 3, at(5, 3, 0), false},
		{"across midnight", at(5, 0, 30), 22, at(4, 22, 0), true},
		{"across midnight, over", at(5, 1, 0), 22, at(4, 22, 0), false},
	}
	for _, tc := range cases {
		start, in := nightlyWindowStart(tc.now, tc.hour)
		if in != tc.wantIn || (in && !start.Equal(tc.wantStart)) {
			t.Errorf("%s: nightlyWindowStart(%s, %d) = %s, %v; want %s, %v", tc.name, tc.now.Format("15:04"), tc.hour, start, in, tc.wantStart, tc.wantIn)
		}
	}
}

func TestNightlyCoreCandidates(t *testing.T) {
	now := time.Now()
	core := func(site string) models.VulnExposure {
		return models.VulnExposure{Kind: models.VulnExposureCore, Slug: models.CoreExposureSlug, SiteID: site, Version: "6.8.1"}
	}
	exposures := []models.VulnExposure{
		core("vulnerable"),
		core("muted"),
		core("lifted-mute"),
		core("major-only"),
		core("no-core-data"),
		{Kind: models.VulnExposurePlugin, Slug: "akismet", SiteID: "plugin-only"},
		{Kind: models.VulnExposureCore, Slug: models.CoreExposureSlug, SiteID: "resolved", ResolvedAt: &now},
	}
	mutes := []models.VulnMute{
		{Kind: models.VulnExposureCore, SiteID: "muted", Status: models.VulnMuteNeedsReview},
		{Kind: models.VulnExposureCore, SiteID: "lifted-mute", Status: models.VulnMuteLifted},
		{Kind: models.VulnExposurePlugin, SiteID: "vulnerable", Status: models.VulnMuteActive},
	}
	minor := func(site string) models.SiteCore {
		return models.SiteCore{SiteID: site, Version: "6.8.1", MinorUpdate: "6.8.3"}
	}
	cores := []models.SiteCore{
		minor("vulnerable"), minor("muted"), minor("lifted-mute"), minor("plugin-only"), minor("resolved"),
		{SiteID: "major-only", Version: "6.8.1", MajorUpdate: "6.9"},
	}

	got := nightlyCoreCandidates(exposures, mutes, cores)
	var ids []string
	for _, c := range got {
		ids = append(ids, c.SiteID)
	}
	if strings.Join(ids, ",") != "lifted-mute,vulnerable" {
		t.Errorf("candidates = %v, want lifted-mute,vulnerable", ids)
	}
	if got[1].Version != "6.8.1" || got[1].MinorUpdate != "6.8.3" {
		t.Errorf("candidate = %+v", got[1])
	}
}

// stubNightly replaces the nightly run's seams. Jobs it "queues" finish at
// once as updated; sites in active already have a core job.
func stubNightly(t *testing.T, now time.Time, last *models.UpdateJob, active map[string]bool) (queued *[]models.UpdateJob, sent *[]string) {
	t.Helper()
	oldNow, oldLast, oldExp, oldMutes, oldCore, oldEnqueue, oldGet, oldSlack, oldNames, oldPoll :=
		nightlyNow, latestNightlyJob, openExposures, openMutes, allSiteCore, enqueueCore, getJob, sendNightlySlack, siteNames, nightlyPollPeriod
	t.Cleanup(func() {
		nightlyNow, latestNightlyJob, openExposures, openMutes, allSiteCore, enqueueCore, getJob, sendNightlySlack, siteNames, nightlyPollPeriod =
			oldNow, oldLast, oldExp, oldMutes, oldCore, oldEnqueue, oldGet, oldSlack, oldNames, oldPoll
	})

	jobs := map[int64]*models.UpdateJob{}
	queued, sent = &[]models.UpdateJob{}, &[]string{}
	nightlyNow = func() time.Time { return now }
	nightlyPollPeriod = time.Millisecond
	latestNightlyJob = func() (*models.UpdateJob, error) { return last, nil }
	openExposures = func() ([]models.VulnExposure, error) {
		return []models.VulnExposure{
			{Kind: models.VulnExposureCore, SiteID: "s1"},
			{Kind: models.VulnExposureCore, SiteID: "s2"},
			{Kind: models.VulnExposureCore, SiteID: "removed"},
		}, nil
	}
	openMutes = func() ([]models.VulnMute, error) { return nil, nil }
	allSiteCore = func() ([]models.SiteCore, error) {
		return []models.SiteCore{
			{SiteID: "s1", Version: "6.8.1", MinorUpdate: "6.8.3"},
			{SiteID: "s2", Version: "6.7.1", MinorUpdate: "6.7.4"},
			{SiteID: "removed", Version: "6.7.1", MinorUpdate: "6.7.4"},
		}, nil
	}
	enqueueCore = func(job *models.UpdateJob) error {
		if active[job.SiteID] {
			return ErrCoreJobActive
		}
		job.ID = int64(len(jobs) + 1)
		job.CreatedAt = now
		*queued = append(*queued, *job)
		done := *job
		done.Status = models.UpdateJobDone
		done.Results = []models.UpdateResult{{Name: "WordPress", Status: models.UpdateUpdated, OldVersion: "6.8.1", NewVersion: "6.8.3"}}
		jobs[job.ID] = &done
		return nil
	}
	getJob = func(id int64) (*models.UpdateJob, error) { return jobs[id], nil }
	sendNightlySlack = func(msg string) error { *sent = append(*sent, msg); return nil }
	siteNames = func() map[string]string { return map[string]string{"s1": "one.example.com", "s2": "two.example.com"} }
	return queued, sent
}

func TestRunNightlyCoreIfDue(t *testing.T) {
	night := time.Date(2026, 11, 5, 3, 5, 0, 0, time.Local)

	// "removed" isn't in the site list, so it's left out.
	queued, sent := stubNightly(t, night, nil, nil)
	runNightlyCoreIfDue(context.Background(), 3)
	if len(*queued) != 2 {
		t.Fatalf("queued %d jobs, want 2 (not the removed site)", len(*queued))
	}
	for _, job := range *queued {
		p := jobCore(job)
		if job.Kind != models.UpdateJobKindCore || p.Target != "minor" || p.AllowMajor || job.CreatedBy != NightlyCreatedBy {
			t.Errorf("job = %+v, payload %+v", job, p)
		}
	}
	if len(*sent) != 1 || !strings.Contains((*sent)[0], "2 updated") || !strings.Contains((*sent)[0], "one.example.com: 6.8.1 → 6.8.3") {
		t.Errorf("Slack summary = %q", *sent)
	}

	// Already run tonight: nothing is queued again.
	queued, sent = stubNightly(t, night.Add(time.Hour), &models.UpdateJob{CreatedAt: night}, nil)
	runNightlyCoreIfDue(context.Background(), 3)
	if len(*queued) != 0 || len(*sent) != 0 {
		t.Errorf("ran twice in one night: queued %d, sent %d", len(*queued), len(*sent))
	}

	// Last night's run doesn't count for tonight.
	queued, _ = stubNightly(t, night, &models.UpdateJob{CreatedAt: night.AddDate(0, 0, -1)}, nil)
	runNightlyCoreIfDue(context.Background(), 3)
	if len(*queued) != 2 {
		t.Errorf("queued %d jobs after last night's run, want 2", len(*queued))
	}

	// Outside the window nothing happens.
	queued, _ = stubNightly(t, night.Add(-2*time.Hour), nil, nil)
	runNightlyCoreIfDue(context.Background(), 3)
	if len(*queued) != 0 {
		t.Errorf("queued %d jobs outside the window", len(*queued))
	}

	// A site with a core update already queued is left to it.
	queued, _ = stubNightly(t, night, nil, map[string]bool{"s1": true})
	runNightlyCoreIfDue(context.Background(), 3)
	if len(*queued) != 1 || (*queued)[0].SiteID != "s2" {
		t.Errorf("queued = %+v, want only s2", *queued)
	}

	// No candidates, or all already being updated: no Slack message.
	_, sent = stubNightly(t, night, nil, map[string]bool{"s1": true, "s2": true})
	runNightlyCoreIfDue(context.Background(), 3)
	if len(*sent) != 0 {
		t.Errorf("sent %q with nothing queued", *sent)
	}
}

func TestNightlySummary(t *testing.T) {
	jobs := []models.UpdateJob{
		{SiteID: "s1", Status: models.UpdateJobDone, Results: []models.UpdateResult{{Status: models.UpdateUpdated, OldVersion: "6.8.1", NewVersion: "6.8.3"}}},
		{SiteID: "s2", Status: models.UpdateJobDone, Results: []models.UpdateResult{{Status: models.UpdateFailed, Error: "Error: Could not create directory."}}},
		{SiteID: "s3", Status: models.UpdateJobFailed, Error: "site not reachable via WP-CLI"},
		{SiteID: "s4", Status: models.UpdateJobDone, Results: []models.UpdateResult{{Status: models.UpdateUpToDate}}},
		{SiteID: "s5", Status: models.UpdateJobRunning},
	}
	msg := nightlySummary(jobs, map[string]string{"s1": "one.example.com"})
	for _, want := range []string{
		"⚠️ *Nightly WordPress core updates* for core vulnerabilities: 1 updated, 2 failed",
		"• one.example.com: 6.8.1 → 6.8.3",
		"• s2: Error: Could not create directory.",
		"• s3: site not reachable via WP-CLI",
		"*Already up to date*\n• s4",
		"*Still running*\n• s5",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("summary lacks %q:\n%s", want, msg)
		}
	}
	if nightlySummary(nil, nil) != "" {
		t.Error("summary for no jobs isn't empty")
	}
}
