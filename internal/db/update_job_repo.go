package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

const updateJobColumns = `id, kind, site_id, status, plugins, target, source, activate, upload_path, results, core, error, created_by, created_at, started_at, finished_at`

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
	if job.Plugins == nil {
		job.Plugins = []models.PluginUpdateRequest{}
	}
	plugins, err := json.Marshal(job.Plugins)
	if err != nil {
		return fmt.Errorf("failed to encode job plugins: %w", err)
	}
	job.Status = models.UpdateJobQueued
	job.CreatedAt = time.Now()
	job.Results = []models.UpdateResult{}

	err = db.QueryRow(
		`INSERT INTO plugin_update_jobs (kind, site_id, status, plugins, target, source, activate, upload_path, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		job.Kind, job.SiteID, job.Status, string(plugins), job.Target, job.Source, job.Activate, job.UploadPath, job.CreatedBy, job.CreatedAt,
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

// ListUpdateJobs returns every queued or running job plus those that
// finished at or after finishedSince, oldest first.
func ListUpdateJobs(finishedSince time.Time) ([]models.UpdateJob, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(
		`SELECT `+updateJobColumns+` FROM plugin_update_jobs
		 WHERE status IN (?, ?) OR finished_at >= ?
		 ORDER BY id`,
		models.UpdateJobQueued, models.UpdateJobRunning, finishedSince,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list update jobs: %w", err)
	}
	return scanUpdateJobs(rows)
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
	defer rows.Close()

	jobs := []models.UpdateJob{}
	for rows.Next() {
		var job models.UpdateJob
		var plugins string
		var target, source, uploadPath, results, core, errMsg, createdBy sql.NullString
		var startedAt, finishedAt sql.NullTime
		if err := rows.Scan(
			&job.ID, &job.Kind, &job.SiteID, &job.Status, &plugins, &target, &source, &job.Activate, &uploadPath, &results, &core, &errMsg, &createdBy,
			&job.CreatedAt, &startedAt, &finishedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan update job: %w", err)
		}
		if err := json.Unmarshal([]byte(plugins), &job.Plugins); err != nil {
			return nil, fmt.Errorf("invalid plugins in job %d: %w", job.ID, err)
		}
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
		job.Target = target.String
		job.Source = source.String
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
