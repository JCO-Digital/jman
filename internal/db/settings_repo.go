package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// SystemSettingsUserID is a reserved sentinel user_id for settings that are
// global rather than tied to a specific user — the settings table is keyed
// by (user_id, key) with no dedicated global-settings table, so global
// settings are stored under this sentinel instead.
const SystemSettingsUserID = "system"

// DefaultVulnAssigneeSettingKey is the global setting key holding the
// username that newly-created vulnerability Tasks are auto-assigned to.
// An empty value means vulnerability Tasks are left unassigned.
const DefaultVulnAssigneeSettingKey = "default_vuln_assignee"

// GetSetting retrieves a specific setting for a user by key.
func GetSetting(userID, key string) (*models.Setting, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT user_id, key, value, created_at, updated_at FROM settings WHERE user_id = ? AND key = ?`
	var s models.Setting
	var valueJSON sql.NullString

	err := db.QueryRow(query, userID, key).Scan(&s.UserID, &s.Key, &valueJSON, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get setting: %w", err)
	}

	if valueJSON.Valid {
		if err := json.Unmarshal([]byte(valueJSON.String), &s.Value); err != nil {
			return nil, fmt.Errorf("failed to unmarshal setting value: %w", err)
		}
	}

	return &s, nil
}

// GetAllSettings retrieves all settings for a specific user.
func GetAllSettings(userID string) ([]models.Setting, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT user_id, key, value, created_at, updated_at FROM settings WHERE user_id = ? ORDER BY key ASC`
	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query settings: %w", err)
	}
	defer rows.Close()

	var settings []models.Setting
	for rows.Next() {
		var s models.Setting
		var valueJSON sql.NullString
		if err := rows.Scan(&s.UserID, &s.Key, &valueJSON, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		if valueJSON.Valid {
			if err := json.Unmarshal([]byte(valueJSON.String), &s.Value); err != nil {
				return nil, fmt.Errorf("failed to unmarshal setting value for key %s: %w", s.Key, err)
			}
		}
		settings = append(settings, s)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return settings, nil
}

// SaveSetting creates or updates a setting for a user.
func SaveSetting(userID, key string, value any) (*models.Setting, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	valueJSON, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal setting value: %w", err)
	}

	now := time.Now()
	query := `
		INSERT INTO settings (user_id, key, value, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at
	`

	_, err = db.Exec(query, userID, key, string(valueJSON), now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to save setting: %w", err)
	}

	return GetSetting(userID, key)
}

// DeleteSetting removes a setting for a user.
func DeleteSetting(userID, key string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	query := `DELETE FROM settings WHERE user_id = ? AND key = ?`
	_, err := db.Exec(query, userID, key)
	if err != nil {
		return fmt.Errorf("failed to delete setting: %w", err)
	}

	return nil
}
