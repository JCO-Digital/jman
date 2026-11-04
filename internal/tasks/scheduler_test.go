package tasks

import (
	"testing"

	"github.com/JCO-Digital/jman/internal/config"
	"github.com/JCO-Digital/jman/internal/models"
)

func TestTaskLink(t *testing.T) {
	old := config.Cfg.WebURL
	t.Cleanup(func() { config.Cfg.WebURL = old })

	config.Cfg.WebURL = "https://jman.example.com/"
	if got, want := taskLink(&models.Task{ID: 12}), "<https://jman.example.com/tasks?task=12|Open task #12>"; got != want {
		t.Errorf("taskLink = %q, want %q", got, want)
	}

	// Without the UI's address, or a saved task, there's nothing to link.
	if got := taskLink(&models.Task{}); got != "" {
		t.Errorf("unsaved task: taskLink = %q", got)
	}
	config.Cfg.WebURL = ""
	if got := taskLink(&models.Task{ID: 12}); got != "" {
		t.Errorf("no webUrl: taskLink = %q", got)
	}
}
