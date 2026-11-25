package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

const checklistColumns = `id, task_id, position, text, done, done_by, done_at,
	COALESCE(ref_type, ''), COALESCE(ref_id, ''), created_at, COALESCE(created_by, '')`

func scanChecklistItem(scanner interface{ Scan(dest ...any) error }) (*models.TaskChecklistItem, error) {
	var it models.TaskChecklistItem
	if err := scanner.Scan(&it.ID, &it.TaskID, &it.Position, &it.Text, &it.Done, &it.DoneBy, &it.DoneAt,
		&it.RefType, &it.RefID, &it.CreatedAt, &it.CreatedBy); err != nil {
		return nil, err
	}
	return &it, nil
}

// GetTaskChecklist returns a task's checklist in order.
func GetTaskChecklist(taskID int) ([]models.TaskChecklistItem, error) {
	database := GetAPIDB()
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := database.Query(`SELECT `+checklistColumns+`
		FROM task_checklist_items WHERE task_id = ? ORDER BY position, id`, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to query checklist: %w", err)
	}
	defer rows.Close()
	items := []models.TaskChecklistItem{}
	for rows.Next() {
		it, err := scanChecklistItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *it)
	}
	return items, rows.Err()
}

// GetTaskChecklistItem returns one item of a task's checklist, or nil.
func GetTaskChecklistItem(taskID, itemID int) (*models.TaskChecklistItem, error) {
	database := GetAPIDB()
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	it, err := scanChecklistItem(database.QueryRow(`SELECT `+checklistColumns+`
		FROM task_checklist_items WHERE task_id = ? AND id = ?`, taskID, itemID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get checklist item: %w", err)
	}
	return it, nil
}

// AddTaskChecklistItems appends items to the end of a task's checklist, in
// the order given, and returns them as saved.
func AddTaskChecklistItems(taskID int, items []models.TaskChecklistItem, username string) ([]models.TaskChecklistItem, error) {
	database := GetAPIDB()
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	tx, err := database.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var next int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(position), -1) + 1 FROM task_checklist_items WHERE task_id = ?`,
		taskID).Scan(&next); err != nil {
		return nil, fmt.Errorf("failed to read checklist position: %w", err)
	}

	now := time.Now()
	saved := make([]models.TaskChecklistItem, 0, len(items))
	for _, it := range items {
		var refType, refID any
		if it.RefType != "" {
			refType, refID = string(it.RefType), it.RefID
		}
		var id int64
		if err := tx.QueryRow(`INSERT INTO task_checklist_items
			(task_id, position, text, done, ref_type, ref_id, created_at, created_by)
			VALUES (?, ?, ?, 0, ?, ?, ?, ?) RETURNING id`,
			taskID, next, it.Text, refType, refID, now, username).Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to insert checklist item: %w", err)
		}
		saved = append(saved, models.TaskChecklistItem{
			ID: int(id), TaskID: taskID, Position: next, Text: it.Text,
			RefType: it.RefType, RefID: it.RefID, CreatedAt: now, CreatedBy: username,
		})
		next++
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return saved, nil
}

// UpdateTaskChecklistItem saves an item's text and done state. Ticking an
// item records who ticked it and when; unticking clears both.
func UpdateTaskChecklistItem(it *models.TaskChecklistItem, username string) error {
	database := GetAPIDB()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}
	if it.Done && it.DoneAt == nil {
		now := time.Now()
		it.DoneAt, it.DoneBy = &now, &username
	} else if !it.Done {
		it.DoneAt, it.DoneBy = nil, nil
	}
	_, err := database.Exec(`UPDATE task_checklist_items SET text = ?, done = ?, done_by = ?, done_at = ?
		WHERE id = ? AND task_id = ?`, it.Text, it.Done, it.DoneBy, it.DoneAt, it.ID, it.TaskID)
	if err != nil {
		return fmt.Errorf("failed to update checklist item: %w", err)
	}
	return nil
}

// ReorderTaskChecklist puts a task's checklist in the order of ids. Items
// not in ids keep their place after the ones that are.
func ReorderTaskChecklist(taskID int, ids []int) error {
	database := GetAPIDB()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}
	tx, err := database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Leave room in front for the listed items.
	if _, err := tx.Exec(`UPDATE task_checklist_items SET position = position + ? WHERE task_id = ?`,
		len(ids), taskID); err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE task_checklist_items SET position = ? WHERE task_id = ? AND id = ?`,
			i, taskID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteTaskChecklistItem removes one item from a task's checklist.
func DeleteTaskChecklistItem(taskID, itemID int) error {
	database := GetAPIDB()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := database.Exec(`DELETE FROM task_checklist_items WHERE task_id = ? AND id = ?`, taskID, itemID)
	return err
}

// GetTaskWatchers returns the usernames watching a task.
func GetTaskWatchers(taskID int) ([]string, error) {
	database := GetAPIDB()
	if database == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := database.Query(`SELECT username FROM task_watchers WHERE task_id = ? ORDER BY created_at, username`, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to query watchers: %w", err)
	}
	defer rows.Close()
	watchers := []string{}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		watchers = append(watchers, u)
	}
	return watchers, rows.Err()
}

// WatchTask adds username to a task's watchers; watching twice is a no-op.
func WatchTask(taskID int, username string) error {
	database := GetAPIDB()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := database.Exec(`INSERT OR IGNORE INTO task_watchers (task_id, username, created_at) VALUES (?, ?, ?)`,
		taskID, username, time.Now())
	return err
}

// UnwatchTask removes username from a task's watchers.
func UnwatchTask(taskID int, username string) error {
	database := GetAPIDB()
	if database == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := database.Exec(`DELETE FROM task_watchers WHERE task_id = ? AND username = ?`, taskID, username)
	return err
}

// LoadTaskDetails fills in a task's checklist and watchers.
func LoadTaskDetails(task *models.Task) error {
	items, err := GetTaskChecklist(task.ID)
	if err != nil {
		return err
	}
	watchers, err := GetTaskWatchers(task.ID)
	if err != nil {
		return err
	}
	task.Checklist, task.Watchers = items, watchers
	task.ChecklistTotal, task.ChecklistDone = len(items), 0
	for _, it := range items {
		if it.Done {
			task.ChecklistDone++
		}
	}
	return nil
}
