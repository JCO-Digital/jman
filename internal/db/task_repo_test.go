package db

import (
	"os"
	"sync"
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
)

func setupTaskRepoTest(t *testing.T) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "jman-task-repo-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	oldDataDir := config.RunData.DataDir
	config.RunData.DataDir = tempDir

	if err := Init(); err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}

	t.Cleanup(func() {
		Close()
		os.RemoveAll(tempDir)
		config.RunData.DataDir = oldDataDir
	})
}

func TestCompleteTaskMarksCompletedAndSpawnsRecurringTask(t *testing.T) {
	setupTaskRepoTest(t)

	interval := "1d"
	task := &models.Task{
		Type:     models.TaskTypeRepeating,
		Status:   models.TaskStatusPending,
		Priority: models.TaskPriorityMedium,
		Title:    "recurring task",
		Interval: &interval,
	}
	if err := SaveTask(task, "creator"); err != nil {
		t.Fatalf("failed to save task: %v", err)
	}

	completed, err := CompleteTask(task.ID, "completer")
	if err != nil {
		t.Fatalf("CompleteTask returned error: %v", err)
	}
	if completed.Status != models.TaskStatusCompleted {
		t.Fatalf("expected status completed, got %q", completed.Status)
	}

	tasks, err := GetTasks(TaskFilter{})
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected original + 1 recurring task, got %d tasks", len(tasks))
	}
}

func TestCompleteTaskConcurrentDoesNotDuplicateRecurringTask(t *testing.T) {
	setupTaskRepoTest(t)

	interval := "1d"
	task := &models.Task{
		Type:     models.TaskTypeRepeating,
		Status:   models.TaskStatusPending,
		Priority: models.TaskPriorityMedium,
		Title:    "recurring task",
		Interval: &interval,
	}
	if err := SaveTask(task, "creator"); err != nil {
		t.Fatalf("failed to save task: %v", err)
	}

	const concurrency = 10
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = CompleteTask(task.ID, "completer")
		}()
	}
	wg.Wait()

	tasks, err := GetTasks(TaskFilter{})
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected exactly 1 recurring task to be spawned (2 total), got %d tasks", len(tasks))
	}
}
