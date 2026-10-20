package models

import (
	"encoding/json"
	"fmt"
	"time"
)

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
	// User management kinds create or delete one WordPress user.
	UpdateJobKindUserCreate = "user_create"
	UpdateJobKindUserDelete = "user_delete"
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

// IsUserKind reports whether kind creates or deletes a WordPress user.
func IsUserKind(kind string) bool {
	return kind == UpdateJobKindUserCreate || kind == UpdateJobKindUserDelete
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
	// UpdateSkippedLocked means a locked plugin still has an update, but
	// no fix release of it was available, so it wasn't updated.
	UpdateSkippedLocked = "Skipped (locked)"
)

// UpdateJob is one background change on a single site: `wp plugin update`
// for a set of plugins (Kind "plugins"), `wp core update` (Kind "core"),
// activating, deactivating, deleting or uninstalling a set of plugins,
// installing one plugin (Kind "install"), or creating or deleting a
// WordPress user. What to change is in Payload, whose type depends on Kind
// (see the *Payload types).
type UpdateJob struct {
	ID      int64           `json:"id"`
	Kind    string          `json:"kind"`
	SiteID  string          `json:"site_id"` // site UUID
	Status  string          `json:"status"`
	Payload json.RawMessage `json:"payload"`
	// UploadPath is the local path of an uploaded ZIP for an install job.
	// It's removed once the job has finished.
	UploadPath string `json:"-"`
	// Results has one entry per requested plugin (or a single "WordPress"
	// entry for core jobs, or the user's login for user jobs) once the job
	// has finished.
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

// NewUpdateJob returns a job of the given kind with payload, one of the
// *Payload types, encoded.
func NewUpdateJob(kind, siteID string, payload any) UpdateJob {
	return UpdateJob{Kind: kind, SiteID: siteID, Payload: EncodePayload(payload)}
}

// EncodePayload encodes one of the *Payload types, which always encode.
func EncodePayload(payload any) json.RawMessage {
	b, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("failed to encode job payload %T: %v", payload, err))
	}
	return b
}

// DecodePayload decodes a job's payload into the type for its kind. A job
// without a payload decodes to the zero value.
func DecodePayload[T any](job UpdateJob) (T, error) {
	var p T
	if len(job.Payload) == 0 {
		return p, nil
	}
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return p, fmt.Errorf("invalid payload in %s job %d: %w", job.Kind, job.ID, err)
	}
	return p, nil
}

// PluginsPayload is the payload of plugin update jobs and plugin
// management jobs (activate, deactivate, delete, uninstall).
type PluginsPayload struct {
	Plugins []PluginUpdateRequest `json:"plugins"`
	// AllowMajor lets an update job go past fix releases on locked
	// plugins. Only a confirmed single-plugin update sets it.
	AllowMajor bool `json:"allow_major,omitempty"`
}

// CorePayload is the payload of core update jobs.
type CorePayload struct {
	// Target is "minor" or "major".
	Target string `json:"target"`
	// AllowMajor lets a major update run on a locked site.
	AllowMajor bool `json:"allow_major,omitempty"`
}

// InstallPayload is the payload of plugin install jobs.
type InstallPayload struct {
	// Source is a WordPress.org slug, a ZIP URL, or the original filename
	// of an uploaded ZIP.
	Source string `json:"source"`
	// Activate makes the job activate the plugin after installing.
	Activate bool `json:"activate,omitempty"`
}

// UserCreatePayload is the payload of user create jobs. The password is
// never part of it: jman-api only keeps it in memory until the job runs.
type UserCreatePayload struct {
	Login string `json:"login"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// UserDeletePayload is the payload of user delete jobs. Login and Email are
// what jman knew of the user when the job was created; the job checks that
// the user still matches them before deleting it.
type UserDeletePayload struct {
	UserID int    `json:"user_id"`
	Login  string `json:"login"`
	Email  string `json:"email"`
	// Reassign is the login or email of the user who gets the deleted
	// user's content; empty when the user has none.
	Reassign string `json:"reassign,omitempty"`
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
	// Note explains a skipped update, e.g. that a known vulnerability is
	// only fixed by a major update the lock held back.
	Note string `json:"note,omitempty"`
}
