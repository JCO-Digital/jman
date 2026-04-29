package db

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/models"
)

// GetIgnoredVulns returns all vulnerability UUIDs on the ignore list.
func GetIgnoredVulns() ([]models.IgnoredVuln, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT uuid, reason, created_at FROM vuln_ignored ORDER BY uuid ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query ignored vulns: %w", err)
	}
	defer rows.Close()

	var vulns []models.IgnoredVuln
	for rows.Next() {
		var v models.IgnoredVuln
		if err := rows.Scan(&v.UUID, &v.Reason, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan ignored vuln: %w", err)
		}
		vulns = append(vulns, v)
	}
	return vulns, nil
}

// GetIgnoredVulnMap returns a set of ignored UUIDs for fast lookup.
func GetIgnoredVulnMap() (map[string]bool, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT uuid FROM vuln_ignored`)
	if err != nil {
		return nil, fmt.Errorf("failed to query ignored vuln map: %w", err)
	}
	defer rows.Close()

	ignored := make(map[string]bool)
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			return nil, fmt.Errorf("failed to scan ignored vuln uuid: %w", err)
		}
		ignored[uuid] = true
	}
	return ignored, nil
}

// IgnoreVuln adds a vulnerability UUID to the ignore list.
func IgnoreVuln(uuid, reason string) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	_, err := db.Exec(`
		INSERT INTO vuln_ignored (uuid, reason)
		VALUES (?, ?)
		ON CONFLICT(uuid) DO UPDATE SET
			reason = excluded.reason,
			created_at = CURRENT_TIMESTAMP
	`, uuid, reason)
	if err != nil {
		return fmt.Errorf("failed to ignore vuln %s: %w", uuid, err)
	}
	return nil
}

// UnignoreVuln removes a vulnerability UUID from the ignore list.
// Returns nil without error if the UUID was not on the list.
func UnignoreVuln(uuid string) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	res, err := db.Exec(`DELETE FROM vuln_ignored WHERE uuid = ?`, uuid)
	if err != nil {
		return fmt.Errorf("failed to unignore vuln %s: %w", uuid, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("vulnerability UUID %q not found in ignore list", uuid)
	}
	return nil
}
