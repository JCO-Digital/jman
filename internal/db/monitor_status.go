package db

import (
	"database/sql"
	"fmt"
)

// GetSiteMode returns the current monitoring mode for a given domain from the database.
func GetSiteMode(domain string) (string, error) {
	db := GetDB()
	if db == nil {
		return "", fmt.Errorf("database not initialized")
	}

	var mode string
	query := `SELECT current_mode FROM monitor_status WHERE domain = LOWER(?)`
	err := db.QueryRow(query, domain).Scan(&mode)
	if err != nil {
		if err == sql.ErrNoRows {
			return "normal", nil // Default mode if site has never been checked
		}
		return "", fmt.Errorf("failed to get site mode for %s: %w", domain, err)
	}
	return mode, nil
}

// IsSiteInAlertMode checks if a domain is currently in alert mode.
func IsSiteInAlertMode(domain string) (bool, error) {
	mode, err := GetSiteMode(domain)
	if err != nil {
		return false, err
	}
	return mode == "alert", nil
}
