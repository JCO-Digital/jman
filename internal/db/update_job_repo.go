package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

const updateJobColumns = `id, kind, site_id, status, payload, upload_path, results, core, text_diff, calls, error, priority, batch_id, created_by, created_at, started_at, finished_at`

// listUpdateJobColumns are updateJobColumns for job lists, which show only
// whether a job has wp-cli output: the output is replaced by that.
const listUpdateJobColumns = `id, kind, site_id, status, payload, upload_path, results, core, text_diff,
	CASE WHEN calls IS NULL OR calls IN ('', 'null', '[]') THEN NULL ELSE 'listed' END,
	error, priority, batch_id, created_by, created_at, started_at, finished_at`

// CreateUpdateJob stores a new queued job and sets its ID, Status
// and CreatedAt. Kind defaults to plugins.
func CreateUpdateJob(job *models.UpdateJob) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	if job.Kind == "" {
		job.Kind = models.UpdateJobKindPlugins
	}
	if len(job.Payload) == 0 {
		job.Payload = json.RawMessage("{}")
	}
	job.Status = models.UpdateJobQueued
	job.CreatedAt = time.Now()
	job.Results = []models.UpdateResult{}

	err := db.QueryRow(
		`INSERT INTO plugin_update_jobs (kind, site_id, status, payload, upload_path, priority, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		job.Kind, job.SiteID, job.Status, string(job.Payload), job.UploadPath, int(job.Priority), job.CreatedBy, job.CreatedAt,
	).Scan(&job.ID)
	if err != nil {
		return fmt.Errorf("failed to create update job: %w", err)
	}
	return nil
}

// GetUpdateJob returns a job by ID, or nil if it doesn't exist.
func GetUpdateJob(id int64) (*models.UpdateJob, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT `+updateJobColumns+` FROM plugin_update_jobs WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get update job: %w", err)
	}
	jobs, err := scanUpdateJobs(rows)
	if err != nil || len(jobs) == 0 {
		return nil, err
	}
	return &jobs[0], nil
}

// LatestUpdateJobBy returns the most recently created job of createdBy
// (e.g. "nightly"), or nil if there is none.
func LatestUpdateJobBy(createdBy string) (*models.UpdateJob, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT `+updateJobColumns+` FROM plugin_update_jobs WHERE created_by = ? ORDER BY id DESC LIMIT 1`, createdBy)
	if err != nil {
		return nil, fmt.Errorf("failed to get the latest update job: %w", err)
	}
	jobs, err := scanUpdateJobs(rows)
	if err != nil || len(jobs) == 0 {
		return nil, err
	}
	return &jobs[0], nil
}

// ListUpdateJobs returns every queued or running job plus those that
// finished at or after finishedSince, oldest first. As a list, the jobs
// have their text diff's summary but not the diff, and HasOutput but not
// the wp-cli output (GetUpdateJob has both).
func ListUpdateJobs(finishedSince time.Time) ([]models.UpdateJob, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(
		`SELECT `+listUpdateJobColumns+` FROM plugin_update_jobs
		 WHERE status IN (?, ?) OR finished_at >= ?
		 ORDER BY id`,
		models.UpdateJobQueued, models.UpdateJobRunning, finishedSince,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list update jobs: %w", err)
	}
	return scanUpdateJobList(rows, true)
}

// ListTextDiffJobs returns the jobs created at or after since that have a
// text diff, newest first, as a list (see ListUpdateJobs).
func ListTextDiffJobs(since time.Time) ([]models.UpdateJob, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.Query(
		`SELECT `+listUpdateJobColumns+` FROM plugin_update_jobs
		 WHERE text_diff IS NOT NULL AND created_at >= ?
		 ORDER BY id DESC`,
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list text diff jobs: %w", err)
	}
	return scanUpdateJobList(rows, true)
}

// ListQueuedUpdateJobs returns the jobs still waiting to run, oldest first.
func ListQueuedUpdateJobs() ([]models.UpdateJob, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(
		`SELECT `+updateJobColumns+` FROM plugin_update_jobs WHERE status = ? ORDER BY id`,
		models.UpdateJobQueued,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list queued update jobs: %w", err)
	}
	return scanUpdateJobs(rows)
}

// StartUpdateJob marks a queued job as running.
func StartUpdateJob(id int64) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(
		`UPDATE plugin_update_jobs SET status = ?, started_at = ? WHERE id = ?`,
		models.UpdateJobRunning, time.Now(), id,
	)
	return err
}

// StartUpdateJobBatch marks queued jobs that run together as running and
// records the batch they ran in (the first job's ID).
func StartUpdateJobBatch(ids []int64) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	if len(ids) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now()
	for _, id := range ids {
		if _, err := tx.Exec(
			`UPDATE plugin_update_jobs SET status = ?, started_at = ?, batch_id = ? WHERE id = ?`,
			models.UpdateJobRunning, now, ids[0], id,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetUpdateJobPriority changes a queued job's priority. It reports false
// if the job doesn't exist or is no longer queued.
func SetUpdateJobPriority(id int64, priority models.UpdateJobPriority) (bool, error) {
	db := GetAPIDB()
	if db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	res, err := db.Exec(
		`UPDATE plugin_update_jobs SET priority = ? WHERE id = ? AND status = ?`,
		int(priority), id, models.UpdateJobQueued,
	)
	if err != nil {
		return false, fmt.Errorf("failed to set priority of job %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// FinishUpdateJob records a job's final status, results, job-level error
// and (for core jobs) the refreshed core state.
func FinishUpdateJob(id int64, status string, results []models.UpdateResult, core *models.SiteCore, errMsg string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	encoded, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("failed to encode job results: %w", err)
	}
	var coreJSON sql.NullString
	if core != nil {
		b, err := json.Marshal(core)
		if err != nil {
			return fmt.Errorf("failed to encode job core state: %w", err)
		}
		coreJSON = sql.NullString{String: string(b), Valid: true}
	}
	_, err = db.Exec(
		`UPDATE plugin_update_jobs SET status = ?, results = ?, core = ?, error = ?, finished_at = ? WHERE id = ?`,
		status, string(encoded), coreJSON, errMsg, time.Now(), id,
	)
	return err
}

// SetUpdateJobCalls stores the wp-cli calls of a job's failed update
// attempts, with their output.
func SetUpdateJobCalls(id int64, calls []models.WPCLICall) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	b, err := json.Marshal(calls)
	if err != nil {
		return fmt.Errorf("failed to encode job calls: %w", err)
	}
	_, err = db.Exec(`UPDATE plugin_update_jobs SET calls = ? WHERE id = ?`, string(b), id)
	return err
}

// SetUpdateJobTextDiff stores a job's text diff.
func SetUpdateJobTextDiff(id int64, diff models.TextDiff) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	b, err := json.Marshal(diff)
	if err != nil {
		return fmt.Errorf("failed to encode text diff: %w", err)
	}
	_, err = db.Exec(`UPDATE plugin_update_jobs SET text_diff = ? WHERE id = ?`, string(b), id)
	if err != nil {
		return fmt.Errorf("failed to save text diff of job %d: %w", id, err)
	}
	return nil
}

// InterruptRunningTextDiffs marks text diffs left running by a previous
// jman-api process as interrupted. Their jobs may already be finished: the
// after fetch runs once the update is done.
func InterruptRunningTextDiffs() error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(
		`UPDATE plugin_update_jobs SET text_diff = json_set(text_diff, '$.status', ?)
		 WHERE text_diff IS NOT NULL AND json_extract(text_diff, '$.status') = ?`,
		models.TextDiffInterrupted, models.TextDiffRunning,
	)
	return err
}

// HasActiveCoreUpdateJob reports whether a core update job is queued or
// running for the site.
func HasActiveCoreUpdateJob(siteID string) (bool, error) {
	db := GetAPIDB()
	if db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM plugin_update_jobs WHERE site_id = ? AND kind = ? AND status IN (?, ?)`,
		siteID, models.UpdateJobKindCore, models.UpdateJobQueued, models.UpdateJobRunning,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("failed to check core update jobs: %w", err)
	}
	return n > 0, nil
}

// InterruptRunningUpdateJobs marks every job left running by a
// previous jman-api process as interrupted and returns them.
func InterruptRunningUpdateJobs(reason string) ([]models.UpdateJob, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(
		`SELECT `+updateJobColumns+` FROM plugin_update_jobs WHERE status = ?`,
		models.UpdateJobRunning,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list running update jobs: %w", err)
	}
	jobs, err := scanUpdateJobs(rows)
	if err != nil {
		return nil, err
	}
	for i := range jobs {
		if err := FinishUpdateJob(jobs[i].ID, models.UpdateJobInterrupted, nil, nil, reason); err != nil {
			return nil, err
		}
		jobs[i].Status = models.UpdateJobInterrupted
		jobs[i].Error = reason
	}
	return jobs, nil
}

// PruneFinishedUpdateJobs deletes jobs that finished before cutoff.
// Their outcome is kept in the site update ledger.
func PruneFinishedUpdateJobs(cutoff time.Time) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(`DELETE FROM plugin_update_jobs WHERE finished_at < ?`, cutoff)
	return err
}

func scanUpdateJobs(rows *sql.Rows) ([]models.UpdateJob, error) {
	return scanUpdateJobList(rows, false)
}

// listTextDiff is the part of a TextDiff its summary needs: decoding only
// this skips the pages' text, the bulk of a diff.
type listTextDiff struct {
	Status string `json:"status"`
	Pages  []struct {
		Outcome string `json:"outcome"`
	} `json:"pages"`
}

// scanUpdateJobList scans jobs selected with updateJobColumns, or with
// listUpdateJobColumns if list is set.
func scanUpdateJobList(rows *sql.Rows, list bool) ([]models.UpdateJob, error) {
	defer rows.Close()

	jobs := []models.UpdateJob{}
	for rows.Next() {
		var job models.UpdateJob
		var payload string
		var uploadPath, results, core, textDiff, calls, errMsg, createdBy sql.NullString
		var startedAt, finishedAt sql.NullTime
		var priority int
		var batchID sql.NullInt64
		if err := rows.Scan(
			&job.ID, &job.Kind, &job.SiteID, &job.Status, &payload, &uploadPath, &results, &core, &textDiff, &calls, &errMsg,
			&priority, &batchID, &createdBy, &job.CreatedAt, &startedAt, &finishedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan update job: %w", err)
		}
		job.Payload = json.RawMessage(payload)
		job.Priority = models.UpdateJobPriority(priority)
		job.BatchID = batchID.Int64
		job.Results = []models.UpdateResult{}
		if results.Valid && results.String != "" && results.String != "null" {
			if err := json.Unmarshal([]byte(results.String), &job.Results); err != nil {
				return nil, fmt.Errorf("invalid results in job %d: %w", job.ID, err)
			}
		}
		if core.Valid && core.String != "" {
			job.Core = &models.SiteCore{}
			if err := json.Unmarshal([]byte(core.String), job.Core); err != nil {
				return nil, fmt.Errorf("invalid core state in job %d: %w", job.ID, err)
			}
		}
		switch {
		case !textDiff.Valid || textDiff.String == "":
		case list:
			var d listTextDiff
			if err := json.Unmarshal([]byte(textDiff.String), &d); err != nil {
				return nil, fmt.Errorf("invalid text diff in job %d: %w", job.ID, err)
			}
			td := models.TextDiff{Status: d.Status, Pages: make([]models.TextDiffPage, len(d.Pages))}
			for i, p := range d.Pages {
				td.Pages[i].Outcome = p.Outcome
			}
			summary := td.Summary()
			job.TextDiffSummary = &summary
		default:
			job.TextDiff = &models.TextDiff{}
			if err := json.Unmarshal([]byte(textDiff.String), job.TextDiff); err != nil {
				return nil, fmt.Errorf("invalid text diff in job %d: %w", job.ID, err)
			}
			summary := job.TextDiff.Summary()
			job.TextDiffSummary = &summary
		}
		if list {
			job.HasOutput = calls.Valid
		} else if calls.Valid && calls.String != "" && calls.String != "null" {
			if err := json.Unmarshal([]byte(calls.String), &job.Calls); err != nil {
				return nil, fmt.Errorf("invalid calls in job %d: %w", job.ID, err)
			}
			job.HasOutput = len(job.Calls) > 0
		}
		job.UploadPath = uploadPath.String
		job.Error = errMsg.String
		job.CreatedBy = createdBy.String
		if startedAt.Valid {
			job.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			job.FinishedAt = &finishedAt.Time
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate update jobs: %w", err)
	}
	return jobs, nil
}

// ListUpdateJobUploadPaths returns the upload paths of jobs that haven't
// finished, so leftover uploads of finished or deleted jobs can be removed.
func ListUpdateJobUploadPaths() (map[string]bool, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.Query(
		`SELECT upload_path FROM plugin_update_jobs WHERE upload_path IS NOT NULL AND upload_path != '' AND status IN (?, ?)`,
		models.UpdateJobQueued, models.UpdateJobRunning,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list job uploads: %w", err)
	}
	defer rows.Close()
	paths := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths[p] = true
	}
	return paths, rows.Err()
}
