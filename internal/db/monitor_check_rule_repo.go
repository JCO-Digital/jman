package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

// ListMonitorCheckRules returns every monitor check rule, oldest first.
func ListMonitorCheckRules() ([]models.MonitorCheckRule, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT id, match_type, value, action, note, created_at, created_by
		FROM monitor_check_rules ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list monitor check rules: %w", err)
	}
	defer rows.Close()

	rules := []models.MonitorCheckRule{}
	for rows.Next() {
		var r models.MonitorCheckRule
		var createdAt sql.NullTime
		var createdBy sql.NullString
		if err := rows.Scan(&r.ID, &r.MatchType, &r.Value, &r.Action, &r.Note, &createdAt, &createdBy); err != nil {
			return nil, fmt.Errorf("failed to scan monitor check rule: %w", err)
		}
		r.CreatedAt = createdAt.Time
		r.CreatedBy = createdBy.String
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// CreateMonitorCheckRule inserts r, assigning it a new ID.
func CreateMonitorCheckRule(r *models.MonitorCheckRule, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	r.ID = utils.NewV7UUID()
	if r.Action == "" {
		r.Action = models.MonitorRuleActionStartPaused
	}
	r.CreatedAt = time.Now().UTC()
	r.CreatedBy = username
	_, err := db.Exec(`INSERT INTO monitor_check_rules (id, match_type, value, action, note, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, r.ID, r.MatchType, r.Value, r.Action, r.Note, r.CreatedAt, r.CreatedBy)
	if err != nil {
		return fmt.Errorf("failed to create monitor check rule: %w", err)
	}
	return nil
}

// DeleteMonitorCheckRule deletes a rule. Checks it paused stay paused but
// no longer refer to it. It reports whether the rule existed.
func DeleteMonitorCheckRule(id string) (bool, error) {
	db := GetAPIDB()
	if db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE monitor_checks SET paused_by_rule_id = NULL WHERE paused_by_rule_id = ?`, id); err != nil {
		return false, fmt.Errorf("failed to detach checks from rule %s: %w", id, err)
	}
	res, err := tx.Exec(`DELETE FROM monitor_check_rules WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("failed to delete monitor check rule %s: %w", id, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, tx.Commit()
}
