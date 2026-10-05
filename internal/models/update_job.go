package models

import "time"

// Update job kinds.
const (
	// UpdateJobKindPlugins updates the listed plugins.
	UpdateJobKindPlugins = "plugins"
	UpdateJobKindCore    = "core"
	// Plugin management kinds act on the listed plugins.
	UpdateJobKindActivate   = "activate"
	UpdateJobKindDeactivate = "deactivate"
	// UpdateJobKindDelete removes the plugins' files only.
	UpdateJobKindDelete = "delete"
	// UpdateJobKindUninstall runs the plugins' uninstall routines (which
	// usually remove their data) and then deletes them.
	UpdateJobKindUninstall = "uninstall"
	// UpdateJobKindInstall installs one plugin from Source.
	UpdateJobKindInstall = "install"
)

// IsPluginActionKind reports whether kind is one of the plugin management
// kinds (activate, deactivate, delete, uninstall).
func IsPluginActionKind(kind string) bool {
	switch kind {
	case UpdateJobKindActivate, UpdateJobKindDeactivate, UpdateJobKindDelete, UpdateJobKindUninstall:
		return true
	}
	return false
}

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
	// UpdateDone means a plugin management action succeeded.
	UpdateDone = "Done"
)

// UpdateJob is one background change on a single site: `wp plugin update`
// for a set of plugins (Kind "plugins"), `wp core update` (Kind "core"),
// activating, deactivating, deleting or uninstalling a set of plugins, or
// installing one plugin (Kind "install").
type UpdateJob struct {
	ID     int64  `json:"id"`
	Kind   string `json:"kind"`
	SiteID string `json:"site_id"` // site UUID
	Status string `json:"status"`
	// Plugins lists the plugins to act on; empty for core and install jobs.
	Plugins []PluginUpdateRequest `json:"plugins"`
	// Target is "minor" or "major" for core jobs.
	Target string `json:"target,omitempty"`
	// Source is what an install job installs: a WordPress.org slug, a ZIP
	// URL, or the original filename of an uploaded ZIP.
	Source string `json:"source,omitempty"`
	// Activate makes an install job activate the plugin after installing.
	Activate bool `json:"activate,omitempty"`
	// UploadPath is the local path of an uploaded ZIP for an install job.
	// It's removed once the job has finished.
	UploadPath string `json:"-"`
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
