package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

var ErrTaskNotFound = errors.New("task not found")

// SaveTask inserts or updates a task in the database.
func SaveTask(task *models.Task, username string) error {
	database := GetDB()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	if task.ID == 0 {
		query := `
		INSERT INTO tasks (
			type, status, priority, title, description,
			site_id, server_id, organization_id, plugin_slug,
			assigned_to, metadata, interval, due_date, reminder_date,
			created_at, created_by, updated_at, last_notified_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		result, err := database.Exec(query,
			task.Type, task.Status, task.Priority, task.Title, task.Description,
			task.SiteID, task.ServerID, task.OrganizationID, task.PluginSlug,
			task.AssignedTo, task.Metadata, task.Interval, task.DueDate, task.ReminderDate,
			now, username, now, task.LastNotifiedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert task: %w", err)
		}
		id, _ := result.LastInsertId()
		task.ID = int(id)
		task.CreatedAt = now
		task.CreatedBy = username
		task.UpdatedAt = now
	} else {
		query := `
		UPDATE tasks SET
			type = ?, status = ?, priority = ?, title = ?, description = ?,
			site_id = ?, server_id = ?, organization_id = ?, plugin_slug = ?,
			assigned_to = ?, metadata = ?, interval = ?, due_date = ?, reminder_date = ?,
			completed_at = ?, updated_at = ?, last_notified_at = ?
		WHERE id = ?
		`
		_, err := database.Exec(query,
			task.Type, task.Status, task.Priority, task.Title, task.Description,
			task.SiteID, task.ServerID, task.OrganizationID, task.PluginSlug,
			task.AssignedTo, task.Metadata, task.Interval, task.DueDate, task.ReminderDate,
			task.CompletedAt, now, task.LastNotifiedAt, task.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to update task: %w", err)
		}
		task.UpdatedAt = now
	}
	return nil
}

// GetTask retrieves a single task by ID.
func GetTask(id int) (*models.Task, error) {
	database := GetDB()
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT
		id, type, status, priority, title, description,
		site_id, server_id, organization_id, plugin_slug,
		assigned_to, metadata, interval, due_date, reminder_date,
		created_at, completed_at, created_by, updated_at, last_notified_at
	FROM tasks WHERE id = ?`

	var t models.Task
	err := database.QueryRow(query, id).Scan(
		&t.ID, &t.Type, &t.Status, &t.Priority, &t.Title, &t.Description,
		&t.SiteID, &t.ServerID, &t.OrganizationID, &t.PluginSlug,
		&t.AssignedTo, &t.Metadata, &t.Interval, &t.DueDate, &t.ReminderDate,
		&t.CreatedAt, &t.CompletedAt, &t.CreatedBy, &t.UpdatedAt, &t.LastNotifiedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	return &t, nil
}

// TaskFilter defines criteria for querying tasks.
type TaskFilter struct {
	Status         models.TaskStatus
	Priority       models.TaskPriority
	AssignedTo     string
	SiteID         int
	OrganizationID int
	ServerID       int
	Search         string
}

// GetTasks retrieves tasks based on the provided filter.
func GetTasks(filter TaskFilter) ([]models.Task, error) {
	database := GetDB()
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT
		id, type, status, priority, title, description,
		site_id, server_id, organization_id, plugin_slug,
		assigned_to, metadata, interval, due_date, reminder_date,
		created_at, completed_at, created_by, updated_at, last_notified_at
	FROM tasks WHERE 1=1`

	var args []interface{}

	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}
	if filter.Priority != "" {
		query += " AND priority = ?"
		args = append(args, filter.Priority)
	}
	if filter.AssignedTo != "" {
		query += " AND assigned_to = ?"
		args = append(args, filter.AssignedTo)
	}
	if filter.SiteID != 0 {
		query += " AND site_id = ?"
		args = append(args, filter.SiteID)
	}
	if filter.OrganizationID != 0 {
		query += " AND organization_id = ?"
		args = append(args, filter.OrganizationID)
	}
	if filter.ServerID != 0 {
		query += " AND server_id = ?"
		args = append(args, filter.ServerID)
	}
	if filter.Search != "" {
		query += " AND (title LIKE ? OR description LIKE ?)"
		term := "%" + filter.Search + "%"
		args = append(args, term, term)
	}

	query += " ORDER BY CASE priority WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END, due_date ASC"

	rows, err := database.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	var tasks []models.Task
	for rows.Next() {
		var t models.Task
		err := rows.Scan(
			&t.ID, &t.Type, &t.Status, &t.Priority, &t.Title, &t.Description,
			&t.SiteID, &t.ServerID, &t.OrganizationID, &t.PluginSlug,
			&t.AssignedTo, &t.Metadata, &t.Interval, &t.DueDate, &t.ReminderDate,
			&t.CreatedAt, &t.CompletedAt, &t.CreatedBy, &t.UpdatedAt, &t.LastNotifiedAt,
		)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// CompleteTask marks a task as completed and handles the generation of recurring tasks.
func CompleteTask(id int, username string) error {
	task, err := GetTask(id)
	if err != nil {
		return err
	}
	if task == nil {
		return ErrTaskNotFound
	}

	now := time.Now()
	task.Status = models.TaskStatusCompleted
	task.CompletedAt = &now

	if err := SaveTask(task, username); err != nil {
		return err
	}

	// Handle repetition logic
	if task.Type == models.TaskTypeRepeating || task.Type == models.TaskTypeDynamic {
		return createNextRecurringTask(task, username)
	}

	return nil
}

// createNextRecurringTask calculates the next due date and creates a new task instance.
func createNextRecurringTask(prevTask *models.Task, username string) error {
	if prevTask.Interval == nil || *prevTask.Interval == "" {
		return nil
	}

	// Try custom parser first (d, w, m, y) to avoid Go's ParseDuration
	// interpreting 'm' as minutes instead of months.
	duration, err := parseCustomDuration(*prevTask.Interval)
	if err != nil {
		// Fall back to standard Go duration (e.g., "1h", "10s")
		duration, err = time.ParseDuration(*prevTask.Interval)
		if err != nil {
			return fmt.Errorf("invalid interval format: %w", err)
		}
	}

	var nextDueDate time.Time
	if prevTask.Type == models.TaskTypeRepeating {
		// Repeating: base + interval
		if prevTask.DueDate != nil {
			nextDueDate = prevTask.DueDate.Add(duration)
		} else {
			nextDueDate = time.Now().Add(duration)
		}
	} else {
		// Dynamic: completion + interval
		nextDueDate = time.Now().Add(duration)
	}

	// Calculate reminder date if it existed on the previous task (relative to due date)
	var nextReminderDate *time.Time
	if prevTask.DueDate != nil && prevTask.ReminderDate != nil {
		reminderLead := prevTask.DueDate.Sub(*prevTask.ReminderDate)
		rd := nextDueDate.Add(-reminderLead)
		nextReminderDate = &rd
	}

	newTask := &models.Task{
		Type:           prevTask.Type,
		Status:         models.TaskStatusPending,
		Priority:       prevTask.Priority,
		Title:          prevTask.Title,
		Description:    prevTask.Description,
		SiteID:         prevTask.SiteID,
		ServerID:       prevTask.ServerID,
		OrganizationID: prevTask.OrganizationID,
		PluginSlug:     prevTask.PluginSlug,
		AssignedTo:     prevTask.AssignedTo,
		Metadata:       prevTask.Metadata,
		Interval:       prevTask.Interval,
		DueDate:        &nextDueDate,
		ReminderDate:   nextReminderDate,
	}

	return SaveTask(newTask, username)
}

// parseCustomDuration handles "d", "w", "m", "y" formats that Go's time.ParseDuration doesn't support.
func parseCustomDuration(s string) (time.Duration, error) {
	var val int
	var unit string
	_, err := fmt.Sscanf(s, "%d%s", &val, &unit)
	if err != nil {
		return 0, err
	}

	switch unit {
	case "d":
		return time.Duration(val) * 24 * time.Hour, nil
	case "w":
		return time.Duration(val) * 7 * 24 * time.Hour, nil
	case "m":
		// Approximated as 30 days
		return time.Duration(val) * 30 * 24 * time.Hour, nil
	case "y":
		// Approximated as 365 days
		return time.Duration(val) * 365 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown unit: %s", unit)
	}
}

// DeleteTask removes a task from the database.
func DeleteTask(id int) error {
	database := GetDB()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := database.Exec("DELETE FROM tasks WHERE id = ?", id)
	return err
}

// GetOpenVulnerabilityTask searches for an incomplete vulnerability task for a site.
func GetOpenVulnerabilityTask(siteID int) (*models.Task, error) {
	database := GetDB()
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `
	SELECT
		id, type, status, priority, title, description,
		site_id, server_id, organization_id, plugin_slug,
		assigned_to, metadata, interval, due_date, reminder_date,
		created_at, completed_at, created_by, updated_at, last_notified_at
	FROM tasks
	WHERE site_id = ? AND status != 'completed' AND status != 'skipped' AND title LIKE 'Security Vulnerabilities%'
	LIMIT 1`

	var t models.Task
	err := database.QueryRow(query, siteID).Scan(
		&t.ID, &t.Type, &t.Status, &t.Priority, &t.Title, &t.Description,
		&t.SiteID, &t.ServerID, &t.OrganizationID, &t.PluginSlug,
		&t.AssignedTo, &t.Metadata, &t.Interval, &t.DueDate, &t.ReminderDate,
		&t.CreatedAt, &t.CompletedAt, &t.CreatedBy, &t.UpdatedAt, &t.LastNotifiedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find open vuln task: %w", err)
	}
	return &t, nil
}
