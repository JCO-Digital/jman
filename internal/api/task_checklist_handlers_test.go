package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

// doTaskRequest runs handler as user alice, with {id}/{itemId} path values.
func doTaskRequest(t *testing.T, method, target, body string, handler http.HandlerFunc, path map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rd)
	for k, v := range path {
		req.SetPathValue(k, v)
	}
	req = req.WithContext(contextWithClaims(context.Background(), &AuthClaims{Username: "alice", Level: config.LevelEdit}))
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

func decodeTask(t *testing.T, rr *httptest.ResponseRecorder) models.Task {
	t.Helper()
	var task models.Task
	if err := json.Unmarshal(rr.Body.Bytes(), &task); err != nil {
		t.Fatalf("decode task: %v (%s)", err, rr.Body.String())
	}
	return task
}

func TestTaskChecklistLifecycle(t *testing.T) {
	setupSettingsTest(t)

	rr := doTaskRequest(t, "POST", "/api/tasks",
		`{"title":"Monthly","type":"repeating","interval":"1m","checklist":[{"text":" Backups "},{"text":"example.com","ref_type":"site","ref_id":"abc"}]}`,
		CreateTaskHandler, nil)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	task := decodeTask(t, rr)
	if len(task.Checklist) != 2 || task.Checklist[0].Text != "Backups" || task.Checklist[1].RefType != models.TaskChecklistRefSite {
		t.Fatalf("checklist = %+v", task.Checklist)
	}
	if len(task.Watchers) != 1 || task.Watchers[0] != "alice" {
		t.Fatalf("creator should watch: %v", task.Watchers)
	}
	id := strconv.Itoa(task.ID)
	first := strconv.Itoa(task.Checklist[0].ID)
	second := strconv.Itoa(task.Checklist[1].ID)

	// Bad items are refused.
	for _, body := range []string{`{"items":[]}`, `{"items":[{"text":"  "}]}`, `{"items":[{"text":"x","ref_type":"bogus","ref_id":"1"}]}`, `{"items":[{"text":"x","ref_type":"site"}]}`} {
		if rr := doTaskRequest(t, "POST", "/", body, AddTaskChecklistItemsHandler, map[string]string{"id": id}); rr.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", body, rr.Code)
		}
	}

	rr = doTaskRequest(t, "POST", "/", `{"items":[{"text":"Report"}]}`, AddTaskChecklistItemsHandler, map[string]string{"id": id})
	var items []models.TaskChecklistItem
	json.Unmarshal(rr.Body.Bytes(), &items)
	if rr.Code != http.StatusCreated || len(items) != 3 || items[2].Text != "Report" {
		t.Fatalf("add: %d %+v", rr.Code, items)
	}
	third := strconv.Itoa(items[2].ID)

	rr = doTaskRequest(t, "PATCH", "/", `{"done":true}`, UpdateTaskChecklistItemHandler, map[string]string{"id": id, "itemId": first})
	json.Unmarshal(rr.Body.Bytes(), &items)
	if !items[0].Done || items[0].DoneBy == nil || *items[0].DoneBy != "alice" || items[0].DoneAt == nil {
		t.Fatalf("tick: %+v", items[0])
	}

	rr = doTaskRequest(t, "PUT", "/", `{"ids":[`+third+`,`+first+`]}`, ReorderTaskChecklistHandler, map[string]string{"id": id})
	json.Unmarshal(rr.Body.Bytes(), &items)
	if got := []string{items[0].Text, items[1].Text, items[2].Text}; got[0] != "Report" || got[1] != "Backups" || got[2] != "example.com" {
		t.Fatalf("order = %v", got)
	}

	rr = doTaskRequest(t, "DELETE", "/", "", DeleteTaskChecklistItemHandler, map[string]string{"id": id, "itemId": second})
	json.Unmarshal(rr.Body.Bytes(), &items)
	if len(items) != 2 {
		t.Fatalf("delete: %+v", items)
	}

	// An item of another task isn't found.
	if rr := doTaskRequest(t, "PATCH", "/", `{"done":true}`, UpdateTaskChecklistItemHandler, map[string]string{"id": "999", "itemId": first}); rr.Code != http.StatusNotFound {
		t.Errorf("other task: %d", rr.Code)
	}

	// Completing makes the next round with the checklist unticked, the
	// same watchers and a link back.
	if rr := doTaskRequest(t, "POST", "/", "", CompleteTaskHandler, map[string]string{"id": id}); rr.Code != http.StatusOK {
		t.Fatalf("complete: %d", rr.Code)
	}
	all, err := db.GetTasks(db.TaskFilter{Status: models.TaskStatusPending})
	if err != nil || len(all) != 1 {
		t.Fatalf("next task: %v %+v", err, all)
	}
	next := all[0]
	if next.PreviousTaskID == nil || *next.PreviousTaskID != task.ID || next.ChecklistTotal != 2 || next.ChecklistDone != 0 {
		t.Fatalf("next = %+v", next)
	}
	if err := db.LoadTaskDetails(&next); err != nil {
		t.Fatal(err)
	}
	if next.Checklist[0].Text != "Report" || next.Checklist[0].Done || len(next.Watchers) != 1 {
		t.Fatalf("next details: %+v %v", next.Checklist, next.Watchers)
	}
}

func TestUpdateTaskSetsTypeIntervalAndPlugin(t *testing.T) {
	setupSettingsTest(t)
	task := decodeTask(t, doTaskRequest(t, "POST", "/", `{"title":"T"}`, CreateTaskHandler, nil))
	rr := doTaskRequest(t, "PATCH", "/", `{"type":"dynamic","interval":"2w","plugin_slug":"akismet"}`,
		UpdateTaskHandler, map[string]string{"id": strconv.Itoa(task.ID)})
	got := decodeTask(t, rr)
	if got.Type != models.TaskTypeDynamic || got.Interval == nil || *got.Interval != "2w" || got.PluginSlug == nil || *got.PluginSlug != "akismet" {
		t.Fatalf("updated = %+v", got)
	}
}

func TestTaskNotesWatchAndDelete(t *testing.T) {
	setupSettingsTest(t)
	task := decodeTask(t, doTaskRequest(t, "POST", "/", `{"title":"T"}`, CreateTaskHandler, nil))
	id := strconv.Itoa(task.ID)

	if rr := doTaskRequest(t, "DELETE", "/", "", WatchTaskHandler, map[string]string{"id": id}); rr.Code != http.StatusOK || strings.TrimSpace(rr.Body.String()) != "[]" {
		t.Fatalf("unwatch: %d %s", rr.Code, rr.Body.String())
	}
	// A note with watch off doesn't watch; the default does.
	doTaskRequest(t, "POST", "/", `{"parent_type":"Task","parent_id":"`+id+`","content":"a","watch":false}`, CreateNoteHandler, nil)
	if w, _ := db.GetTaskWatchers(task.ID); len(w) != 0 {
		t.Fatalf("watch:false watched: %v", w)
	}
	if rr := doTaskRequest(t, "POST", "/", `{"parent_type":"Task","parent_id":"`+id+`","content":"b"}`, CreateNoteHandler, nil); rr.Code != http.StatusCreated {
		t.Fatalf("note: %d", rr.Code)
	}
	if w, _ := db.GetTaskWatchers(task.ID); len(w) != 1 {
		t.Fatalf("note didn't watch: %v", w)
	}
	// Notes need a task that exists.
	if rr := doTaskRequest(t, "POST", "/", `{"parent_type":"Task","parent_id":"999","content":"c"}`, CreateNoteHandler, nil); rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown task note: %d", rr.Code)
	}

	if rr := doTaskRequest(t, "DELETE", "/", "", DeleteTaskHandler, map[string]string{"id": id}); rr.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rr.Code)
	}
	if notes, _ := db.GetNotes(models.NoteParentTypeTask, id); len(notes) != 0 {
		t.Fatalf("notes left: %+v", notes)
	}
}
