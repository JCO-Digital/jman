package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/tasks"
)

// ListTasksHandler handles GET /api/tasks
func ListTasksHandler(w http.ResponseWriter, r *http.Request) {
	filter := db.TaskFilter{
		Status:      models.TaskStatus(r.URL.Query().Get("status")),
		Priority:    models.TaskPriority(r.URL.Query().Get("priority")),
		AssignedTo:  r.URL.Query().Get("assigned_to"),
		CompletedBy: r.URL.Query().Get("completed_by"),
		Search:      r.URL.Query().Get("search"),
	}

	if sid := r.URL.Query().Get("site_id"); sid != "" {
		filter.SiteID, _ = strconv.Atoi(sid)
	}
	if oid := r.URL.Query().Get("organization_id"); oid != "" {
		filter.OrganizationID, _ = strconv.Atoi(oid)
	}
	if svid := r.URL.Query().Get("server_id"); svid != "" {
		filter.ServerID, _ = strconv.Atoi(svid)
	}

	tasks, err := db.GetTasks(filter)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to retrieve tasks")
		return
	}

	WriteJSON(w, http.StatusOK, tasks)
}

// GetTaskHandler handles GET /api/tasks/{id}
func GetTaskHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid task ID")
		return
	}

	task, err := db.GetTask(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if task == nil {
		WriteError(w, http.StatusNotFound, "Task not found")
		return
	}

	WriteJSON(w, http.StatusOK, task)
}

// CreateTaskHandler handles POST /api/tasks
func CreateTaskHandler(w http.ResponseWriter, r *http.Request) {
	claims := GetAuthClaims(r.Context())
	var task models.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if task.Title == "" {
		WriteError(w, http.StatusBadRequest, "Title is required")
		return
	}

	if task.Status == "" {
		task.Status = models.TaskStatusPending
	}
	if task.Priority == "" {
		task.Priority = models.TaskPriorityMedium
	}
	if task.Type == "" {
		task.Type = models.TaskTypeOneTime
	}

	if err := db.SaveTask(&task, claims.Username); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to create task")
		return
	}

	if task.AssignedTo != nil && *task.AssignedTo != "" {
		taskCopy := task
		go tasks.NotifyTaskAssigned(&taskCopy)
	}

	WriteJSON(w, http.StatusCreated, task)
}

// UpdateTaskHandler handles PATCH /api/tasks/{id}
func UpdateTaskHandler(w http.ResponseWriter, r *http.Request) {
	claims := GetAuthClaims(r.Context())
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid task ID")
		return
	}

	existing, err := db.GetTask(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if existing == nil {
		WriteError(w, http.StatusNotFound, "Task not found")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to read request body")
		return
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request JSON")
		return
	}

	var updates models.Task
	if err := json.Unmarshal(body, &updates); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	// Helper to check for key presence in JSON
	has := func(key string) bool {
		_, ok := raw[key]
		return ok
	}

	if has("title") {
		if updates.Title == "" {
			WriteError(w, http.StatusBadRequest, "Title cannot be empty")
			return
		}
		existing.Title = updates.Title
	}
	if has("description") {
		existing.Description = updates.Description
	}
	if has("status") {
		if updates.Status == "" {
			WriteError(w, http.StatusBadRequest, "Status cannot be empty")
			return
		}
		existing.Status = updates.Status
	}
	if has("priority") {
		if updates.Priority == "" {
			WriteError(w, http.StatusBadRequest, "Priority cannot be empty")
			return
		}
		existing.Priority = updates.Priority
	}
	if has("assigned_to") {
		// Reset LastNotifiedAt if the assignee changes, so the new user gets a reminder
		oldAssignee := existing.AssignedTo
		newAssignee := updates.AssignedTo

		isChanged := (oldAssignee == nil && newAssignee != nil) ||
			(oldAssignee != nil && newAssignee == nil) ||
			(oldAssignee != nil && newAssignee != nil && *oldAssignee != *newAssignee)

		if isChanged {
			existing.LastNotifiedAt = nil
		}
		existing.AssignedTo = updates.AssignedTo

		// If a new user is assigned, send a notification
		if isChanged && newAssignee != nil && *newAssignee != "" {
			// Pass a copy to the goroutine to avoid data races
			taskCopy := *existing
			go tasks.NotifyTaskAssigned(&taskCopy)
		}
	}
	if has("due_date") {
		existing.DueDate = updates.DueDate
	}
	if has("reminder_date") {
		existing.ReminderDate = updates.ReminderDate
	}
	if has("metadata") {
		existing.Metadata = updates.Metadata
	}

	if err := db.SaveTask(existing, claims.Username); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to update task")
		return
	}

	WriteJSON(w, http.StatusOK, existing)
}

// CompleteTaskHandler handles POST /api/tasks/{id}/complete
func CompleteTaskHandler(w http.ResponseWriter, r *http.Request) {
	claims := GetAuthClaims(r.Context())
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid task ID")
		return
	}

	if err := db.CompleteTask(id, claims.Username); err != nil {
		if errors.Is(err, db.ErrTaskNotFound) {
			WriteError(w, http.StatusNotFound, "Task not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "Failed to complete task")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "completed"})
}

// DeleteTaskHandler handles DELETE /api/tasks/{id}
func DeleteTaskHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid task ID")
		return
	}

	if err := db.DeleteTask(id); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to delete task")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
