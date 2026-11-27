package updatejobs

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/wpcli"
)

// The kinds of update attempt in the update log.
const (
	// attemptPlugins is a `wp plugin update` of plugins without a lock.
	attemptPlugins = "plugins"
	// attemptPatch is a fix-release-only `wp plugin update --patch` of
	// locked plugins.
	attemptPatch = "patch"
	// attemptZip is a plugin updated from a ZIP (uploaded, downloaded or
	// copied from another site) with `wp plugin install --force`.
	attemptZip = "zip"
	// attemptCore is `wp core update` with its `wp core update-db`.
	attemptCore = "core"
)

// updateLogFile is the default update log's name in the data directory.
const updateLogFile = "update-attempts.jsonl"

var (
	// cliPHPVersion is the PHP version wp-cli runs on for a site.
	cliPHPVersion = wpcli.CLIPHPVersion
	// sitePHPVersion is the site's own PHP-FPM version, from the
	// inventory.
	sitePHPVersion = func(siteID string) string {
		site, err := db.GetManagedSite(siteID)
		if err != nil || site == nil {
			return ""
		}
		return site.PHPVersion
	}
	updateLogMu sync.Mutex
)

// attempt is one update command of a job and the wp-cli calls it made.
// The caller fills in results (one per plugin it covered, or WordPress)
// and unchanged (results wp-cli reported as a success whose version
// didn't change) once the job knows them.
type attempt struct {
	kind      string
	started   time.Time
	duration  time.Duration
	calls     []models.WPCLICall
	results   []models.UpdateResult
	unchanged map[string]bool
}

func newAttempt(kind string) *attempt {
	return &attempt{kind: kind, started: time.Now(), unchanged: map[string]bool{}}
}

// done marks the end of the attempt's update command.
func (a *attempt) done() { a.duration = time.Since(a.started) }

// failed reports whether a call failed, a result failed, or a reported
// update didn't change a version.
func (a *attempt) failed() bool {
	if len(a.unchanged) > 0 {
		return true
	}
	for _, c := range a.calls {
		if c.Error != "" {
			return true
		}
	}
	for _, r := range a.results {
		if r.Status == models.UpdateFailed {
			return true
		}
	}
	return false
}

// attemptLine is one line of the update log.
type attemptLine struct {
	Time  time.Time `json:"time"`
	JobID int64     `json:"job_id"`
	// JobIDs lists every job of a combined run (docs/update-queue.md),
	// first JobID; the attempt's results cover all their plugins. Empty
	// for a job that ran on its own.
	JobIDs    []int64 `json:"job_ids,omitempty"`
	JobKind   string  `json:"job_kind"`
	CreatedBy string  `json:"created_by"`
	Type      string  `json:"type"`
	SiteID    string  `json:"site_id"`
	Site      string  `json:"site"`
	Server    string  `json:"server"`
	// WPVersion is the cached WordPress version when the job started.
	WPVersion string `json:"wp_version"`
	// PHPVersion is the site's PHP-FPM version; CLIPHPVersion is the PHP
	// wp-cli (and so the update) ran on.
	PHPVersion    string          `json:"php_version"`
	CLIPHPVersion string          `json:"cli_php_version"`
	Failed        bool            `json:"failed"`
	DurationMS    int64           `json:"duration_ms"`
	Results       []attemptResult `json:"results"`
	// Calls has everything wp-cli printed, for failed attempts only.
	Calls []models.WPCLICall `json:"calls,omitempty"`
}

type attemptResult struct {
	Name       string `json:"name"`
	OldVersion string `json:"old_version"`
	NewVersion string `json:"new_version"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	// Cause and CauseDetail are a failure's, as in models.UpdateResult.
	Cause       string `json:"cause,omitempty"`
	CauseDetail string `json:"cause_detail,omitempty"`
	// VersionUnchanged means wp-cli reported a success but the version
	// installed afterwards is the old one.
	VersionUnchanged bool `json:"version_unchanged,omitempty"`
}

// storeAttemptCalls keeps the wp-cli calls of a job's failed attempts with
// the job, for the UI to show. Call it before the job is finished, so the
// output is there when the job shows as done.
func storeAttemptCalls(job models.UpdateJob, attempts []*attempt) {
	var calls []models.WPCLICall
	for _, a := range attempts {
		if a.failed() {
			calls = append(calls, a.calls...)
		}
	}
	if len(calls) == 0 {
		return
	}
	if err := db.SetUpdateJobCalls(job.ID, calls); err != nil {
		log.Printf("Update job %d: failed to store wp-cli output: %v", job.ID, err)
	}
}

// updateLogPath is where the update log goes, or "" when it's off. Without
// a configured path or a data directory (tests) there is no log.
func updateLogPath() string {
	switch p := config.Cfg.UpdateLog; {
	case p == "off":
		return ""
	case p != "":
		return p
	case config.RunData.DataDir != "":
		return filepath.Join(config.RunData.DataDir, updateLogFile)
	}
	return ""
}

// logAttempts appends a job's attempts to the update log. It looks up the
// PHP versions first, which takes a wp-cli call, so call it after the job
// is finished.
func logAttempts(job models.UpdateJob, site models.CliSite, wpVersion string, attempts []*attempt) {
	logJobAttempts([]models.UpdateJob{job}, site, wpVersion, attempts)
}

// logJobAttempts appends the attempts of jobs that ran together to the
// update log, each attempt once: under the first job, with every job's ID
// in job_ids.
func logJobAttempts(jobs []models.UpdateJob, site models.CliSite, wpVersion string, attempts []*attempt) {
	job := jobs[0]
	var jobIDs []int64
	if len(jobs) > 1 {
		for _, j := range jobs {
			jobIDs = append(jobIDs, j.ID)
		}
	}
	path := updateLogPath()
	if path == "" || len(attempts) == 0 {
		return
	}
	cliPHP, err := cliPHPVersion(site)
	if err != nil {
		log.Printf("Update job %d: failed to get the PHP version of wp-cli on %s: %v", job.ID, site.Name, err)
	}
	phpVersion := sitePHPVersion(site.ID)

	updateLogMu.Lock()
	defer updateLogMu.Unlock()
	// Opened for each job rather than kept open, so logrotate can move
	// the file without jman-api writing on into the rotated one.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("Update job %d: failed to open the update log: %v", job.ID, err)
		return
	}
	defer f.Close()
	for _, a := range attempts {
		line := attemptLine{
			Time:          a.started.UTC(),
			JobID:         job.ID,
			JobIDs:        jobIDs,
			JobKind:       job.Kind,
			CreatedBy:     job.CreatedBy,
			Type:          a.kind,
			SiteID:        site.ID,
			Site:          site.Name,
			Server:        site.ServerName,
			WPVersion:     wpVersion,
			PHPVersion:    phpVersion,
			CLIPHPVersion: cliPHP,
			Failed:        a.failed(),
			DurationMS:    a.duration.Milliseconds(),
		}
		for _, r := range a.results {
			line.Results = append(line.Results, attemptResult{
				Name:             r.Name,
				OldVersion:       r.OldVersion,
				NewVersion:       r.NewVersion,
				Status:           r.Status,
				Error:            r.Error,
				Cause:            r.Cause,
				CauseDetail:      r.CauseDetail,
				VersionUnchanged: a.unchanged[r.Name],
			})
		}
		if line.Failed {
			line.Calls = a.calls
		}
		b, err := json.Marshal(line)
		if err != nil {
			log.Printf("Update job %d: failed to encode an update log line: %v", job.ID, err)
			continue
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			log.Printf("Update job %d: failed to write the update log: %v", job.ID, err)
			return
		}
	}
}
