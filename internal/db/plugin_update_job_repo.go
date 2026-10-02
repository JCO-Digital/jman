package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

const pluginUpdateJobColumns = `id, site_id, status, plugins, results, error, created_by, created_at, started_at, finished_at`

// CreatePluginUpdateJob stores a new queued job and sets its ID, Status
// and CreatedAt.
func CreatePluginUpdateJob(job *models.PluginUpdateJob) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	plugins, err := json.Marshal(job.Plugins)
	if err != nil {
		return fmt.Errorf("failed to encode job plugins: %w", err)
	}
	job.Status = models.PluginUpdateJobQueued
	job.CreatedAt = time.Now()
	job.Results = []models.PluginUpdateResult{}

	err = db.QueryRow(
		`INSERT INTO plugin_update_jobs (site_id, status, plugins, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?) RETURNING id`,
		job.SiteID, job.Status, string(plugins), job.CreatedBy, job.CreatedAt,
	).Scan(&job.ID)
	if err != nil {
		return fmt.Errorf("failed to create plugin update job: %w", err)
	}
	return nil
}

// GetPluginUpdateJob returns a job by ID, or nil if it doesn't exist.
func GetPluginUpdateJob(id int64) (*models.PluginUpdateJob, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT `+pluginUpdateJobColumns+` FROM plugin_update_jobs WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get plugin update job: %w", err)
	}
	jobs, err := scanPluginUpdateJobs(rows)
	if err != nil || len(jobs) == 0 {
		return nil, err
	}
	return &jobs[0], nil
}

// ListPluginUpdateJobs returns every queued or running job plus those that
// finished at or after finishedSince, oldest first.
func ListPluginUpdateJobs(finishedSince time.Time) ([]models.PluginUpdateJob, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(
		`SELECT `+pluginUpdateJobColumns+` FROM plugin_update_jobs
		 WHERE status IN (?, ?) OR finished_at >= ?
		 ORDER BY id`,
		models.PluginUpdateJobQueued, models.PluginUpdateJobRunning, finishedSince,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list plugin update jobs: %w", err)
	}
	return scanPluginUpdateJobs(rows)
}

// ListQueuedPluginUpdateJobs returns the jobs still waiting to run, oldest first.
func ListQueuedPluginUpdateJobs() ([]models.PluginUpdateJob, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(
		`SELECT `+pluginUpdateJobColumns+` FROM plugin_update_jobs WHERE status = ? ORDER BY id`,
		models.PluginUpdateJobQueued,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list queued plugin update jobs: %w", err)
	}
	return scanPluginUpdateJobs(rows)
}

// StartPluginUpdateJob marks a queued job as running.
func StartPluginUpdateJob(id int64) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(
		`UPDATE plugin_update_jobs SET status = ?, started_at = ? WHERE id = ?`,
		models.PluginUpdateJobRunning, time.Now(), id,
	)
	return err
}

// FinishPluginUpdateJob records a job's final status, per-plugin results
// and job-level error.
func FinishPluginUpdateJob(id int64, status string, results []models.PluginUpdateResult, errMsg string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	encoded, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("failed to encode job results: %w", err)
	}
	_, err = db.Exec(
		`UPDATE plugin_update_jobs SET status = ?, results = ?, error = ?, finished_at = ? WHERE id = ?`,
		status, string(encoded), errMsg, time.Now(), id,
	)
	return err
}

// InterruptRunningPluginUpdateJobs marks every job left running by a
// previous jman-api process as interrupted and returns them.
func InterruptRunningPluginUpdateJobs(reason string) ([]models.PluginUpdateJob, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(
		`SELECT `+pluginUpdateJobColumns+` FROM plugin_update_jobs WHERE status = ?`,
		models.PluginUpdateJobRunning,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list running plugin update jobs: %w", err)
	}
	jobs, err := scanPluginUpdateJobs(rows)
	if err != nil {
		return nil, err
	}
	for i := range jobs {
		if err := FinishPluginUpdateJob(jobs[i].ID, models.PluginUpdateJobInterrupted, nil, reason); err != nil {
			return nil, err
		}
		jobs[i].Status = models.PluginUpdateJobInterrupted
		jobs[i].Error = reason
	}
	return jobs, nil
}

// PruneFinishedPluginUpdateJobs deletes jobs that finished before cutoff.
// Their outcome is kept in the site update ledger.
func PruneFinishedPluginUpdateJobs(cutoff time.Time) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec(`DELETE FROM plugin_update_jobs WHERE finished_at < ?`, cutoff)
	return err
}

func scanPluginUpdateJobs(rows *sql.Rows) ([]models.PluginUpdateJob, error) {
	defer rows.Close()

	jobs := []models.PluginUpdateJob{}
	for rows.Next() {
		var job models.PluginUpdateJob
		var plugins string
		var results, errMsg, createdBy sql.NullString
		var startedAt, finishedAt sql.NullTime
		if err := rows.Scan(
			&job.ID, &job.SiteID, &job.Status, &plugins, &results, &errMsg, &createdBy,
			&job.CreatedAt, &startedAt, &finishedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan plugin update job: %w", err)
		}
		if err := json.Unmarshal([]byte(plugins), &job.Plugins); err != nil {
			return nil, fmt.Errorf("invalid plugins in job %d: %w", job.ID, err)
		}
		job.Results = []models.PluginUpdateResult{}
		if results.Valid && results.String != "" && results.String != "null" {
			if err := json.Unmarshal([]byte(results.String), &job.Results); err != nil {
				return nil, fmt.Errorf("invalid results in job %d: %w", job.ID, err)
			}
		}
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
		return nil, fmt.Errorf("failed to iterate plugin update jobs: %w", err)
	}
	return jobs, nil
}
