package models

import (
	"encoding/json"
	"fmt"
	"regexp"
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
	// UpdateJobKindMuInstall writes one file into wp-content/mu-plugins:
	// the bojaco mu-plugin or a bojaco-compat plugin.
	UpdateJobKindMuInstall = "mu_install"
	// UpdateJobKindFileMods sets DISALLOW_FILE_MODS in the site's
	// wp-config.php.
	UpdateJobKindFileMods = "file_mods"
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

// UpdateJobPriority orders queued jobs: higher runs first. The zero value
// is normal. It encodes in JSON as its name ("low", "normal", "high",
// "urgent").
type UpdateJobPriority int

// Update job priorities. Defaults are set by the API from the request's
// shape (docs/update-queue.md): one site is high, several are normal, the
// nightly core update is low. Urgent is only ever set on request.
const (
	UpdateJobPriorityLow    UpdateJobPriority = -10
	UpdateJobPriorityNormal UpdateJobPriority = 0
	UpdateJobPriorityHigh   UpdateJobPriority = 10
	UpdateJobPriorityUrgent UpdateJobPriority = 20
)

var priorityNames = map[UpdateJobPriority]string{
	UpdateJobPriorityLow:    "low",
	UpdateJobPriorityNormal: "normal",
	UpdateJobPriorityHigh:   "high",
	UpdateJobPriorityUrgent: "urgent",
}

// String returns the priority's name, or its number if it has none.
func (p UpdateJobPriority) String() string {
	if name, ok := priorityNames[p]; ok {
		return name
	}
	return fmt.Sprintf("%d", int(p))
}

// ParseUpdateJobPriority parses a priority name.
func ParseUpdateJobPriority(name string) (UpdateJobPriority, error) {
	for p, n := range priorityNames {
		if n == name {
			return p, nil
		}
	}
	return 0, fmt.Errorf("invalid priority %q: must be low, normal, high or urgent", name)
}

// MarshalText encodes the priority as its name.
func (p UpdateJobPriority) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// UnmarshalText decodes a priority name.
func (p *UpdateJobPriority) UnmarshalText(b []byte) error {
	v, err := ParseUpdateJobPriority(string(b))
	if err != nil {
		return err
	}
	*p = v
	return nil
}

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
	// UpdateSkippedRequirements means the plugin's update needs a newer
	// WordPress or PHP version than the site has, so wp-cli would refuse
	// it and it wasn't tried. The result's Note says what it needs.
	UpdateSkippedRequirements = "Skipped (requirements)"
)

// UpdateJob is one background change on a single site: `wp plugin update`
// for a set of plugins (Kind "plugins"), `wp core update` (Kind "core"),
// activating, deactivating, deleting or uninstalling a set of plugins,
// installing one plugin (Kind "install"), creating or deleting a
// WordPress user, writing a file into mu-plugins (Kind "mu_install"), or
// setting DISALLOW_FILE_MODS (Kind "file_mods"). What to change is in Payload, whose type depends on Kind
// (see the *Payload types).
type UpdateJob struct {
	ID      int64           `json:"id"`
	Kind    string          `json:"kind"`
	SiteID  string          `json:"site_id"` // site UUID
	Status  string          `json:"status"`
	Payload json.RawMessage `json:"payload"`
	// UploadPath is the local path of an uploaded ZIP for an install job,
	// or of the file a mu-plugin install job writes. It's removed once the
	// last job that uses it has finished.
	UploadPath string `json:"-"`
	// Results has one entry per requested plugin (or a single "WordPress"
	// entry for core jobs, or the user's login for user jobs) once the job
	// has finished.
	Results []UpdateResult `json:"results"`
	// Core is the site's core version state refreshed after a core job.
	Core *SiteCore `json:"core,omitempty"`
	// TextDiff compares the site's visible text before and after a plugin,
	// ZIP or core update (see docs/update-text-diff.md). Job lists leave it
	// out and only carry TextDiffSummary.
	TextDiff        *TextDiff        `json:"text_diff,omitempty"`
	TextDiffSummary *TextDiffSummary `json:"text_diff_summary,omitempty"`
	// Error describes a job-level failure or warning (e.g. the site was
	// left in maintenance mode). It's a short message; Calls has wp-cli's
	// full output.
	Error string `json:"error,omitempty"`
	// Calls are the wp-cli commands of a plugin, ZIP or core update that
	// failed or didn't change a version, with everything they printed.
	// Only GetUpdateJob returns them; HasOutput tells lists they exist.
	// Jobs that ran together all get the calls of their shared run.
	Calls     []WPCLICall `json:"calls,omitempty"`
	HasOutput bool        `json:"has_output,omitempty"`
	// Priority orders the queue (see UpdateJobPriority).
	Priority UpdateJobPriority `json:"priority"`
	// Position is the job's approximate place in the queue (1 runs next),
	// set only on queued jobs returned by the API. It isn't stored.
	Position int `json:"position,omitempty"`
	// BatchID is the ID of the first job of a group of plugin update jobs
	// on one site that ran together in one WP-CLI call, sharing a text
	// diff. Zero for a job that ran on its own.
	BatchID    int64      `json:"batch_id,omitempty"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// Active reports whether the job is still waiting or running.
func (j UpdateJob) Active() bool {
	return j.Status == UpdateJobQueued || j.Status == UpdateJobRunning
}

// TextDiffPending reports whether the job's text diff is still running,
// which it does for a while after the job has finished.
func (j UpdateJob) TextDiffPending() bool {
	return j.TextDiffSummary != nil && j.TextDiffSummary.Status == TextDiffRunning
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
	// Plugin is set when the job updates an installed plugin from a ZIP:
	// the plugin's slug, which the ZIP was checked to hold. OldVersion is
	// its version when the job was created.
	Plugin     string `json:"plugin,omitempty"`
	OldVersion string `json:"old_version,omitempty"`
	// FromSite names the site the ZIP's files were copied from, if they
	// were.
	FromSite string `json:"from_site,omitempty"`
}

// MuInstallPayload is the payload of mu-plugin install jobs. The file's
// contents are in the job's upload, which jman-api fetched or was sent
// and whose hash it added to the scan allowlist.
type MuInstallPayload struct {
	// File is the file's name in wp-content/mu-plugins, e.g. "bojaco.php"
	// or "compat-woocommerce.php".
	File string `json:"file"`
	// Source is the URL jman-api downloaded the file from, or the name of
	// the uploaded file.
	Source string `json:"source"`
	SHA256 string `json:"sha256"`
}

// FileModsPayload is the payload of file_mods jobs.
type FileModsPayload struct {
	// Disallow is the value DISALLOW_FILE_MODS is set to: true stops
	// plugin, theme and core changes from wp-admin.
	Disallow bool `json:"disallow"`
}

// MuPluginBojaco is the bojaco mu-plugin's file name in mu-plugins.
const MuPluginBojaco = "bojaco.php"

var compatFileRegex = regexp.MustCompile(`^compat-[a-z0-9][a-z0-9-]*\.php$`)

// CompatFile is the file name of the bojaco-compat plugin for a plugin.
func CompatFile(slug string) string { return "compat-" + slug + ".php" }

// ValidMuPluginFile reports whether name is a file mu-plugin install jobs
// may write: the bojaco mu-plugin or a bojaco-compat plugin.
func ValidMuPluginFile(name string) bool {
	return name == MuPluginBojaco || compatFileRegex.MatchString(name)
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

// WPCLICall is one wp-cli command an update ran and everything it
// printed, kept with failed jobs and in the update attempt log.
type WPCLICall struct {
	// Args are the command's arguments, without jman's global ones
	// (--ssh, --path, --user, --exec, --skip-*).
	Args       []string `json:"args"`
	Stdout     string   `json:"stdout,omitempty"`
	Stderr     string   `json:"stderr,omitempty"`
	Error      string   `json:"error,omitempty"`
	DurationMS int64    `json:"duration_ms"`
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
