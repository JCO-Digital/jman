package db

import (
	"fmt"

	"github.com/JCO-Digital/jman/internal/models"
)

// GetIgnoredSites returns a list of all currently ignored sites.
func GetIgnoredSites() ([]models.IgnoredSite, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT domain, reason, created_at FROM monitor_ignored_sites ORDER BY domain ASC`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query ignored sites: %w", err)
	}
	defer rows.Close()

	sites := []models.IgnoredSite{}
	for rows.Next() {
		var s models.IgnoredSite
		if err := rows.Scan(&s.Domain, &s.Reason, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan ignored site: %w", err)
		}
		sites = append(sites, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sites, nil
}

// IsSiteIgnored checks if a domain is in the ignored sites list.
func IsSiteIgnored(domain string) (bool, error) {
	db := GetDB()
	if db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM monitor_ignored_sites WHERE domain = LOWER(?))`
	err := db.QueryRow(query, domain).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if site is ignored: %w", err)
	}
	return exists, nil
}

// GetIgnoredDomains returns a map of ignored domains for fast lookup.
func GetIgnoredDomains() (map[string]bool, error) {
	db := GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := `SELECT LOWER(domain) FROM monitor_ignored_sites`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query ignored domains: %w", err)
	}
	defer rows.Close()

	ignored := make(map[string]bool)
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			return nil, fmt.Errorf("failed to scan ignored domain: %w", err)
		}
		ignored[domain] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ignored, nil
}

// IgnoreSite adds a site to the ignore list and logs the action to history.
func IgnoreSite(domain, reason string) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert or update the ignored site
	_, err = tx.Exec(`
		INSERT INTO monitor_ignored_sites (domain, reason)
		VALUES (LOWER(?), ?)
		ON CONFLICT(domain) DO UPDATE SET
			reason = excluded.reason,
			created_at = CURRENT_TIMESTAMP
	`, domain, reason)
	if err != nil {
		return fmt.Errorf("failed to ignore site %s: %w", domain, err)
	}

	// Log to history
	_, err = tx.Exec(`
		INSERT INTO monitor_ignored_history (domain, action, reason)
		VALUES (?, 'ignore', ?)
	`, domain, reason)
	if err != nil {
		return fmt.Errorf("failed to log ignore history for %s: %w", domain, err)
	}

	return tx.Commit()
}

// UnignoreSite removes a site from the ignore list and logs the action to history.
func UnignoreSite(domain string) error {
	db := GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete from ignored sites
	res, err := tx.Exec(`DELETE FROM monitor_ignored_sites WHERE domain = LOWER(?)`, domain)
	if err != nil {
		return fmt.Errorf("failed to unignore site %s: %w", domain, err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return nil // Already not ignored
	}

	// Log to history
	_, err = tx.Exec(`
		INSERT INTO monitor_ignored_history (domain, action)
		VALUES (?, 'unignore')
	`, domain)
	if err != nil {
		return fmt.Errorf("failed to log unignore history for %s: %w", domain, err)
	}

	return tx.Commit()
}
