package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

// maxChecklistItemText caps one checklist line.
const maxChecklistItemText = 1000

// validChecklistItems trims and checks new checklist items: each needs text,
// and a ref_type needs a ref_id.
func validChecklistItems(items []models.TaskChecklistItem) ([]models.TaskChecklistItem, error) {
	out := make([]models.TaskChecklistItem, 0, len(items))
	for _, it := range items {
		text := strings.TrimSpace(it.Text)
		if text == "" {
			return nil, errors.New("checklist items need text")
		}
		if len(text) > maxChecklistItemText {
			return nil, errors.New("checklist item text is too long")
		}
		item := models.TaskChecklistItem{Text: text}
		if it.RefType != "" {
			if !models.IsValidTaskChecklistRefType(it.RefType) {
				return nil, errors.New("invalid checklist ref_type")
			}
			if strings.TrimSpace(it.RefID) == "" {
				return nil, errors.New("checklist ref_type needs a ref_id")
			}
			item.RefType, item.RefID = it.RefType, strings.TrimSpace(it.RefID)
		}
		out = append(out, item)
	}
	return out, nil
}

// taskFromPath reads the {id} path value and loads that task, writing the
// error response and returning nil when there's none.
func taskFromPath(w http.ResponseWriter, r *http.Request) *models.Task {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid task ID")
		return nil
	}
	task, err := db.GetTask(id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Database error")
		return nil
	}
	if task == nil {
		WriteError(w, http.StatusNotFound, "Task not found")
		return nil
	}
	return task
}

// writeChecklist responds with the task's whole checklist, so the client
// never has to piece it together.
func writeChecklist(w http.ResponseWriter, status, taskID int) {
	items, err := db.GetTaskChecklist(taskID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Database error")
		return
	}
	WriteJSON(w, status, items)
}

// AddTaskChecklistItemsHandler handles POST /api/tasks/{id}/checklist with
// {"items": [{"text": ..., "ref_type": ..., "ref_id": ...}, ...]}, appending
// the items to the checklist.
func AddTaskChecklistItemsHandler(w http.ResponseWriter, r *http.Request) {
	task := taskFromPath(w, r)
	if task == nil {
		return
	}
	var body struct {
		Items []models.TaskChecklistItem `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(body.Items) == 0 {
		WriteError(w, http.StatusBadRequest, "No items given")
		return
	}
	items, err := validChecklistItems(body.Items)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := db.AddTaskChecklistItems(task.ID, items, getUsername(r)); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to add checklist items")
		return
	}
	writeChecklist(w, http.StatusCreated, task.ID)
}

// UpdateTaskChecklistItemHandler handles PATCH
// /api/tasks/{id}/checklist/{itemId} with text and/or done.
func UpdateTaskChecklistItemHandler(w http.ResponseWriter, r *http.Request) {
	task := taskFromPath(w, r)
	if task == nil {
		return
	}
	itemID, err := strconv.Atoi(r.PathValue("itemId"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid item ID")
		return
	}
	var body struct {
		Text *string `json:"text"`
		Done *bool   `json:"done"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	item, err := db.GetTaskChecklistItem(task.ID, itemID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Database error")
		return
	}
	if item == nil {
		WriteError(w, http.StatusNotFound, "Checklist item not found")
		return
	}
	if body.Text != nil {
		valid, err := validChecklistItems([]models.TaskChecklistItem{{Text: *body.Text}})
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		item.Text = valid[0].Text
	}
	if body.Done != nil && *body.Done != item.Done {
		item.Done = *body.Done
		item.DoneAt, item.DoneBy = nil, nil
	}
	if err := db.UpdateTaskChecklistItem(item, getUsername(r)); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to update checklist item")
		return
	}
	writeChecklist(w, http.StatusOK, task.ID)
}

// ReorderTaskChecklistHandler handles PUT /api/tasks/{id}/checklist/order
// with {"ids": [...]}, the item IDs in their new order.
func ReorderTaskChecklistHandler(w http.ResponseWriter, r *http.Request) {
	task := taskFromPath(w, r)
	if task == nil {
		return
	}
	var body struct {
		IDs []int `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := db.ReorderTaskChecklist(task.ID, body.IDs); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to reorder checklist")
		return
	}
	writeChecklist(w, http.StatusOK, task.ID)
}

// DeleteTaskChecklistItemHandler handles DELETE
// /api/tasks/{id}/checklist/{itemId}.
func DeleteTaskChecklistItemHandler(w http.ResponseWriter, r *http.Request) {
	task := taskFromPath(w, r)
	if task == nil {
		return
	}
	itemID, err := strconv.Atoi(r.PathValue("itemId"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid item ID")
		return
	}
	if err := db.DeleteTaskChecklistItem(task.ID, itemID); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to delete checklist item")
		return
	}
	writeChecklist(w, http.StatusOK, task.ID)
}

// WatchTaskHandler handles PUT (watch) and DELETE (unwatch)
// /api/tasks/{id}/watch for the signed-in user, responding with the task's
// watchers.
func WatchTaskHandler(w http.ResponseWriter, r *http.Request) {
	task := taskFromPath(w, r)
	if task == nil {
		return
	}
	username := getUsername(r)
	var err error
	if r.Method == http.MethodDelete {
		err = db.UnwatchTask(task.ID, username)
	} else {
		err = db.WatchTask(task.ID, username)
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to update watchers")
		return
	}
	watchers, err := db.GetTaskWatchers(task.ID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Database error")
		return
	}
	WriteJSON(w, http.StatusOK, watchers)
}
