package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/JCO-Digital/jman/internal/ai"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/verb"
)

// maxAIRunWait bounds the wait parameter of POST /api/ai/agents/{name}/runs.
const maxAIRunWait = 2 * time.Minute

// AIStatusHandler returns whether the AI gateway runs agents and today's
// estimated spend.
func AIStatusHandler(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, ai.Default.Status())
}

// ListAIAgentsHandler returns the AI agents.
func ListAIAgentsHandler(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, ai.Default.Agents())
}

// CreateAIRunHandler queues a run of the agent in the path. The body is
// optional: {"input": {...}, "batch": true}. With ?wait=<seconds> (at most 120) it waits
// for the run to finish and answers 200 with the finished run, or 202 with
// the run still queued or running. Without it, it answers 202 at once.
func CreateAIRunHandler(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !ai.Default.HasAgent(name) {
		WriteError(w, http.StatusNotFound, "Unknown agent")
		return
	}

	wait := time.Duration(0)
	if v := r.URL.Query().Get("wait"); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs < 0 || time.Duration(secs)*time.Second > maxAIRunWait {
			WriteError(w, http.StatusBadRequest, "wait must be a number of seconds from 0 to 120")
			return
		}
		wait = time.Duration(secs) * time.Second
	}

	var req models.AIRunRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}

	run, err := ai.Default.Enqueue(name, req.Input, req.Batch, getUsername(r), models.AIRunTriggerAPI)
	var inputErr *ai.InputError
	switch {
	case errors.As(err, &inputErr):
		WriteError(w, http.StatusBadRequest, inputErr.Error())
		return
	case errors.Is(err, ai.ErrDisabled):
		WriteError(w, http.StatusServiceUnavailable, err.Error())
		return
	case err != nil:
		verb.LogPrintf(verb.Normal, "Failed to queue AI run: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to queue the run")
		return
	}

	if wait > 0 {
		if waited, err := ai.Default.Wait(r.Context(), run.ID, wait); err == nil {
			run = waited
		}
	}
	status := http.StatusAccepted
	if models.AIRunFinished(run.Status) {
		status = http.StatusOK
	}
	WriteJSON(w, status, run)
}

// ListAIRunsHandler lists AI runs, newest first, without their tool calls.
// Query params: agent, status, limit (default 50, max 500).
func ListAIRunsHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := db.AIRunFilter{Agent: q.Get("agent"), Status: q.Get("status")}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			WriteError(w, http.StatusBadRequest, "limit must be between 1 and 500")
			return
		}
		f.Limit = n
	}
	runs, err := db.ListAIRuns(f)
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to list AI runs: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to list runs")
		return
	}
	WriteJSON(w, http.StatusOK, runs)
}

// GetAIRunHandler returns one AI run with its tool calls.
func GetAIRunHandler(w http.ResponseWriter, r *http.Request) {
	run, err := db.GetAIRun(r.PathValue("id"))
	if errors.Is(err, db.ErrAIRunNotFound) {
		WriteError(w, http.StatusNotFound, "Run not found")
		return
	}
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to get AI run: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to get the run")
		return
	}
	WriteJSON(w, http.StatusOK, run)
}

// CancelAIRunHandler cancels a queued or running AI run.
func CancelAIRunHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ok, err := ai.Default.Cancel(id)
	if err != nil {
		verb.LogPrintf(verb.Normal, "Failed to cancel AI run: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to cancel the run")
		return
	}
	run, getErr := db.GetAIRun(id)
	if errors.Is(getErr, db.ErrAIRunNotFound) {
		WriteError(w, http.StatusNotFound, "Run not found")
		return
	}
	if !ok {
		WriteError(w, http.StatusConflict, "The run has already finished")
		return
	}
	if getErr != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to get the run")
		return
	}
	WriteJSON(w, http.StatusOK, run)
}
