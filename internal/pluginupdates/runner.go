// Package pluginupdates runs plugin updates as background jobs inside
// jman-api. A job updates one or more plugins on one site with a single
// `wp plugin update` call; jobs for different sites run concurrently, jobs
// for the same site one at a time. Jobs live in api.db, so their state
// survives page reloads and is visible to every user, and each finished job
// writes one entry to the site update ledger.
package pluginupdates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/cache"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// Workers is how many jobs (on different sites) run at once.
const Workers = 4

// jobRetention is how long finished jobs are kept; their outcome stays in
// the site update ledger.
const jobRetention = 7 * 24 * time.Hour

const interruptedReason = "jman-api restarted while this update was running; check the site's plugin versions"

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
	refreshPluginCache = cache.UpdateSitePluginCache
)

// queue holds the IDs of jobs waiting to run and the sites with a job in
// progress.
type queue struct {
	mu      sync.Mutex
	cond    *sync.Cond
	pending []models.PluginUpdateJob
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

func (qu *queue) push(job models.PluginUpdateJob) {
	qu.mu.Lock()
	qu.pending = append(qu.pending, job)
	qu.mu.Unlock()
	qu.cond.Broadcast()
}

// next blocks until a job whose site isn't busy is available, marks that
// site busy and returns the job. ok is false once the queue is closed.
func (qu *queue) next() (job models.PluginUpdateJob, ok bool) {
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
	interrupted, err := db.InterruptRunningPluginUpdateJobs(interruptedReason)
	if err != nil {
		return fmt.Errorf("failed to recover plugin update jobs: %w", err)
	}
	for _, job := range interrupted {
		writeLedger(job, nil, "failed", interruptedReason)
	}

	queued, err := db.ListQueuedPluginUpdateJobs()
	if err != nil {
		return fmt.Errorf("failed to load queued plugin update jobs: %w", err)
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
			if err := db.PruneFinishedPluginUpdateJobs(time.Now().Add(-jobRetention)); err != nil {
				log.Printf("Failed to prune plugin update jobs: %v", err)
			}
			select {
			case <-ctx.Done():
				qu.close()
				return
			case <-ticker.C:
			}
		}
	}()

	log.Printf("Plugin update runner started (%d workers, %d queued jobs, %d interrupted)", Workers, len(queued), len(interrupted))
	return nil
}

// Enqueue stores a new job and queues it to run.
func Enqueue(job *models.PluginUpdateJob) error {
	if err := db.CreatePluginUpdateJob(job); err != nil {
		return err
	}
	q.push(*job)
	return nil
}

// run executes one job and records its outcome.
func run(job models.PluginUpdateJob) {
	if err := db.StartPluginUpdateJob(job.ID); err != nil {
		log.Printf("Plugin update job %d: failed to mark running: %v", job.ID, err)
	}

	site, err := findSite(job.SiteID)
	if err != nil {
		finish(job, models.PluginUpdateJobFailed, failAll(job.Plugins, err.Error()), err.Error())
		return
	}

	names := make([]string, len(job.Plugins))
	for i, p := range job.Plugins {
		names[i] = p.Name
	}
	updates, updateErr := updatePlugins(*site, names)
	results := buildResults(job.Plugins, updates, updateErr)

	jobErr := ""
	if updateErr != nil {
		jobErr = updateErr.Error()
		log.Printf("Plugin update job %d on %s: %v", job.ID, site.Name, updateErr)
	}

	// Refresh the cached plugin list so the ledger status and the UI see
	// the post-update state.
	cacheErr := refreshPluginCache(*site)
	if cacheErr != nil {
		log.Printf("Plugin update job %d: failed to refresh plugin cache for %s: %v", job.ID, site.Name, cacheErr)
	}

	finish(job, models.PluginUpdateJobDone, results, jobErr)
	writeLedger(job, results, ledgerStatus(job.SiteID, results, cacheErr == nil), "")
}

func finish(job models.PluginUpdateJob, status string, results []models.PluginUpdateResult, errMsg string) {
	if err := db.FinishPluginUpdateJob(job.ID, status, results, errMsg); err != nil {
		log.Printf("Plugin update job %d: failed to record result: %v", job.ID, err)
	}
	if status == models.PluginUpdateJobFailed {
		writeLedger(job, results, "failed", errMsg)
	}
}

func failAll(reqs []models.PluginUpdateRequest, msg string) []models.PluginUpdateResult {
	results := make([]models.PluginUpdateResult, len(reqs))
	for i, r := range reqs {
		results[i] = models.PluginUpdateResult{Name: r.Name, OldVersion: r.OldVersion, NewVersion: r.OldVersion, Status: models.PluginUpdateFailed, Error: msg}
	}
	return results
}

// buildResults works out each requested plugin's outcome from WP-CLI's
// per-plugin results and, when the call failed, from the versions the
// post-failure check found installed. A plugin whose version changed counts
// as updated even if the call as a whole failed or timed out.
func buildResults(reqs []models.PluginUpdateRequest, updates []wpcli.UpdateResult, err error) []models.PluginUpdateResult {
	reported := make(map[string]wpcli.UpdateResult, len(updates))
	for _, u := range updates {
		reported[u.Name] = u
	}
	var failure *wpcli.UpdateFailure
	errors.As(err, &failure)

	results := make([]models.PluginUpdateResult, 0, len(reqs))
	for _, req := range reqs {
		r := models.PluginUpdateResult{Name: req.Name, OldVersion: req.OldVersion, NewVersion: req.OldVersion}
		u, ok := reported[req.Name]
		switch {
		case ok && u.Status == models.PluginUpdateUpdated:
			if u.OldVersion != "" {
				r.OldVersion = u.OldVersion
			}
			r.NewVersion = u.NewVersion
			r.Status = models.PluginUpdateUpdated
		case ok:
			r.Status = models.PluginUpdateFailed
			r.Error = u.Status
		case err == nil:
			// Not in WP-CLI's table: it had nothing to update.
			r.Status = models.PluginUpdateUpToDate
		case failure != nil && failure.Versions[req.Name] != "" && req.OldVersion != "" && failure.Versions[req.Name] != req.OldVersion:
			r.NewVersion = failure.Versions[req.Name]
			r.Status = models.PluginUpdateUpdated
		default:
			if failure != nil && failure.Versions[req.Name] != "" {
				r.NewVersion = failure.Versions[req.Name]
			}
			r.Status = models.PluginUpdateFailed
			r.Error = err.Error()
		}
		results = append(results, r)
	}
	return results
}

// ledgerStatus follows the site update ledger's convention: "failed" if any
// plugin failed, "full" if the site has no plugin updates left, otherwise
// "partial".
func ledgerStatus(siteID string, results []models.PluginUpdateResult, cacheFresh bool) string {
	for _, r := range results {
		if r.Status == models.PluginUpdateFailed {
			return "failed"
		}
	}
	if !cacheFresh {
		return "partial"
	}
	plugins, err := db.GetSitePlugins(siteID)
	if err != nil {
		return "partial"
	}
	for _, p := range plugins {
		if p.Update != "" && p.Update != "none" {
			return "partial"
		}
	}
	return "full"
}

// writeLedger records a finished job in the site update ledger, using the
// single-plugin or bulk data format the UI already renders.
func writeLedger(job models.PluginUpdateJob, results []models.PluginUpdateResult, status, jobErr string) {
	if results == nil {
		results = failAll(job.Plugins, jobErr)
	}

	var data any
	if len(results) == 1 {
		r := results[0]
		single := map[string]any{"plugin": r.Name, "old_version": r.OldVersion, "new_version": r.NewVersion}
		if r.Error != "" {
			single["error"] = r.Error
		}
		data = single
	} else {
		succeeded := 0
		updates := make([]map[string]any, len(results))
		for i, r := range results {
			entry := map[string]any{"plugin": r.Name, "old_version": r.OldVersion, "new_version": r.NewVersion, "status": "success"}
			if r.Status == models.PluginUpdateFailed {
				entry["status"] = "failed"
				entry["error"] = r.Error
			} else {
				succeeded++
			}
			updates[i] = entry
		}
		data = map[string]any{
			"updates": updates,
			"summary": fmt.Sprintf("Bulk update of %d plugin(s): %d succeeded, %d failed.", len(results), succeeded, len(results)-succeeded),
		}
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		log.Printf("Plugin update job %d: failed to encode ledger entry: %v", job.ID, err)
		return
	}
	updatedBy := job.CreatedBy
	if updatedBy == "" {
		updatedBy = "system"
	}
	if err := db.SaveSiteUpdateLedgerEntry(&models.SiteUpdateLedgerEntry{
		SiteID:     job.SiteID,
		UpdateType: "plugin",
		Status:     status,
		DataJSON:   string(encoded),
		UpdatedBy:  updatedBy,
	}); err != nil {
		log.Printf("Plugin update job %d: failed to write ledger entry: %v", job.ID, err)
	}
}
