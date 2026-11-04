package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// ErrAIRunNotFound is returned for an unknown AI run ID.
var ErrAIRunNotFound = errors.New("AI run not found")

const aiRunColumns = `id, agent, agent_version, model, status, input, batch, batch_id, prompt_state, output, error, stop_reason,
	refusal_category, request_hash, reused_from, turns, input_tokens, output_tokens,
	cache_read_input_tokens, cache_creation_input_tokens, cost_micros, requested_by, triggered_by,
	created_at, started_at, finished_at`

func apiDBOrErr() (*APIDB, error) {
	dbConn := GetAPIDB()
	if dbConn == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return dbConn, nil
}

// InsertAIRun stores a new run.
func InsertAIRun(r *models.AIRun) error {
	dbConn, err := apiDBOrErr()
	if err != nil {
		return err
	}
	input := string(r.Input)
	if input == "" {
		input = "{}"
	}
	_, err = dbConn.Exec(`INSERT INTO ai_runs (id, agent, agent_version, model, status, input, batch, requested_by,
		triggered_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Agent, r.AgentVersion, r.Model, r.Status, input, r.Batch, r.RequestedBy, r.Trigger, r.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert AI run: %w", err)
	}
	return nil
}

// ClaimNextAIRun marks the oldest queued run running and returns it, or
// nil when none is queued.
func ClaimNextAIRun(now time.Time) (*models.AIRun, error) {
	dbConn, err := apiDBOrErr()
	if err != nil {
		return nil, err
	}
	row := dbConn.QueryRow(`UPDATE ai_runs SET status = ?, started_at = ?
		WHERE id = (SELECT id FROM ai_runs WHERE status = ? ORDER BY created_at, id LIMIT 1)
		RETURNING `+aiRunColumns, models.AIRunRunning, now, models.AIRunQueued)
	r, err := scanAIRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

// RequeueRunningAIRuns puts runs left running by a stopped jman-api back in
// the queue. Agents only read, so running one again is safe. Runs waiting
// for a submitted batch stay running: the batch is still collected.
func RequeueRunningAIRuns() (int64, error) {
	dbConn, err := apiDBOrErr()
	if err != nil {
		return 0, err
	}
	res, err := dbConn.Exec(`UPDATE ai_runs SET status = ?, started_at = NULL WHERE status = ? AND batch_id = ''`,
		models.AIRunQueued, models.AIRunRunning)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// FinishAIRun stores a run's result. A run canceled meanwhile stays
// canceled but still gets its usage and cost, which count toward the daily
// budget. It reports whether the run was updated.
func FinishAIRun(r *models.AIRun) (bool, error) {
	dbConn, err := apiDBOrErr()
	if err != nil {
		return false, err
	}
	var output any
	if len(r.Output) > 0 {
		output = string(r.Output)
	}
	res, err := dbConn.Exec(`UPDATE ai_runs SET
		status = CASE WHEN status = ? THEN status ELSE ? END,
		batch = ?, batch_id = ?, prompt_state = NULL,
		output = CASE WHEN status = ? THEN output ELSE ? END,
		error = CASE WHEN status = ? THEN error ELSE ? END,
		model = ?, stop_reason = ?, refusal_category = ?, request_hash = ?, reused_from = ?, turns = ?,
		input_tokens = ?, output_tokens = ?, cache_read_input_tokens = ?, cache_creation_input_tokens = ?,
		cost_micros = ?, finished_at = COALESCE(finished_at, ?)
		WHERE id = ? AND status IN (?, ?, ?)`,
		models.AIRunCanceled, r.Status, r.Batch, r.BatchID, models.AIRunCanceled, output, models.AIRunCanceled, r.Error,
		r.Model, r.StopReason, r.RefusalCategory, r.RequestHash, r.ReusedFrom, r.Turns,
		r.Usage.InputTokens, r.Usage.OutputTokens, r.Usage.CacheReadInputTokens, r.Usage.CacheCreationInputTokens,
		r.CostMicros, r.FinishedAt, r.ID, models.AIRunQueued, models.AIRunRunning, models.AIRunCanceled)
	if err != nil {
		return false, fmt.Errorf("failed to finish AI run: %w", err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// SetAIRunBatch records that a running run's request was submitted as a
// batch, with what collecting it needs. It reports whether the run was
// still running.
func SetAIRunBatch(r *models.AIRun) (bool, error) {
	dbConn, err := apiDBOrErr()
	if err != nil {
		return false, err
	}
	var state any
	if len(r.PromptState) > 0 {
		state = string(r.PromptState)
	}
	res, err := dbConn.Exec(`UPDATE ai_runs SET batch = 1, batch_id = ?, prompt_state = ?, model = ?, request_hash = ?
		WHERE id = ? AND status = ?`, r.BatchID, state, r.Model, r.RequestHash, r.ID, models.AIRunRunning)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ListPendingAIBatchRuns returns the running runs waiting for a batch,
// oldest first.
func ListPendingAIBatchRuns() ([]models.AIRun, error) {
	dbConn, err := apiDBOrErr()
	if err != nil {
		return nil, err
	}
	rows, err := dbConn.Query(`SELECT `+aiRunColumns+` FROM ai_runs WHERE status = ? AND batch_id != ''
		ORDER BY created_at`, models.AIRunRunning)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []models.AIRun
	for rows.Next() {
		r, err := scanAIRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *r)
	}
	return runs, rows.Err()
}

// CancelAIRun marks a queued or running run canceled and reports whether
// it was.
func CancelAIRun(id string, now time.Time) (bool, error) {
	dbConn, err := apiDBOrErr()
	if err != nil {
		return false, err
	}
	res, err := dbConn.Exec(`UPDATE ai_runs SET status = ?, finished_at = ? WHERE id = ? AND status IN (?, ?)`,
		models.AIRunCanceled, now, id, models.AIRunQueued, models.AIRunRunning)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// GetAIRun returns a run with its tool calls.
func GetAIRun(id string) (*models.AIRun, error) {
	dbConn, err := apiDBOrErr()
	if err != nil {
		return nil, err
	}
	r, err := scanAIRun(dbConn.QueryRow(`SELECT `+aiRunColumns+` FROM ai_runs WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAIRunNotFound
	}
	if err != nil {
		return nil, err
	}
	r.Steps, err = ListAIRunSteps(id)
	return r, err
}

// AIRunFilter selects runs for ListAIRuns. Empty fields match everything.
type AIRunFilter struct {
	Agent  string
	Status string
	Limit  int
}

// ListAIRuns returns runs, newest first, without their tool calls.
func ListAIRuns(f AIRunFilter) ([]models.AIRun, error) {
	dbConn, err := apiDBOrErr()
	if err != nil {
		return nil, err
	}
	var where []string
	var args []any
	if f.Agent != "" {
		where = append(where, "agent = ?")
		args = append(args, f.Agent)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	q := `SELECT ` + aiRunColumns + ` FROM ai_runs`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	q += " ORDER BY created_at DESC, id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := dbConn.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []models.AIRun{}
	for rows.Next() {
		r, err := scanAIRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *r)
	}
	return runs, rows.Err()
}

// FindReusableAIRun returns the newest run of agent and version that
// succeeded with requestHash after since without reusing another run, or
// nil.
func FindReusableAIRun(agent, version, requestHash string, since time.Time) (*models.AIRun, error) {
	dbConn, err := apiDBOrErr()
	if err != nil {
		return nil, err
	}
	r, err := scanAIRun(dbConn.QueryRow(`SELECT `+aiRunColumns+` FROM ai_runs
		WHERE agent = ? AND agent_version = ? AND request_hash = ? AND status = ? AND reused_from = ''
			AND finished_at >= ?
		ORDER BY finished_at DESC LIMIT 1`, agent, version, requestHash, models.AIRunSucceeded, since))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

// LastAIRunCreatedAt returns when agent last got a run by trigger, or nil.
// Runs that failed or were canceled before they cost anything don't count,
// so they are retried soon; a run that spent money waits its interval.
func LastAIRunCreatedAt(agent, trigger string) (*time.Time, error) {
	dbConn, err := apiDBOrErr()
	if err != nil {
		return nil, err
	}
	var t sql.NullTime
	err = dbConn.QueryRow(`SELECT MAX(created_at) FROM ai_runs WHERE agent = ? AND triggered_by = ?
		AND NOT (status IN (?, ?) AND cost_micros = 0)`, agent, trigger, models.AIRunFailed, models.AIRunCanceled).Scan(&t)
	if err != nil || !t.Valid {
		return nil, err
	}
	return &t.Time, nil
}

// SumAICostSince returns the estimated cost of the runs created at or after
// since, in millionths of a US dollar. Running runs count with what they
// have spent so far only once they finish.
func SumAICostSince(since time.Time) (int64, error) {
	dbConn, err := apiDBOrErr()
	if err != nil {
		return 0, err
	}
	var total sql.NullInt64
	err = dbConn.QueryRow(`SELECT SUM(cost_micros) FROM ai_runs WHERE created_at >= ?`, since).Scan(&total)
	return total.Int64, err
}

// InsertAIRunStep stores one tool call of a run.
func InsertAIRunStep(runID string, s models.AIRunStep) error {
	dbConn, err := apiDBOrErr()
	if err != nil {
		return err
	}
	input := string(s.Input)
	if input == "" {
		input = "{}"
	}
	_, err = dbConn.Exec(`INSERT INTO ai_run_steps (run_id, seq, turn, tool, input, output, is_error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, runID, s.Seq, s.Turn, s.Tool, input, s.Output, s.IsError, s.CreatedAt)
	return err
}

// ListAIRunSteps returns a run's tool calls in order.
func ListAIRunSteps(runID string) ([]models.AIRunStep, error) {
	dbConn, err := apiDBOrErr()
	if err != nil {
		return nil, err
	}
	rows, err := dbConn.Query(`SELECT seq, turn, tool, input, output, is_error, created_at FROM ai_run_steps
		WHERE run_id = ? ORDER BY seq`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var steps []models.AIRunStep
	for rows.Next() {
		var s models.AIRunStep
		var input string
		if err := rows.Scan(&s.Seq, &s.Turn, &s.Tool, &input, &s.Output, &s.IsError, &s.CreatedAt); err != nil {
			return nil, err
		}
		s.Input = json.RawMessage(input)
		steps = append(steps, s)
	}
	return steps, rows.Err()
}

func scanAIRun(row rowScanner) (*models.AIRun, error) {
	var r models.AIRun
	var input string
	var output, state sql.NullString
	var started, finished sql.NullTime
	err := row.Scan(&r.ID, &r.Agent, &r.AgentVersion, &r.Model, &r.Status, &input, &r.Batch, &r.BatchID, &state,
		&output, &r.Error,
		&r.StopReason, &r.RefusalCategory, &r.RequestHash, &r.ReusedFrom, &r.Turns,
		&r.Usage.InputTokens, &r.Usage.OutputTokens, &r.Usage.CacheReadInputTokens,
		&r.Usage.CacheCreationInputTokens, &r.CostMicros, &r.RequestedBy, &r.Trigger,
		&r.CreatedAt, &started, &finished)
	if err != nil {
		return nil, err
	}
	r.Input = json.RawMessage(input)
	if state.Valid {
		r.PromptState = json.RawMessage(state.String)
	}
	if output.Valid {
		r.Output = json.RawMessage(output.String)
	}
	if started.Valid {
		t := started.Time
		r.StartedAt = &t
	}
	if finished.Valid {
		t := finished.Time
		r.FinishedAt = &t
	}
	return &r, nil
}
