package tasks

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/db"
	"github.com/JCO-Digital/jman/internal/models"
)

type sentDM struct{ user, message string }

// setupNotify gives the notifier a fresh api.db, short timers and a
// recorder in place of Slack.
func setupNotify(t *testing.T, quiet, maxWait time.Duration) func() []sentDM {
	t.Helper()
	oldDir := config.RunData.DataDir
	config.RunData.DataDir = t.TempDir()
	if err := db.InitAPI(); err != nil {
		t.Fatal(err)
	}
	oldQuiet, oldMax, oldSend := notifyQuiet, notifyMaxWait, sendDM
	notifyQuiet, notifyMaxWait = quiet, maxWait
	var mu sync.Mutex
	var sent []sentDM
	sendDM = func(user, message string) {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, sentDM{user, message})
	}
	t.Cleanup(func() {
		FlushTaskNotifications()
		db.Close()
		config.RunData.DataDir = oldDir
		notifyQuiet, notifyMaxWait, sendDM = oldQuiet, oldMax, oldSend
	})
	return func() []sentDM {
		mu.Lock()
		defer mu.Unlock()
		return append([]sentDM(nil), sent...)
	}
}

func ptr[T any](v T) *T { return &v }

func saveTask(t *testing.T, task *models.Task) {
	t.Helper()
	if err := db.SaveTask(task, "alice"); err != nil {
		t.Fatal(err)
	}
}

func TestNotifyGroupsChangesPerRecipient(t *testing.T) {
	sent := setupNotify(t, time.Hour, time.Hour)

	task := &models.Task{Title: "Fix", Type: models.TaskTypeOneTime, Status: models.TaskStatusPending,
		Priority: models.TaskPriorityLow, AssignedTo: ptr("bob")}
	saveTask(t, task)
	db.WatchTask(task.ID, "alice")
	db.WatchTask(task.ID, "carol")

	// alice creates it, then edits it field by field; carol writes a note.
	NotifyTaskChange(nil, task, "alice")
	steps := []func(*models.Task){
		func(t *models.Task) { t.Priority = models.TaskPriorityHigh },
		func(t *models.Task) { t.Status = models.TaskStatusInProgress },
		func(t *models.Task) { t.Status = models.TaskStatusPending }, // back again
	}
	for _, step := range steps {
		before := *task
		step(task)
		saveTask(t, task)
		NotifyTaskChange(&before, task, "alice")
	}
	NotifyTaskNote(task, "carol", "Waiting on client")
	if got := sent(); len(got) != 0 {
		t.Fatalf("sent before the quiet time: %+v", got)
	}

	FlushTaskNotifications()
	got := map[string]string{}
	for _, dm := range sent() {
		got[dm.user] = dm.message
	}
	if len(sent()) != 3 {
		t.Fatalf("want one DM each to alice, bob and carol, got %+v", sent())
	}
	// alice watches too, but made the edits: she only hears of carol's note.
	if m := got["alice"]; !strings.Contains(m, "New note on task") || strings.Contains(m, "Priority") {
		t.Errorf("alice: %q", m)
	}
	// bob was assigned at creation: the whole task, as it is now, plus the note.
	if m := got["bob"]; !strings.Contains(m, "New Task Assigned to You") || !strings.Contains(m, "Priority: high") ||
		!strings.Contains(m, "*carol*: Waiting on client") {
		t.Errorf("bob: %q", m)
	}
	// carol watches: one change (status went back), no note of her own.
	m := got["carol"]
	if !strings.Contains(m, "Priority: low → high") || strings.Contains(m, "Status") || strings.Contains(m, "Waiting") {
		t.Errorf("carol: %q", m)
	}
}

func TestNotifySendsAfterQuietTime(t *testing.T) {
	sent := setupNotify(t, 20*time.Millisecond, time.Hour)
	task := &models.Task{Title: "Fix", Type: models.TaskTypeOneTime, Status: models.TaskStatusPending,
		Priority: models.TaskPriorityLow, AssignedTo: ptr("bob")}
	saveTask(t, task)

	NotifyTaskNote(task, "alice", "hello")
	deadline := time.Now().Add(2 * time.Second)
	for len(sent()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	got := sent()
	if len(got) != 1 || got[0].user != "bob" || !strings.Contains(got[0].message, "New note on task: Fix") {
		t.Fatalf("sent = %+v", got)
	}
}

func TestNotifySkipsWhoStoppedWatching(t *testing.T) {
	sent := setupNotify(t, time.Hour, time.Hour)
	task := &models.Task{Title: "Fix", Type: models.TaskTypeOneTime, Status: models.TaskStatusPending,
		Priority: models.TaskPriorityLow}
	saveTask(t, task)
	db.WatchTask(task.ID, "carol")

	NotifyTaskNote(task, "alice", "hello")
	db.UnwatchTask(task.ID, "carol")
	FlushTaskNotifications()
	if got := sent(); len(got) != 0 {
		t.Fatalf("sent = %+v", got)
	}
}

func TestMergeChanges(t *testing.T) {
	got := mergeChanges(
		[]taskFieldChange{{"Status", "pending", "in_progress"}, {"Title", "a", "b"}},
		[]taskFieldChange{{"Status", "in_progress", "pending"}, {"Priority", "low", "high"}},
	)
	if len(got) != 2 || got[0] != (taskFieldChange{"Title", "a", "b"}) || got[1] != (taskFieldChange{"Priority", "low", "high"}) {
		t.Fatalf("merge = %+v", got)
	}
}
