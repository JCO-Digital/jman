package models

import "time"

// Update job kinds.
const (
	UpdateJobKindPlugins = "plugins"
	UpdateJobKindCore    = "core"
)

// Update job statuses.
const (
	UpdateJobQueued  = "queued"
	UpdateJobRunning = "running"
	// UpdateJobDone means the job ran to completion; individual
	// results may still have failed (see Results).
	UpdateJobDone = "done"
	// UpdateJobFailed means the job couldn't run at all (e.g. the
	// site is no longer reachable over WP-CLI).
	UpdateJobFailed = "failed"
	// UpdateJobInterrupted means jman-api stopped while the job was
	// running, so its outcome on the site is unknown.
	UpdateJobInterrupted = "interrupted"
)

// Result statuses in UpdateResult.Status.
const (
	UpdateUpdated  = "Updated"
	UpdateUpToDate = "Up to date"
	UpdateFailed   = "failed"
)

// UpdateJob is one background update on a single site: either
// `wp plugin update` for a set of plugins (Kind "plugins") or
// `wp core update` (Kind "core").
type UpdateJob struct {
	ID     int64  `json:"id"`
	Kind   string `json:"kind"`
	SiteID string `json:"site_id"` // site UUID
	Status string `json:"status"`
	// Plugins lists the plugins to update; empty for core jobs.
	Plugins []PluginUpdateRequest `json:"plugins"`
	// Target is "minor" or "major" for core jobs.
	Target string `json:"target,omitempty"`
	// Results has one entry per requested plugin (or a single "WordPress"
	// entry for core jobs) once the job has finished.
	Results []UpdateResult `json:"results"`
	// Core is the site's core version state refreshed after a core job.
	Core *SiteCore `json:"core,omitempty"`
	// Error describes a job-level failure or warning (e.g. the site was
	// left in maintenance mode).
	Error      string     `json:"error,omitempty"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// Active reports whether the job is still waiting or running.
func (j UpdateJob) Active() bool {
	return j.Status == UpdateJobQueued || j.Status == UpdateJobRunning
}

// PluginUpdateRequest is a plugin to update, with the version installed
// when the job was created (empty if unknown).
type PluginUpdateRequest struct {
	Name       string `json:"name"`
	OldVersion string `json:"old_version"`
}

// UpdateResult is the outcome of updating one plugin, or WordPress core.
type UpdateResult struct {
	Name       string `json:"name"`
	OldVersion string `json:"old_version"`
	NewVersion string `json:"new_version"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}
