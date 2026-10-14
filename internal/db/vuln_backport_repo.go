package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// ErrDuplicateVulnBackport is returned when a rule for the same
// vulnerability and branch already exists.
var ErrDuplicateVulnBackport = errors.New("a backport rule for this vulnerability and branch already exists")

const vulnBackportColumns = `id, type, slug, vuln_uuid, branch, min_version, reason, created_at, created_by, updated_at, updated_by`

func scanVulnBackport(row interface{ Scan(...any) error }) (models.VulnBackport, error) {
	var b models.VulnBackport
	err := row.Scan(&b.ID, &b.Type, &b.Slug, &b.VulnUUID, &b.Branch, &b.MinVersion, &b.Reason, &b.CreatedAt, &b.CreatedBy, &b.UpdatedAt, &b.UpdatedBy)
	return b, err
}

// CreateVulnBackport inserts a new backport rule. It returns
// ErrDuplicateVulnBackport if the vulnerability/branch pair already exists.
func CreateVulnBackport(rule *models.VulnBackport, username string) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	result, err := db.Exec(`
		INSERT INTO vuln_backports (type, slug, vuln_uuid, branch, min_version, reason, created_at, created_by, updated_at, updated_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.Type, rule.Slug, rule.VulnUUID, rule.Branch, rule.MinVersion, rule.Reason, now, username, now, username)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrDuplicateVulnBackport
		}
		return fmt.Errorf("failed to insert backport rule: %w", err)
	}
	id, _ := result.LastInsertId()
	rule.ID = int(id)
	rule.CreatedAt = now
	rule.CreatedBy = username
	rule.UpdatedAt = now
	rule.UpdatedBy = username
	return nil
}

// GetVulnBackport fetches a single backport rule by ID, or nil if not found.
func GetVulnBackport(id int) (*models.VulnBackport, error) {
	db := GetInventoryDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	b, err := scanVulnBackport(db.QueryRow(`SELECT `+vulnBackportColumns+` FROM vuln_backports WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get backport rule: %w", err)
	}
	return &b, nil
}

// ListVulnBackports returns all backport rules.
func ListVulnBackports() ([]models.VulnBackport, error) {
	db := GetInventoryDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query(`SELECT ` + vulnBackportColumns + ` FROM vuln_backports ORDER BY type ASC, slug ASC, vuln_uuid ASC, branch ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query backport rules: %w", err)
	}
	defer rows.Close()

	rules := []models.VulnBackport{}
	for rows.Next() {
		b, err := scanVulnBackport(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, b)
	}
	return rules, rows.Err()
}

// DeleteVulnBackport removes a backport rule.
func DeleteVulnBackport(id int) error {
	db := GetInventoryDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM vuln_backports WHERE id = ?", id)
	return err
}
