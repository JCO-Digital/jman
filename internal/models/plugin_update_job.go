package models

import "time"

// Plugin update job statuses.
const (
	PluginUpdateJobQueued  = "queued"
	PluginUpdateJobRunning = "running"
	// PluginUpdateJobDone means the job ran to completion; individual
	// plugins may still have failed (see Results).
	PluginUpdateJobDone = "done"
	// PluginUpdateJobFailed means the job couldn't run at all (e.g. the
	// site is no longer reachable over WP-CLI).
	PluginUpdateJobFailed = "failed"
	// PluginUpdateJobInterrupted means jman-api stopped while the job was
	// running, so its outcome on the site is unknown.
	PluginUpdateJobInterrupted = "interrupted"
)

// Per-plugin result statuses in PluginUpdateResult.Status.
const (
	PluginUpdateUpdated  = "Updated"
	PluginUpdateUpToDate = "Up to date"
	PluginUpdateFailed   = "failed"
)

// PluginUpdateJob is one background run of `wp plugin update` for a set of
// plugins on a single site.
type PluginUpdateJob struct {
	ID      int64                 `json:"id"`
	SiteID  string                `json:"site_id"` // site UUID
	Status  string                `json:"status"`
	Plugins []PluginUpdateRequest `json:"plugins"`
	// Results has one entry per requested plugin once the job has finished.
	Results []PluginUpdateResult `json:"results"`
	// Error describes a job-level failure or warning (e.g. the site was
	// left in maintenance mode).
	Error      string     `json:"error,omitempty"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// Active reports whether the job is still waiting or running.
func (j PluginUpdateJob) Active() bool {
	return j.Status == PluginUpdateJobQueued || j.Status == PluginUpdateJobRunning
}

// PluginUpdateRequest is a plugin to update, with the version installed
// when the job was created (empty if unknown).
type PluginUpdateRequest struct {
	Name       string `json:"name"`
	OldVersion string `json:"old_version"`
}

// PluginUpdateResult is the outcome of updating one plugin.
type PluginUpdateResult struct {
	Name       string `json:"name"`
	OldVersion string `json:"old_version"`
	NewVersion string `json:"new_version"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}
