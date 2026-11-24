// Package updatejobs runs site updates as background jobs inside jman-api.
// A plugins job updates one or more plugins on one site with a single
// `wp plugin update` call; a core job runs `wp core update`; other jobs
// manage plugins or WordPress users. Jobs for
// different sites run concurrently, jobs for the same site one at a time.
// Jobs live in api.db, so their state survives page reloads and is visible
// to every user, and each finished job writes one entry to the site update
// ledger.
package updatejobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/sitestate"
	"github.com/JCO-Digital/jman/internal/vuln"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// Workers is how many jobs (on different sites) run at once.
const Workers = 4

// jobRetention is how long finished jobs are kept; their outcome stays in
// the site update ledger.
const jobRetention = 7 * 24 * time.Hour

const interruptedReason = "jman-api restarted while this update was running; check the site's plugin versions"

const interruptedCoreReason = "jman-api restarted while this update was running; check the site's WordPress version"

// coreResultName is the UpdateResult name used for core jobs.
const coreResultName = "WordPress"

// Seams for tests.
var (
	findSite = func(siteID string) (*models.CliSite, error) {
		sites, err := cache.GetFastSiteList()
		if err != nil {
			return nil, fmt.Errorf("failed to get site list: %w", err)
		}
		for _, s := range sites {
			if s.ID == siteID {
				return &s, nil
			}
		}
		return nil, fmt.Errorf("site %s not found or not reachable via WP-CLI", siteID)
	}
	updatePlugins      = wpcli.UpdatePlugin
	updatePluginsPatch = wpcli.UpdatePluginPatch
	updatePackages     = wpcli.PluginUpdatePackages
	siteLocks          = db.GetSiteLocks
	refreshPluginCache = cache.UpdateSitePluginCache
	updateCore         = wpcli.UpdateCore
	// refreshCore is the refresh after a core job, which records its own
	// ledger entry (see sitestate.SourceJob).
	refreshCore = func(site models.CliSite) (*models.SiteCore, error) {
		return cache.RefreshSiteCore(site, sitestate.SourceJob)
	}
	// pluginVulnerable reports whether a plugin version on a site has an
	// unsuppressed known vulnerability.
	pluginVulnerable = func(siteID, plugin, version string) bool {
		matcher, err := db.NewVulnIgnoreMatcher()
		if err != nil {
			log.Printf("Failed to load vulnerability ignore entries: %v", err)
		}
		return vuln.IsPluginVulnerableOnSite(siteID, plugin, version, matcher)
	}
	// pluginFixedIn returns the version the vulnerability database records
	// as fixing a plugin version's known vulnerabilities on a site, or "".
	pluginFixedIn = func(siteID, plugin, version string) string {
		matcher, err := db.NewVulnIgnoreMatcher()
		if err != nil {
			log.Printf("Failed to load vulnerability ignore entries: %v", err)
		}
		return vuln.PluginFixedInOnSite(siteID, plugin, version, matcher)
	}
	// syncSiteExposures re-evaluates a site's vulnerability exposures
	// (Monitoring) after a job changed its plugins or core.
	syncSiteExposures = vuln.SyncSiteVulnExposures
	cachedCoreVersion = func(siteID string) string {
		cores, err := db.GetAllSiteCore()
		if err != nil {
			return ""
		}
		for _, c := range cores {
			if c.SiteID == siteID {
				return c.Version
			}
		}
		return ""
	}
)

// queue holds the IDs of jobs waiting to run and the sites with a job in
// progress.
type queue struct {
	mu      sync.Mutex
	cond    *sync.Cond
	pending []models.UpdateJob
	busy    map[string]bool
	closed  bool
}

var q = newQueue()

// running tracks the goroutines started by Start, so tests can stop the
// runner and wait for it before swapping package state.
var running sync.WaitGroup

func newQueue() *queue {
	qu := &queue{busy: map[string]bool{}}
	qu.cond = sync.NewCond(&qu.mu)
	return qu
}

func (qu *queue) push(job models.UpdateJob) {
	qu.mu.Lock()
	qu.pending = append(qu.pending, job)
	qu.mu.Unlock()
	qu.cond.Broadcast()
}

// next blocks until a job whose site isn't busy is available, marks that
// site busy and returns the job. ok is false once the queue is closed.
func (qu *queue) next() (job models.UpdateJob, ok bool) {
	qu.mu.Lock()
	defer qu.mu.Unlock()
	for {
		if qu.closed {
			return job, false
		}
		for i, j := range qu.pending {
			if qu.busy[j.SiteID] {
				continue
			}
			qu.pending = append(qu.pending[:i], qu.pending[i+1:]...)
			qu.busy[j.SiteID] = true
			return j, true
		}
		qu.cond.Wait()
	}
}

func (qu *queue) done(siteID string) {
	qu.mu.Lock()
	delete(qu.busy, siteID)
	qu.mu.Unlock()
	qu.cond.Broadcast()
}

func (qu *queue) close() {
	qu.mu.Lock()
	qu.closed = true
	qu.mu.Unlock()
	qu.cond.Broadcast()
}

// Start recovers jobs left by a previous process and starts the workers.
// Jobs that were running are marked interrupted (their outcome is
// unknown); jobs that were still queued run again.
func Start(ctx context.Context) error {
	interrupted, err := db.InterruptRunningUpdateJobs(interruptedReason)
	if err != nil {
		return fmt.Errorf("failed to recover update jobs: %w", err)
	}
	for _, job := range interrupted {
		switch {
		case job.Kind == models.UpdateJobKindCore:
			// The stored reason mentions plugin versions; core jobs get
			// their own wording in the ledger and the job.
			if err := db.FinishUpdateJob(job.ID, models.UpdateJobInterrupted, nil, nil, interruptedCoreReason); err != nil {
				log.Printf("Update job %d: failed to record interruption: %v", job.ID, err)
			}
			writeCoreLedger(job, "", interruptedCoreReason, "failed", "")
		case models.IsUserKind(job.Kind):
			r := models.UpdateResult{Name: userLogin(job), Status: models.UpdateFailed, Error: interruptedUserReason}
			if err := db.FinishUpdateJob(job.ID, models.UpdateJobInterrupted, []models.UpdateResult{r}, nil, interruptedUserReason); err != nil {
				log.Printf("Update job %d: failed to record interruption: %v", job.ID, err)
			}
			writeUserLedger(job, r, interruptedUserReason)
		case job.Kind == models.UpdateJobKindMuInstall:
			r := models.UpdateResult{Name: jobMuInstall(job).File, Status: models.UpdateFailed, Error: interruptedMuReason}
			if err := db.FinishUpdateJob(job.ID, models.UpdateJobInterrupted, []models.UpdateResult{r}, nil, interruptedMuReason); err != nil {
				log.Printf("Update job %d: failed to record interruption: %v", job.ID, err)
			}
			writeMuLedger(job, r, interruptedMuReason)
		case isPluginManagementJob(job):
			if err := db.FinishUpdateJob(job.ID, models.UpdateJobInterrupted, nil, nil, interruptedActionReason); err != nil {
				log.Printf("Update job %d: failed to record interruption: %v", job.ID, err)
			}
			writeActionLedger(job, failAll(actionTargets(job), interruptedActionReason), interruptedActionReason)
		default:
			writeLedger(job, nil, "failed", interruptedReason)
		}
	}
	if err := db.InterruptRunningTextDiffs(); err != nil {
		log.Printf("Failed to recover update text diffs: %v", err)
	}
	// Uploads of interrupted jobs (now finished) are no longer needed.
	cleanupUploads()

	queued, err := db.ListQueuedUpdateJobs()
	if err != nil {
		return fmt.Errorf("failed to load queued update jobs: %w", err)
	}
	qu := q
	for _, job := range queued {
		qu.push(job)
	}

	for range Workers {
		running.Add(1)
		go func() {
			defer running.Done()
			for {
				job, ok := qu.next()
				if !ok {
					return
				}
				run(job)
				qu.done(job.SiteID)
			}
		}()
	}

	running.Add(1)
	go func() {
		defer running.Done()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			if err := db.PruneFinishedUpdateJobs(time.Now().Add(-jobRetention)); err != nil {
				log.Printf("Failed to prune update jobs: %v", err)
			}
			SweepStagedZips()
			select {
			case <-ctx.Done():
				qu.close()
				return
			case <-ticker.C:
			}
		}
	}()

	log.Printf("Update job runner started (%d workers, %d queued jobs, %d interrupted)", Workers, len(queued), len(interrupted))
	return nil
}

// Enqueue stores a new job and queues it to run.
func Enqueue(job *models.UpdateJob) error {
	if err := db.CreateUpdateJob(job); err != nil {
		return err
	}
	q.push(*job)
	return nil
}

// EnqueueAll stores jobs and then queues them, so no job runs (and, for
// jobs sharing an uploaded ZIP, removes it) before all of them are stored.
// On error, the jobs stored before it are still queued; n is how many.
func EnqueueAll(jobs []models.UpdateJob) (n int, err error) {
	for i := range jobs {
		if err = db.CreateUpdateJob(&jobs[i]); err != nil {
			break
		}
		n++
	}
	for _, job := range jobs[:n] {
		q.push(job)
	}
	return n, err
}

// run executes one job and records its outcome.
func run(job models.UpdateJob) {
	if err := db.StartUpdateJob(job.ID); err != nil {
		log.Printf("Update job %d: failed to mark running: %v", job.ID, err)
	}

	if job.UploadPath != "" {
		defer removeUpload(job)
	}
	// A user create job's password is dropped once it has run, whatever
	// the outcome.
	defer forgetPassword(job.ID)

	site, err := findSite(job.SiteID)
	if err == nil {
		err = checkPayload(job)
	}
	if err != nil {
		switch {
		case job.Kind == models.UpdateJobKindCore:
			finishCore(job, models.UpdateJobFailed, failedCoreResult("", err.Error()), nil, err.Error())
			writeCoreLedger(job, "", err.Error(), "failed", "")
		case models.IsUserKind(job.Kind):
			failUserJob(job, err.Error())
		case job.Kind == models.UpdateJobKindMuInstall:
			failMuInstall(job, err.Error())
		case isPluginManagementJob(job):
			results := failAll(actionTargets(job), err.Error())
			finishAction(job, models.UpdateJobFailed, results, err.Error())
			writeActionLedger(job, results, err.Error())
		default:
			finish(job, models.UpdateJobFailed, failAll(jobPlugins(job), err.Error()), err.Error())
		}
		return
	}

	var baseline *textBaseline
	if runsTextDiff(job) {
		baseline = takeTextBaseline(job, *site)
	}

	switch {
	case job.Kind == models.UpdateJobKindCore:
		runCore(job, *site)
	case models.IsPluginActionKind(job.Kind):
		runPluginAction(job, *site)
	case job.Kind == models.UpdateJobKindInstall:
		runInstall(job, *site)
	case job.Kind == models.UpdateJobKindMuInstall:
		runMuInstall(job, *site)
	case job.Kind == models.UpdateJobKindUserCreate:
		runUserCreate(job, *site)
		return
	case job.Kind == models.UpdateJobKindUserDelete:
		runUserDelete(job, *site)
		return
	default:
		runPlugins(job, *site)
	}

	if baseline != nil {
		compareText(job, baseline)
	}

	// The job refreshed the site's plugin (or core) cache: bring its
	// exposures in line now rather than at the next full sync. The job is
	// already finished, so this only delays the worker's next job.
	if err := syncSiteExposures(job.SiteID); err != nil {
		log.Printf("Update job %d: failed to sync vulnerability exposures for site %s: %v", job.ID, job.SiteID, err)
	}
}

// runPlugins updates the job's plugins. Unlocked plugins are updated with
// one WP-CLI call; locked plugins (unless the job allows major updates)
// with a second, patch-only call, and only if their pending update comes
// from WordPress.org: wp-cli looks fix releases up there by slug, which
// would replace a custom plugin that shares a WordPress.org plugin's slug.
// Locks are read here rather than when the job was queued, so a lock added
// in between still applies.
func runPlugins(job models.UpdateJob, site models.CliSite) {
	payload := jobPluginsPayload(job)
	plugins := payload.Plugins
	// Checked against the pre-update versions, for the ledger status.
	updatedVulnerable := anyVulnerable(job.SiteID, plugins)
	wpVersion := cachedCoreVersion(job.SiteID)

	var free, locked []models.PluginUpdateRequest
	if payload.AllowMajor {
		free = plugins
	} else {
		locks, err := siteLocks(job.SiteID)
		if err != nil {
			// Without the locks it's unknown which plugins may get major
			// updates, so update none.
			msg := fmt.Sprintf("failed to load update locks: %v", err)
			finish(job, models.UpdateJobFailed, failAll(plugins, msg), msg)
			return
		}
		for _, p := range plugins {
			if locks.PluginLocked(p.Name) {
				locked = append(locked, p)
			} else {
				free = append(free, p)
			}
		}
	}

	var errs []string
	var attempts []*attempt
	// The plugins each attempt's update command covered.
	covered := map[*attempt][]string{}
	byName := map[string]models.UpdateResult{}
	if len(free) > 0 {
		a := newAttempt(attemptPlugins)
		covered[a] = requestNames(free)
		updates, err := updatePlugins(site, covered[a], &a.calls)
		a.done()
		attempts = append(attempts, a)
		if err != nil {
			errs = append(errs, err.Error())
			log.Printf("Plugin update job %d on %s: %v", job.ID, site.Name, err)
		}
		for _, r := range buildResults(free, updates, err) {
			byName[r.Name] = r
		}
	}
	if len(locked) > 0 {
		results, a := updateLocked(job, site, locked, &errs)
		for _, r := range results {
			byName[r.Name] = r
		}
		if a != nil {
			attempts = append(attempts, a)
			for _, r := range a.results {
				covered[a] = append(covered[a], r.Name)
			}
		}
	}

	// Refresh the cached plugin list so the ledger status and the UI see
	// the post-update state.
	cacheErr := refreshPluginCache(site)
	if cacheErr != nil {
		log.Printf("Plugin update job %d: failed to refresh plugin cache for %s: %v", job.ID, site.Name, cacheErr)
	} else {
		if len(locked) > 0 {
			markHeldBack(job.SiteID, locked, byName)
		}
		markUnchanged(job.SiteID, covered, byName)
	}

	results := make([]models.UpdateResult, len(plugins))
	for i, p := range plugins {
		results[i] = byName[p.Name]
	}
	for _, a := range attempts {
		a.results = a.results[:0]
		for _, name := range covered[a] {
			a.results = append(a.results, byName[name])
		}
	}
	jobErr := strings.Join(errs, "; ")
	storeAttemptCalls(job, attempts)
	finish(job, models.UpdateJobDone, results, jobErr)
	writeLedger(job, results, ledgerStatus(job.SiteID, results, cacheErr == nil, updatedVulnerable), "")
	logAttempts(job, site, wpVersion, attempts)
}

// markUnchanged finds plugins wp-cli reported as updated whose version in
// the refreshed plugin cache is still the old one: the update "succeeded"
// without installing anything new, as some premium updaters do when the
// download isn't the release they announced. Their result keeps its
// status, with the version set back and a note, and the attempt that
// covered them is marked so the update log treats it as failed.
func markUnchanged(siteID string, covered map[*attempt][]string, byName map[string]models.UpdateResult) {
	plugins, err := db.GetSitePlugins(siteID)
	if err != nil {
		return
	}
	installed := make(map[string]string, len(plugins))
	for _, p := range plugins {
		installed[p.Name] = p.Version
	}
	for a, names := range covered {
		for _, name := range names {
			r := byName[name]
			version, ok := installed[name]
			if r.Status != models.UpdateUpdated || r.OldVersion == "" || !ok || version != r.OldVersion {
				continue
			}
			r.NewVersion = version
			r.Note = joinNotes(r.Note, fmt.Sprintf("wp-cli reported an update, but %s is still installed", version))
			byName[name] = r
			a.unchanged[name] = true
		}
	}
}

// updateLocked applies fix-release updates to locked plugins whose pending
// update comes from WordPress.org, and returns a result for every locked
// plugin, and the attempt when it ran an update (its results are the
// plugins it covered). Plugins it doesn't update come back as up to date;
// markHeldBack then flags those that still have an update.
func updateLocked(job models.UpdateJob, site models.CliSite, locked []models.PluginUpdateRequest, errs *[]string) ([]models.UpdateResult, *attempt) {
	packages, err := updatePackages(site)
	if err != nil {
		msg := fmt.Sprintf("failed to check update sources of locked plugins: %v", err)
		*errs = append(*errs, msg)
		log.Printf("Plugin update job %d on %s: %s", job.ID, site.Name, msg)
		return failAll(locked, msg), nil
	}

	var patchable, rest []models.PluginUpdateRequest
	for _, p := range locked {
		if wpcli.IsWordPressOrgPackage(packages[p.Name]) {
			patchable = append(patchable, p)
		} else {
			rest = append(rest, p)
		}
	}

	// Locked plugins whose update doesn't come from WordPress.org are
	// held back; those with no update at all are up to date.
	results := buildResults(rest, nil, nil)
	for i := range results {
		if packages[results[i].Name] != "" {
			results[i].Status = models.UpdateSkippedLocked
		}
	}
	var a *attempt
	if len(patchable) > 0 {
		a = newAttempt(attemptPatch)
		updates, err := updatePluginsPatch(site, requestNames(patchable), &a.calls)
		a.done()
		if err != nil {
			*errs = append(*errs, err.Error())
			log.Printf("Plugin update job %d on %s (fix releases): %v", job.ID, site.Name, err)
		}
		a.results = buildResults(patchable, updates, err)
		results = append(results, a.results...)
	}
	return results, a
}

// markHeldBack marks locked plugins that weren't updated but still have an
// update in the refreshed plugin cache as skipped, noting when a known
// vulnerability is only fixed by the held-back update.
func markHeldBack(siteID string, locked []models.PluginUpdateRequest, byName map[string]models.UpdateResult) {
	plugins, err := db.GetSitePlugins(siteID)
	if err != nil {
		return
	}
	cached := make(map[string]models.WPPlugin, len(plugins))
	for _, p := range plugins {
		cached[p.Name] = p
	}
	for _, req := range locked {
		r := byName[req.Name]
		p, ok := cached[req.Name]
		if r.Status == models.UpdateFailed || !ok || p.Update == "" || p.Update == "none" {
			continue
		}
		if r.Status == models.UpdateUpToDate {
			r.Status = models.UpdateSkippedLocked
		}
		// An updated plugin can still be vulnerable if the fix release
		// didn't fix everything.
		if pluginVulnerable(siteID, req.Name, p.Version) {
			if fixedIn := pluginFixedIn(siteID, req.Name, p.Version); fixedIn != "" {
				r.Note = fmt.Sprintf("Still vulnerable; fixed in %s or later, which the update lock holds back", fixedIn)
			} else {
				r.Note = "Still vulnerable; the update lock holds back the latest version"
			}
		}
		byName[req.Name] = r
	}
}

func requestNames(reqs []models.PluginUpdateRequest) []string {
	names := make([]string, len(reqs))
	for i, p := range reqs {
		names[i] = p.Name
	}
	return names
}

// runCore updates WordPress core to the latest minor or major version and
// refreshes the cached core state.
//
// On a locked site a major update needs the job to allow it; otherwise it
// runs as a minor update. The API already refuses such jobs, so this only
// catches a site locked after the job was queued.
func runCore(job models.UpdateJob, site models.CliSite) {
	oldVersion := cachedCoreVersion(job.SiteID)

	payload := jobCore(job)
	major := payload.Target == "major"
	var lockNote string
	if major && !payload.AllowMajor {
		locks, err := siteLocks(job.SiteID)
		switch {
		case err != nil:
			major = false
			lockNote = "Updated to the latest minor version only: failed to load update locks"
			log.Printf("Core update job %d: failed to load update locks: %v", job.ID, err)
		case locks.CoreLocked():
			major = false
			lockNote = "Updated to the latest minor version only: the site is update-locked"
		}
	}

	a := newAttempt(attemptCore)
	result, updateErr := updateCore(site, major, &a.calls)
	a.done()

	var status, errMsg string
	switch {
	case updateErr != nil:
		status = "failed"
		errMsg = updateErr.Error()
		log.Printf("Core update job %d on %s: %v", job.ID, site.Name, updateErr)
	case result.Success:
		status = "full"
	case result.Version == "unknown":
		// UpdateCore returns (zero-value result, nil error) when wp-cli
		// reports WordPress is already at the latest version for this
		// target (e.g. a concurrent update already applied it): not a
		// failure, just nothing to do.
		status = "partial"
	default:
		status = "failed"
		errMsg = "Core update did not complete successfully"
	}

	// Refresh the cached version/update availability regardless of
	// outcome, so the UI reflects the post-update state.
	newVersion := result.Version
	core, cacheErr := refreshCore(site)
	if cacheErr != nil {
		log.Printf("Core update job %d: failed to refresh core cache for %s: %v", job.ID, site.Name, cacheErr)
		core = nil
	} else if newVersion == "unknown" || newVersion == "" {
		// "unknown" means WordPress was already at the latest version;
		// use the freshly checked actual version instead.
		newVersion = core.Version
	}
	if newVersion == "unknown" {
		newVersion = oldVersion
	}

	r := models.UpdateResult{Name: coreResultName, OldVersion: oldVersion, NewVersion: newVersion, Note: lockNote}
	switch status {
	case "failed":
		r.Status = models.UpdateFailed
		r.Error = errMsg
	case "partial":
		r.Status = models.UpdateUpToDate
	default:
		r.Status = models.UpdateUpdated
		// wp-cli reported the update, but the refreshed version is the
		// old one.
		if core != nil && oldVersion != "" && core.Version == oldVersion {
			a.unchanged[coreResultName] = true
			r.NewVersion = core.Version
			r.Note = joinNotes(lockNote, fmt.Sprintf("wp-cli reported an update, but %s is still installed", core.Version))
		}
	}
	a.results = []models.UpdateResult{r}

	storeAttemptCalls(job, []*attempt{a})
	finishCore(job, models.UpdateJobDone, []models.UpdateResult{r}, core, errMsg)
	writeCoreLedger(job, newVersion, errMsg, status, lockNote)
	logAttempts(job, site, oldVersion, []*attempt{a})
}

// joinNotes joins the non-empty notes of a result.
func joinNotes(notes ...string) string {
	var kept []string
	for _, n := range notes {
		if n != "" {
			kept = append(kept, n)
		}
	}
	return strings.Join(kept, "; ")
}

func failedCoreResult(version, msg string) []models.UpdateResult {
	return []models.UpdateResult{{Name: coreResultName, OldVersion: version, NewVersion: version, Status: models.UpdateFailed, Error: msg}}
}

func finishCore(job models.UpdateJob, status string, results []models.UpdateResult, core *models.SiteCore, errMsg string) {
	if err := db.FinishUpdateJob(job.ID, status, results, core, errMsg); err != nil {
		log.Printf("Core update job %d: failed to record result: %v", job.ID, err)
	}
}

// writeCoreLedger records a finished core job in the site update ledger,
// in the format the core update handler used to write.
func writeCoreLedger(job models.UpdateJob, newVersion, errMsg, status, note string) {
	data := map[string]any{"target": jobCore(job).Target, "new_version": newVersion}
	if errMsg != "" {
		data["error"] = errMsg
	}
	if note != "" {
		data["note"] = note
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		log.Printf("Core update job %d: failed to encode ledger entry: %v", job.ID, err)
		return
	}
	if err := db.SaveSiteUpdateLedgerEntry(&models.SiteUpdateLedgerEntry{
		SiteID:     job.SiteID,
		UpdateType: "core",
		Status:     status,
		DataJSON:   string(encoded),
		UpdatedBy:  ledgerUser(job),
	}); err != nil {
		log.Printf("Core update job %d: failed to write ledger entry: %v", job.ID, err)
	}
}

func ledgerUser(job models.UpdateJob) string {
	if job.CreatedBy == "" {
		return "system"
	}
	return job.CreatedBy
}

func finish(job models.UpdateJob, status string, results []models.UpdateResult, errMsg string) {
	if err := db.FinishUpdateJob(job.ID, status, results, nil, errMsg); err != nil {
		log.Printf("Plugin update job %d: failed to record result: %v", job.ID, err)
	}
	if status == models.UpdateJobFailed {
		writeLedger(job, results, "failed", errMsg)
	}
}

func failAll(reqs []models.PluginUpdateRequest, msg string) []models.UpdateResult {
	results := make([]models.UpdateResult, len(reqs))
	for i, r := range reqs {
		results[i] = models.UpdateResult{Name: r.Name, OldVersion: r.OldVersion, NewVersion: r.OldVersion, Status: models.UpdateFailed, Error: msg}
	}
	return results
}

// buildResults works out each requested plugin's outcome from WP-CLI's
// per-plugin results and, when the call failed, from the versions the
// post-failure check found installed. A plugin whose version changed counts
// as updated even if the call as a whole failed or timed out.
func buildResults(reqs []models.PluginUpdateRequest, updates []wpcli.UpdateResult, err error) []models.UpdateResult {
	reported := make(map[string]wpcli.UpdateResult, len(updates))
	for _, u := range updates {
		reported[u.Name] = u
	}
	var failure *wpcli.UpdateFailure
	errors.As(err, &failure)

	results := make([]models.UpdateResult, 0, len(reqs))
	for _, req := range reqs {
		r := models.UpdateResult{Name: req.Name, OldVersion: req.OldVersion, NewVersion: req.OldVersion}
		u, ok := reported[req.Name]
		switch {
		case ok && u.Status == models.UpdateUpdated:
			if u.OldVersion != "" {
				r.OldVersion = u.OldVersion
			}
			r.NewVersion = u.NewVersion
			r.Status = models.UpdateUpdated
		case ok:
			// wp-cli's table only says "Error"; its warnings say why.
			r.Status = models.UpdateFailed
			r.Error = u.Status
			if u.Error != "" {
				r.Error = u.Error
			}
		case err == nil:
			// Not in WP-CLI's table: it had nothing to update.
			r.Status = models.UpdateUpToDate
		case failure != nil && failure.Versions[req.Name] != "" && req.OldVersion != "" && failure.Versions[req.Name] != req.OldVersion:
			r.NewVersion = failure.Versions[req.Name]
			r.Status = models.UpdateUpdated
		default:
			if failure != nil && failure.Versions[req.Name] != "" {
				r.NewVersion = failure.Versions[req.Name]
			}
			r.Status = models.UpdateFailed
			r.Error = err.Error()
		}
		results = append(results, r)
	}
	return results
}

// ledgerStatus picks a plugin update job's ledger status:
//   - "failed" if any plugin failed;
//   - "full" if the site has no plugin updates left (it's up to date);
//   - "vuln" if the job updated vulnerable plugins and no plugin with an
//     update left is vulnerable;
//   - otherwise "partial" (also when the refreshed state is unknown).
func ledgerStatus(siteID string, results []models.UpdateResult, cacheFresh, updatedVulnerable bool) string {
	for _, r := range results {
		if r.Status == models.UpdateFailed {
			return models.LedgerFailed
		}
	}
	if !cacheFresh {
		return models.LedgerPartial
	}
	plugins, err := db.GetSitePlugins(siteID)
	if err != nil {
		return models.LedgerPartial
	}
	var remaining []models.WPPlugin
	for _, p := range plugins {
		if p.Update != "" && p.Update != "none" {
			remaining = append(remaining, p)
		}
	}
	if len(remaining) == 0 {
		return models.LedgerFull
	}
	if !updatedVulnerable {
		return models.LedgerPartial
	}
	for _, p := range remaining {
		if pluginVulnerable(siteID, p.Name, p.Version) {
			return models.LedgerPartial
		}
	}
	return models.LedgerVuln
}

// anyVulnerable reports whether any of the requested plugins was
// vulnerable at the version installed when the job was created.
func anyVulnerable(siteID string, plugins []models.PluginUpdateRequest) bool {
	for _, p := range plugins {
		if p.OldVersion != "" && pluginVulnerable(siteID, p.Name, p.OldVersion) {
			return true
		}
	}
	return false
}

// writeLedger records a finished job in the site update ledger, using the
// single-plugin or bulk data format the UI already renders.
func writeLedger(job models.UpdateJob, results []models.UpdateResult, status, jobErr string) {
	if results == nil {
		results = failAll(jobPlugins(job), jobErr)
	}

	var data any
	if len(results) == 1 {
		r := results[0]
		single := map[string]any{"plugin": r.Name, "old_version": r.OldVersion, "new_version": r.NewVersion}
		if r.Error != "" {
			single["error"] = r.Error
		}
		if r.Status == models.UpdateSkippedLocked {
			single["skipped"] = true
		}
		if r.Note != "" {
			single["note"] = r.Note
		}
		data = single
	} else {
		succeeded, failed, skipped := 0, 0, 0
		updates := make([]map[string]any, len(results))
		for i, r := range results {
			entry := map[string]any{"plugin": r.Name, "old_version": r.OldVersion, "new_version": r.NewVersion, "status": "success"}
			switch r.Status {
			case models.UpdateFailed:
				entry["status"] = "failed"
				entry["error"] = r.Error
				failed++
			case models.UpdateSkippedLocked:
				entry["status"] = "skipped"
				skipped++
			default:
				succeeded++
			}
			if r.Note != "" {
				entry["note"] = r.Note
			}
			updates[i] = entry
		}
		summary := fmt.Sprintf("Bulk update of %d plugin(s): %d succeeded, %d failed", len(results), succeeded, failed)
		if skipped > 0 {
			summary += fmt.Sprintf(", %d held back by update locks", skipped)
		}
		data = map[string]any{
			"updates": updates,
			"summary": summary + ".",
		}
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		log.Printf("Plugin update job %d: failed to encode ledger entry: %v", job.ID, err)
		return
	}
	if err := db.SaveSiteUpdateLedgerEntry(&models.SiteUpdateLedgerEntry{
		SiteID:     job.SiteID,
		UpdateType: "plugin",
		Status:     status,
		DataJSON:   string(encoded),
		UpdatedBy:  ledgerUser(job),
	}); err != nil {
		log.Printf("Plugin update job %d: failed to write ledger entry: %v", job.ID, err)
	}
}
