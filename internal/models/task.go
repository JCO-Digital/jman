package models

import "time"

// TaskType defines the repetition logic of the task.
type TaskType string

const (
	TaskTypeOneTime   TaskType = "one-time"
	TaskTypeRepeating TaskType = "repeating"
	TaskTypeDynamic   TaskType = "dynamic"
)

// TaskStatus defines the current state of a task.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusSkipped    TaskStatus = "skipped"
	TaskStatusOnHold     TaskStatus = "on_hold"
	TaskStatusBlocked    TaskStatus = "blocked"
	TaskStatusOverdue    TaskStatus = "overdue"
)

// TaskPriority defines the importance of the task.
type TaskPriority string

const (
	TaskPriorityLow    TaskPriority = "low"
	TaskPriorityMedium TaskPriority = "medium"
	TaskPriorityHigh   TaskPriority = "high"
)

// Task represents a unit of work or a reminder in the system.
type Task struct {
	ID          int          `json:"id"`
	Type        TaskType     `json:"type"`
	Status      TaskStatus   `json:"status"`
	Priority    TaskPriority `json:"priority"`
	Title       string       `json:"title"`
	Description *string      `json:"description,omitempty"`

	// Linkage
	SiteID         *string `json:"site_id,omitempty"`   // site UUID
	ServerID       *string `json:"server_id,omitempty"` // server UUID
	OrganizationID *int    `json:"organization_id,omitempty"`
	PluginSlug     *string `json:"plugin_slug,omitempty"`

	AssignedTo *string `json:"assigned_to,omitempty"` // Username
	Metadata   *string `json:"metadata,omitempty"`    // JSON string

	Interval *string `json:"interval,omitempty"` // e.g., "30d", "1y"

	DueDate        *time.Time `json:"due_date,omitempty"`
	ReminderDate   *time.Time `json:"reminder_date,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	LastNotifiedAt *time.Time `json:"last_notified_at,omitempty"`

	CreatedBy   string    `json:"created_by"`
	CompletedBy *string   `json:"completed_by,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`

	// PreviousTaskID is the task a repeating or dynamic task was created
	// from when that one was completed.
	PreviousTaskID *int `json:"previous_task_id,omitempty"`

	// Checklist counts, filled in on every read.
	ChecklistTotal int `json:"checklist_total"`
	ChecklistDone  int `json:"checklist_done"`

	// Checklist and Watchers are only filled in when one task is read.
	Checklist []TaskChecklistItem `json:"checklist,omitempty"`
	Watchers  []string            `json:"watchers,omitempty"`
}

// TaskChecklistRefType is the kind of record a checklist item stands for.
type TaskChecklistRefType string

const (
	TaskChecklistRefSite          TaskChecklistRefType = "site"
	TaskChecklistRefServer        TaskChecklistRefType = "server"
	TaskChecklistRefPlugin        TaskChecklistRefType = "plugin"
	TaskChecklistRefInventoryItem TaskChecklistRefType = "inventory_item"
)

// IsValidTaskChecklistRefType reports whether t is a known ref type.
func IsValidTaskChecklistRefType(t TaskChecklistRefType) bool {
	switch t {
	case TaskChecklistRefSite, TaskChecklistRefServer, TaskChecklistRefPlugin, TaskChecklistRefInventoryItem:
		return true
	}
	return false
}

// TaskChecklistItem is one line of a task's checklist. An item may stand for
// a site, server, plugin or inventory item (RefType/RefID); Text is then the
// record's name when it was added, so the item still reads right after the
// record is gone.
type TaskChecklistItem struct {
	ID        int                  `json:"id"`
	TaskID    int                  `json:"task_id"`
	Position  int                  `json:"position"`
	Text      string               `json:"text"`
	Done      bool                 `json:"done"`
	DoneBy    *string              `json:"done_by,omitempty"`
	DoneAt    *time.Time           `json:"done_at,omitempty"`
	RefType   TaskChecklistRefType `json:"ref_type,omitempty"`
	RefID     string               `json:"ref_id,omitempty"`
	CreatedAt time.Time            `json:"created_at"`
	CreatedBy string               `json:"created_by"`
}
