package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

// ListTasksHandler handles GET /api/tasks
func ListTasksHandler(w http.ResponseWriter, r *http.Request) {
	filter := db.TaskFilter{
		Status:     models.TaskStatus(r.URL.Query().Get("status")),
		Priority:   models.TaskPriority(r.URL.Query().Get("priority")),
		AssignedTo: r.URL.Query().Get("assigned_to"),
		Search:     r.URL.Query().Get("search"),
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

	var updates models.Task
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Simple field updates
	if updates.Title != "" {
		existing.Title = updates.Title
	}
	if updates.Description != nil {
		existing.Description = updates.Description
	}
	if updates.Status != "" {
		existing.Status = updates.Status
	}
	if updates.Priority != "" {
		existing.Priority = updates.Priority
	}
	if updates.AssignedTo != nil {
		existing.AssignedTo = updates.AssignedTo
	}
	if updates.DueDate != nil {
		existing.DueDate = updates.DueDate
	}
	if updates.ReminderDate != nil {
		existing.ReminderDate = updates.ReminderDate
	}
	if updates.Metadata != nil {
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
