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
	SiteID         *int    `json:"site_id,omitempty"`
	ServerID       *int    `json:"server_id,omitempty"`
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
}
