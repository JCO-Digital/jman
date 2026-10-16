package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

const monitorCheckColumns = `id, site_id, is_default, type, name, host, path, expect, enabled,
	orphaned_at, created_at, created_by, updated_at, updated_by`

// ListMonitorChecks returns every monitor check, default checks first.
func ListMonitorChecks() ([]models.MonitorCheck, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT ` + monitorCheckColumns + ` FROM monitor_checks ORDER BY is_default DESC, created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list monitor checks: %w", err)
	}
	defer rows.Close()

	checks := []models.MonitorCheck{}
	for rows.Next() {
		c, err := scanMonitorCheck(rows)
		if err != nil {
			return nil, err
		}
		checks = append(checks, *c)
	}
	return checks, rows.Err()
}

// GetMonitorCheck returns the monitor check with the given ID, or nil if
// there is none.
func GetMonitorCheck(id string) (*models.MonitorCheck, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	c, err := scanMonitorCheck(db.QueryRow(`SELECT `+monitorCheckColumns+` FROM monitor_checks WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

// CreateMonitorCheck inserts c, assigning it a new ID when it has none.
func CreateMonitorCheck(c *models.MonitorCheck, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	if c.ID == "" {
		c.ID = utils.NewV7UUID()
	}
	if c.Type == "" {
		c.Type = models.MonitorCheckTypeHTTP
	}
	if c.Path == "" {
		c.Path = "/"
	}
	if c.Expect == "" {
		c.Expect = models.MonitorExpectHTML
	}
	now := time.Now().UTC()
	c.CreatedAt, c.UpdatedAt = now, now
	c.CreatedBy, c.UpdatedBy = username, username

	var siteID any
	if c.SiteID != "" {
		siteID = c.SiteID
	}
	_, err := db.Exec(`
		INSERT INTO monitor_checks (`+monitorCheckColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID, siteID, c.IsDefault, c.Type, c.Name, c.Host, c.Path, c.Expect, c.Enabled,
		c.OrphanedAt, c.CreatedAt, c.CreatedBy, c.UpdatedAt, c.UpdatedBy)
	if err != nil {
		return fmt.Errorf("failed to create monitor check: %w", err)
	}
	return nil
}

// SetMonitorCheckEnabled pauses (enabled = false) or resumes a check.
func SetMonitorCheckEnabled(id string, enabled bool, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	_, err := db.Exec(`UPDATE monitor_checks SET enabled = ?, updated_at = ?, updated_by = ? WHERE id = ?`,
		enabled, time.Now(), username, id)
	if err != nil {
		return fmt.Errorf("failed to update monitor check %s: %w", id, err)
	}
	return nil
}

// SetMonitorCheckOrphaned marks a check as orphaned since at, or clears
// the mark when at is nil. It doesn't touch updated_at/by: orphaning is
// the reconcile's bookkeeping, not an edit.
func SetMonitorCheckOrphaned(id string, at *time.Time) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	_, err := db.Exec(`UPDATE monitor_checks SET orphaned_at = ? WHERE id = ?`, at, id)
	if err != nil {
		return fmt.Errorf("failed to update monitor check %s: %w", id, err)
	}
	return nil
}

func scanMonitorCheck(row rowScanner) (*models.MonitorCheck, error) {
	var c models.MonitorCheck
	var siteID, createdBy, updatedBy sql.NullString
	var orphanedAt, createdAt, updatedAt sql.NullTime
	err := row.Scan(&c.ID, &siteID, &c.IsDefault, &c.Type, &c.Name, &c.Host, &c.Path, &c.Expect, &c.Enabled,
		&orphanedAt, &createdAt, &createdBy, &updatedAt, &updatedBy)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan monitor check: %w", err)
	}
	c.SiteID = siteID.String
	c.CreatedBy = createdBy.String
	c.UpdatedBy = updatedBy.String
	c.CreatedAt = createdAt.Time
	c.UpdatedAt = updatedAt.Time
	if orphanedAt.Valid {
		t := orphanedAt.Time
		c.OrphanedAt = &t
	}
	return &c, nil
}
