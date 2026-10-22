package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
	"github.com/JCO-Digital/jman/internal/utils"
)

const monitorCheckColumns = `id, site_id, is_default, type, name, host, path, expect,
	assert_path, assert_op, assert_value, cache_bypass,
	interval_seconds, grace_seconds, fail_threshold, enabled, paused_by_rule_id,
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
	if c.Expect == "" && c.Type == models.MonitorCheckTypeHTTP {
		c.Expect = models.MonitorExpectHTML
	}
	if c.FailThreshold < 1 {
		c.FailThreshold = 1
	}
	now := time.Now().UTC()
	c.CreatedAt, c.UpdatedAt = now, now
	c.CreatedBy, c.UpdatedBy = username, username

	var siteID, pausedBy any
	if c.SiteID != "" {
		siteID = c.SiteID
	}
	if c.PausedByRuleID != "" {
		pausedBy = c.PausedByRuleID
	}
	_, err := db.Exec(`
		INSERT INTO monitor_checks (`+monitorCheckColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID, siteID, c.IsDefault, c.Type, c.Name, c.Host, c.Path, c.Expect,
		c.AssertPath, c.AssertOp, string(c.AssertValue), c.CacheBypass,
		c.IntervalSeconds, c.GraceSeconds, c.FailThreshold, c.Enabled, pausedBy,
		c.OrphanedAt, c.CreatedAt, c.CreatedBy, c.UpdatedAt, c.UpdatedBy)
	if err != nil {
		return fmt.Errorf("failed to create monitor check: %w", err)
	}
	return nil
}

// UpdateMonitorCheck saves a check's editable fields: name, host, path,
// expectation, assertion, cache bypass and heartbeat settings.
func UpdateMonitorCheck(c *models.MonitorCheck, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	c.UpdatedAt = time.Now().UTC()
	c.UpdatedBy = username
	_, err := db.Exec(`
		UPDATE monitor_checks
		SET name = ?, host = ?, path = ?, expect = ?, assert_path = ?, assert_op = ?, assert_value = ?,
		    cache_bypass = ?, interval_seconds = ?, grace_seconds = ?, fail_threshold = ?,
		    updated_at = ?, updated_by = ?
		WHERE id = ?
	`, c.Name, c.Host, c.Path, c.Expect, c.AssertPath, c.AssertOp, string(c.AssertValue),
		c.CacheBypass, c.IntervalSeconds, c.GraceSeconds, c.FailThreshold, c.UpdatedAt, c.UpdatedBy, c.ID)
	if err != nil {
		return fmt.Errorf("failed to update monitor check %s: %w", c.ID, err)
	}
	return nil
}

// DeleteMonitorCheck deletes a check with its secrets, check-in token,
// monitor state, history and incidents.
func DeleteMonitorCheck(id string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"monitor_check_secrets", "monitor_check_tokens", "monitor_check_status", "monitor_history", "incidents"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE check_id = ?`, id); err != nil {
			return fmt.Errorf("failed to delete %s of check %s: %w", table, id, err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM monitor_checks WHERE id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete monitor check %s: %w", id, err)
	}
	return tx.Commit()
}

// SetMonitorCheckEnabled pauses (enabled = false) or resumes a check.
// Either way the check is no longer paused by a rule.
func SetMonitorCheckEnabled(id string, enabled bool, username string) error {
	return setMonitorCheckEnabled(id, enabled, nil, username)
}

// PauseMonitorCheckByRule pauses a check on behalf of a rule.
func PauseMonitorCheckByRule(id, ruleID, username string) error {
	return setMonitorCheckEnabled(id, false, ruleID, username)
}

func setMonitorCheckEnabled(id string, enabled bool, pausedBy any, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	_, err := db.Exec(`UPDATE monitor_checks SET enabled = ?, paused_by_rule_id = ?, updated_at = ?, updated_by = ? WHERE id = ?`,
		enabled, pausedBy, time.Now(), username, id)
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
	var siteID, pausedBy, createdBy, updatedBy sql.NullString
	var orphanedAt, createdAt, updatedAt sql.NullTime
	var assertValue string
	var cacheBypass sql.NullBool
	err := row.Scan(&c.ID, &siteID, &c.IsDefault, &c.Type, &c.Name, &c.Host, &c.Path, &c.Expect,
		&c.AssertPath, &c.AssertOp, &assertValue, &cacheBypass,
		&c.IntervalSeconds, &c.GraceSeconds, &c.FailThreshold, &c.Enabled, &pausedBy,
		&orphanedAt, &createdAt, &createdBy, &updatedAt, &updatedBy)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan monitor check: %w", err)
	}
	c.SiteID = siteID.String
	c.PausedByRuleID = pausedBy.String
	if assertValue != "" {
		c.AssertValue = json.RawMessage(assertValue)
	}
	if cacheBypass.Valid {
		b := cacheBypass.Bool
		c.CacheBypass = &b
	}
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

// ListMonitorCheckSecrets returns the secret headers of every check (or of
// one check, if checkID is given), without their values.
func ListMonitorCheckSecrets(checkID ...string) ([]models.MonitorCheckSecret, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT check_id, header_name, updated_at, COALESCE(updated_by, '') FROM monitor_check_secrets`
	var args []any
	if len(checkID) > 0 {
		query += ` WHERE check_id = ?`
		args = append(args, checkID[0])
	}
	rows, err := db.Query(query+` ORDER BY check_id, header_name`, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list monitor check secrets: %w", err)
	}
	defer rows.Close()

	secrets := []models.MonitorCheckSecret{}
	for rows.Next() {
		var sec models.MonitorCheckSecret
		var updatedAt sql.NullTime
		if err := rows.Scan(&sec.CheckID, &sec.Name, &updatedAt, &sec.UpdatedBy); err != nil {
			return nil, fmt.Errorf("failed to scan monitor check secret: %w", err)
		}
		sec.UpdatedAt = updatedAt.Time
		secrets = append(secrets, sec)
	}
	return secrets, rows.Err()
}

// GetMonitorCheckSecretValues returns secret header values by check ID and
// header name, for the checks' requests.
func GetMonitorCheckSecretValues() (map[string]map[string]string, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT check_id, header_name, value FROM monitor_check_secrets`)
	if err != nil {
		return nil, fmt.Errorf("failed to load monitor check secrets: %w", err)
	}
	defer rows.Close()

	values := map[string]map[string]string{}
	for rows.Next() {
		var checkID, name, value string
		if err := rows.Scan(&checkID, &name, &value); err != nil {
			return nil, fmt.Errorf("failed to scan monitor check secret: %w", err)
		}
		if values[checkID] == nil {
			values[checkID] = map[string]string{}
		}
		values[checkID][name] = value
	}
	return values, rows.Err()
}

// SetMonitorCheckSecret sets a secret header value on a check.
func SetMonitorCheckSecret(checkID, name, value, username string) error {
	db := GetAPIDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	_, err := db.Exec(`
		INSERT INTO monitor_check_secrets (check_id, header_name, value, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(check_id, header_name) DO UPDATE SET
			value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by
	`, checkID, name, value, time.Now(), username)
	if err != nil {
		return fmt.Errorf("failed to set secret %s on check %s: %w", name, checkID, err)
	}
	return nil
}

// DeleteMonitorCheckSecret removes a secret header from a check, reporting
// whether it existed.
func DeleteMonitorCheckSecret(checkID, name string) (bool, error) {
	db := GetAPIDB()
	if db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	res, err := db.Exec(`DELETE FROM monitor_check_secrets WHERE check_id = ? AND header_name = ?`, checkID, name)
	if err != nil {
		return false, fmt.Errorf("failed to delete secret %s from check %s: %w", name, checkID, err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CheckInTokenPrefix starts every check-in token.
const CheckInTokenPrefix = "jman_ci_"

// MonitorCheckToken describes a heartbeat check's check-in token. The token
// itself is only returned when issued.
type MonitorCheckToken struct {
	CheckID   string
	Prefix    string
	CreatedAt time.Time
	CreatedBy string
}

// IssueMonitorCheckToken creates a new check-in token for a check,
// replacing any earlier one, and returns the plaintext token. Only its
// hash is stored.
func IssueMonitorCheckToken(checkID, username string) (string, error) {
	db := GetAPIDB()
	if db == nil {
		return "", fmt.Errorf("database not initialized")
	}
	secret, err := randomToken(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate check-in token: %w", err)
	}
	raw := CheckInTokenPrefix + secret
	_, err = db.Exec(`
		INSERT INTO monitor_check_tokens (check_id, token_hash, token_prefix, created_at, created_by)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(check_id) DO UPDATE SET
			token_hash = excluded.token_hash, token_prefix = excluded.token_prefix,
			created_at = excluded.created_at, created_by = excluded.created_by
	`, checkID, hashRefreshToken(raw), raw[:len(CheckInTokenPrefix)+6], time.Now(), username)
	if err != nil {
		return "", fmt.Errorf("failed to store check-in token for check %s: %w", checkID, err)
	}
	return raw, nil
}

// GetMonitorCheckIDByToken returns the check a plaintext check-in token
// belongs to, or "" if it belongs to none.
func GetMonitorCheckIDByToken(raw string) (string, error) {
	db := GetAPIDB()
	if db == nil {
		return "", fmt.Errorf("database not initialized")
	}
	if !strings.HasPrefix(raw, CheckInTokenPrefix) {
		return "", nil
	}
	var checkID string
	err := db.QueryRow(`SELECT check_id FROM monitor_check_tokens WHERE token_hash = ?`, hashRefreshToken(raw)).Scan(&checkID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to look up check-in token: %w", err)
	}
	return checkID, nil
}

// ListMonitorCheckTokens returns every check-in token's description, by
// check ID.
func ListMonitorCheckTokens() (map[string]MonitorCheckToken, error) {
	db := GetAPIDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	rows, err := db.Query(`SELECT check_id, token_prefix, created_at, COALESCE(created_by, '') FROM monitor_check_tokens`)
	if err != nil {
		return nil, fmt.Errorf("failed to list check-in tokens: %w", err)
	}
	defer rows.Close()

	tokens := map[string]MonitorCheckToken{}
	for rows.Next() {
		var t MonitorCheckToken
		var createdAt sql.NullTime
		if err := rows.Scan(&t.CheckID, &t.Prefix, &createdAt, &t.CreatedBy); err != nil {
			return nil, fmt.Errorf("failed to scan check-in token: %w", err)
		}
		t.CreatedAt = createdAt.Time
		tokens[t.CheckID] = t
	}
	return tokens, rows.Err()
}
