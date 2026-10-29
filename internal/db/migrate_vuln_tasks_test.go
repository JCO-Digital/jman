package db

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/models"
)

func TestSkipAutoVulnTasks(t *testing.T) {
	setupTaskRepoTest(t)

	meta := `{"vuln_uuids":["abc"]}`
	newTask := func(title, by string, status models.TaskStatus, metadata *string) int {
		t.Helper()
		task := &models.Task{
			Type:     models.TaskTypeOneTime,
			Status:   status,
			Priority: models.TaskPriorityHigh,
			Title:    title,
			Metadata: metadata,
		}
		if err := SaveTask(task, by); err != nil {
			t.Fatal(err)
		}
		return task.ID
	}
	auto := newTask("Security Vulnerabilities - example.com", "system", models.TaskStatusPending, &meta)
	autoInProgress := newTask("Security Vulnerabilities - other.com", "system", models.TaskStatusInProgress, &meta)
	autoDone := newTask("Security Vulnerabilities - done.com", "system", models.TaskStatusCompleted, &meta)
	manual := newTask("Security Vulnerabilities - manual.com", "niklas", models.TaskStatusPending, &meta)
	otherSystem := newTask("Renew certificate", "system", models.TaskStatusPending, nil)

	api := GetAPIDB().DB
	if _, err := api.Exec(`DELETE FROM schema_migrations WHERE name = ?`, vulnTasksMigrationName); err != nil {
		t.Fatal(err)
	}
	if err := SkipAutoVulnTasks(api); err != nil {
		t.Fatal(err)
	}

	want := map[int]models.TaskStatus{
		auto:           models.TaskStatusSkipped,
		autoInProgress: models.TaskStatusSkipped,
		autoDone:       models.TaskStatusCompleted,
		manual:         models.TaskStatusPending,
		otherSystem:    models.TaskStatusPending,
	}
	for id, status := range want {
		task, err := GetTask(id)
		if err != nil || task == nil {
			t.Fatalf("task %d: %v", id, err)
		}
		if task.Status != status {
			t.Errorf("task %d (%s) status = %s, want %s", id, task.Title, task.Status, status)
		}
	}

	// Runs only once: a task matching the pattern afterwards is left alone.
	later := newTask("Security Vulnerabilities - later.com", "system", models.TaskStatusPending, &meta)
	if err := SkipAutoVulnTasks(api); err != nil {
		t.Fatal(err)
	}
	if task, _ := GetTask(later); task == nil || task.Status != models.TaskStatusPending {
		t.Errorf("task created after the migration was changed: %+v", task)
	}
}
