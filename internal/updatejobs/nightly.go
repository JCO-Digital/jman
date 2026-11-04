package updatejobs

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/slack"
)

// The nightly core update: once a night, every site running a WordPress
// version with an open, unmuted core vulnerability exposure, and with a
// minor core update available, gets a minor core update job. Major updates
// are never queued. It is off unless nightlyCoreUpdates is set.

// NightlyCreatedBy is the CreatedBy of the nightly core update's jobs; the
// update ledger and the job list show it as the user.
const NightlyCreatedBy = "nightly"

const (
	// nightlyWindow is how long after the start hour a missed run (jman-api
	// was down at the start) still happens.
	nightlyWindow = 3 * time.Hour
	// nightlyCheckInterval is how often the scheduler checks whether a run
	// is due.
	nightlyCheckInterval = 5 * time.Minute
	// nightlyPollInterval and nightlyMaxWait bound waiting for the queued
	// jobs before the Slack summary goes out.
	nightlyPollInterval = 30 * time.Second
	nightlyMaxWait      = 3 * time.Hour
)

// ErrCoreJobActive is returned by EnqueueCore when the site already has a
// core update job queued or running.
var ErrCoreJobActive = errors.New("a WordPress core update is already queued or running for this site")

// coreJobMu serializes the duplicate check and enqueue of core update jobs,
// so two requests (or a request and the nightly run) can't both queue one.
var coreJobMu sync.Mutex

// EnqueueCore queues a core update job unless the site already has one
// queued or running (ErrCoreJobActive).
func EnqueueCore(job *models.UpdateJob) error {
	coreJobMu.Lock()
	defer coreJobMu.Unlock()
	active, err := db.HasActiveCoreUpdateJob(job.SiteID)
	if err != nil {
		return err
	}
	if active {
		return ErrCoreJobActive
	}
	return Enqueue(job)
}

// Seams for tests.
var (
	nightlyNow        = time.Now
	latestNightlyJob  = func() (*models.UpdateJob, error) { return db.LatestUpdateJobBy(NightlyCreatedBy) }
	openExposures     = db.ListOpenVulnExposures
	openMutes         = db.ListOpenVulnMutes
	allSiteCore       = db.GetAllSiteCore
	enqueueCore       = EnqueueCore
	getJob            = db.GetUpdateJob
	sendNightlySlack  = func(msg string) error { return slack.SendMessage(msg, true) }
	nightlyPollPeriod = nightlyPollInterval
)

// StartNightlyCore starts the nightly core update scheduler, if
// nightlyCoreUpdates is on.
func StartNightlyCore(ctx context.Context) {
	if !config.Cfg.NightlyCoreUpdates {
		log.Println("Nightly core updates are off (nightlyCoreUpdates)")
		return
	}
	hour := config.NightlyCoreUpdateStartHour()
	log.Printf("Nightly core updates start at %02d:00 (server time)", hour)
	go func() {
		ticker := time.NewTicker(nightlyCheckInterval)
		defer ticker.Stop()
		for {
			runNightlyCoreIfDue(ctx, hour)
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()
}

// nightlyWindowStart returns the start of the nightly window that now is
// in, and false if it's outside every window. A window may cross midnight
// (a start hour of 22 runs until 01:00).
func nightlyWindowStart(now time.Time, hour int) (time.Time, bool) {
	start := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	if now.Before(start) {
		start = start.AddDate(0, 0, -1)
	}
	return start, now.Sub(start) < nightlyWindow
}

// runNightlyCoreIfDue runs the nightly core update if now is in tonight's
// window and it hasn't queued jobs since the window started.
func runNightlyCoreIfDue(ctx context.Context, hour int) {
	start, ok := nightlyWindowStart(nightlyNow(), hour)
	if !ok {
		return
	}
	last, err := latestNightlyJob()
	if err != nil {
		log.Printf("Nightly core update: %v", err)
		return
	}
	if last != nil && !last.CreatedAt.Before(start) {
		return
	}
	runNightlyCore(ctx)
}

// nightlyCandidate is a site the nightly core update updates.
type nightlyCandidate struct {
	SiteID      string
	Version     string
	MinorUpdate string
}

// nightlyCoreCandidates returns the sites with an open core exposure that
// isn't muted and a minor core update, by site ID.
func nightlyCoreCandidates(exposures []models.VulnExposure, mutes []models.VulnMute, cores []models.SiteCore) []nightlyCandidate {
	muted := map[string]bool{}
	for _, m := range mutes {
		if m.Kind == models.VulnExposureCore && m.Open() {
			muted[m.SiteID] = true
		}
	}
	coreBySite := make(map[string]models.SiteCore, len(cores))
	for _, c := range cores {
		coreBySite[c.SiteID] = c
	}

	var out []nightlyCandidate
	seen := map[string]bool{}
	for _, e := range exposures {
		if e.Kind != models.VulnExposureCore || e.ResolvedAt != nil || muted[e.SiteID] || seen[e.SiteID] {
			continue
		}
		core, ok := coreBySite[e.SiteID]
		if !ok || core.MinorUpdate == "" {
			continue
		}
		seen[e.SiteID] = true
		out = append(out, nightlyCandidate{SiteID: e.SiteID, Version: core.Version, MinorUpdate: core.MinorUpdate})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SiteID < out[j].SiteID })
	return out
}

// runNightlyCore queues tonight's core updates, waits for them and posts
// the Slack summary.
func runNightlyCore(ctx context.Context) {
	exposures, err := openExposures()
	if err != nil {
		log.Printf("Nightly core update: %v", err)
		return
	}
	mutes, err := openMutes()
	if err != nil {
		log.Printf("Nightly core update: %v", err)
		return
	}
	cores, err := allSiteCore()
	if err != nil {
		log.Printf("Nightly core update: %v", err)
		return
	}

	// Only sites jman can run WP-CLI on; a removed site's exposure would
	// otherwise fail every night. Without the site list, all are tried.
	names := siteNames()
	var ids []int64
	for _, c := range nightlyCoreCandidates(exposures, mutes, cores) {
		if names != nil && names[c.SiteID] == "" {
			continue
		}
		job := models.NewUpdateJob(models.UpdateJobKindCore, c.SiteID, models.CorePayload{Target: "minor"})
		job.CreatedBy = NightlyCreatedBy
		switch err := enqueueCore(&job); {
		case errors.Is(err, ErrCoreJobActive):
			// Someone is already updating it.
		case err != nil:
			log.Printf("Nightly core update: failed to queue %s: %v", c.SiteID, err)
		default:
			ids = append(ids, job.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	log.Printf("Nightly core update: queued %d core update(s)", len(ids))

	jobs := waitForJobs(ctx, ids)
	if msg := nightlySummary(jobs, names); msg != "" {
		if err := sendNightlySlack(msg); err != nil {
			log.Printf("Nightly core update: failed to send the Slack summary: %v", err)
		}
	}
}

// waitForJobs polls the jobs until all have finished, nightlyMaxWait has
// passed or ctx is done, and returns their last state.
func waitForJobs(ctx context.Context, ids []int64) []models.UpdateJob {
	deadline := nightlyNow().Add(nightlyMaxWait)
	for {
		jobs := make([]models.UpdateJob, 0, len(ids))
		pending := false
		for _, id := range ids {
			job, err := getJob(id)
			if err != nil || job == nil {
				continue
			}
			jobs = append(jobs, *job)
			if job.Status == models.UpdateJobQueued || job.Status == models.UpdateJobRunning {
				pending = true
			}
		}
		if !pending || !nightlyNow().Before(deadline) {
			return jobs
		}
		select {
		case <-ctx.Done():
			return jobs
		case <-time.After(nightlyPollPeriod):
		}
	}
}

// siteNames returns the names of the sites jman can run WP-CLI on, by ID,
// or nil if the site list can't be read.
var siteNames = func() map[string]string {
	sites, err := cache.GetFastSiteList()
	if err != nil {
		log.Printf("Nightly core update: failed to get the site list: %v", err)
		return nil
	}
	names := make(map[string]string, len(sites))
	for _, s := range sites {
		names[s.ID] = s.Name
	}
	return names
}

// nightlySummary is the Slack message about the nightly core update's
// jobs: which sites were updated, already up to date, failed, or not done
// yet. It's empty when there were no jobs.
func nightlySummary(jobs []models.UpdateJob, names map[string]string) string {
	if len(jobs) == 0 {
		return ""
	}
	name := func(id string) string {
		if n := names[id]; n != "" {
			return n
		}
		return id
	}
	var updated, upToDate, failed, unfinished []string
	for _, job := range jobs {
		site := name(job.SiteID)
		switch {
		case job.Status == models.UpdateJobQueued || job.Status == models.UpdateJobRunning:
			unfinished = append(unfinished, site)
		case job.Status != models.UpdateJobDone || len(job.Results) == 0:
			failed = append(failed, fmt.Sprintf("%s: %s", site, firstNonEmpty(job.Error, job.Status)))
		default:
			r := job.Results[0]
			switch r.Status {
			case models.UpdateUpdated:
				updated = append(updated, fmt.Sprintf("%s: %s → %s", site, r.OldVersion, r.NewVersion))
			case models.UpdateUpToDate:
				upToDate = append(upToDate, site)
			default:
				failed = append(failed, fmt.Sprintf("%s: %s", site, firstNonEmpty(r.Error, r.Status)))
			}
		}
	}

	var b strings.Builder
	icon := "✅"
	if len(failed) > 0 || len(unfinished) > 0 {
		icon = "⚠️"
	}
	fmt.Fprintf(&b, "%s *Nightly WordPress core updates* for core vulnerabilities: %d updated", icon, len(updated))
	if len(failed) > 0 {
		fmt.Fprintf(&b, ", %d failed", len(failed))
	}
	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		sort.Strings(items)
		fmt.Fprintf(&b, "\n*%s*", title)
		for _, it := range items {
			fmt.Fprintf(&b, "\n• %s", it)
		}
	}
	section("Failed", failed)
	section("Updated", updated)
	section("Already up to date", upToDate)
	section("Still running", unfinished)
	return b.String()
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
